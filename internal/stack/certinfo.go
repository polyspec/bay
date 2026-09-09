package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CertInfo describes one issued certificate.
type CertInfo struct {
	// Name is the file's base name, which is also how Issue addresses it.
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	DNSNames  []string  `json:"dnsNames"`
	NotAfter  time.Time `json:"notAfter"`
	IsCA      bool      `json:"isCA"`
	Trusted   bool      `json:"trusted,omitempty"`
	Path      string    `json:"path"`
	Orphaned  bool      `json:"orphaned,omitempty"`
	ReadError string    `json:"readError,omitempty"`
	daysLeft  int
}

// DaysLeft returns the whole days remaining before the certificate expires. It
// is negative once the certificate has expired.
func (c CertInfo) DaysLeft() int { return c.daysLeft }

// Unreadable returns why the certificate could not be parsed, or an empty
// string when it was read.
func (c CertInfo) Unreadable() string { return c.ReadError }

// Status returns a short description of the certificate's validity.
func (c CertInfo) Status() string {
	switch {
	case c.ReadError != "":
		return "unreadable: " + c.ReadError
	case c.Expired():
		return "expired"
	case c.daysLeft <= 30:
		return fmt.Sprintf("expires in %d day%s", c.daysLeft, pluralDays(c.daysLeft))
	default:
		return "valid until " + c.NotAfter.Format("2006-01-02")
	}
}

func (c CertInfo) Expired() bool { return c.ReadError == "" && time.Now().After(c.NotAfter) }
func (c CertInfo) NeedsAttention() bool {
	return c.ReadError != "" || c.Expired() || c.daysLeft <= 30 || c.Orphaned
}

func pluralDays(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// CertUse is what asks for a certificate's name. Whether a project is running
// is not part of it: a stopped project needs its certificate the moment it
// starts again.
type CertUse struct {
	// Routed are the domains the proxy serves right now.
	Routed []string
	// Declared are the domains the registered projects ask for, read from their
	// Compose files.
	Declared []string
	// Unread says a registered project's file is there but could not be read.
	// What it asks for is not known, so no certificate is called unused: the
	// alternative is offering to remove one the project needs.
	Unread bool
}

// Certificates returns the leaf certificates in certDir. A certificate whose
// name nothing in use asks for is reported as orphaned, which is what a project
// whose files are gone leaves behind.
func Certificates(certDir string, use CertUse) ([]CertInfo, error) {
	entries, err := os.ReadDir(certDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool, len(use.Routed)+len(use.Declared))
	for _, d := range append(append([]string{}, use.Routed...), use.Declared...) {
		used[strings.ToLower(d)] = true
	}

	var out []CertInfo
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".crt") {
			continue
		}
		base := strings.TrimSuffix(name, ".crt")
		info := readCert(filepath.Join(certDir, name))
		info.Name = base
		// The default certificate is machine state and is never orphaned.
		info.Orphaned = !use.Unread && base != DefaultCertName && !used[strings.ToLower(base)]
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// AuthorityInfo reads the public CA certificate without opening a private key
// or creating any machine state.
func AuthorityInfo(dir string) CertInfo {
	info := readCert(filepath.Join(dir, "ca.crt"))
	info.Name = "ca"
	info.IsCA = true
	info.Trusted = info.ReadError == "" && CATrusted(info.Path)
	return info
}

func readCert(path string) CertInfo {
	info := CertInfo{Path: path}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		info.ReadError = err.Error()
		return info
	}
	cert, err := parseCertOnly(pemBytes)
	if err != nil {
		info.ReadError = err.Error()
		return info
	}
	info.Subject = cert.Subject.CommonName
	info.DNSNames = cert.DNSNames
	info.NotAfter = cert.NotAfter
	info.daysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
	return info
}

// RemoveCertificate deletes one issued certificate and its key. It does not
// remove the authority.
func (c *CA) RemoveCertificate(certDir, name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%q is not a certificate name", name)
	}
	crt := filepath.Join(certDir, name+".crt")
	if _, err := os.Stat(crt); err != nil {
		return fmt.Errorf("no certificate named %q", name)
	}
	if err := os.Remove(crt); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(certDir, name+".key")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Reissue removes a certificate. The next proxy sync issues a new one.
func (c *CA) Reissue(certDir, name string) error {
	return c.RemoveCertificate(certDir, name)
}

// RotateCA replaces the signing authority. It keeps the previous certificate so
// one step can remove its trust and add the new one, and it deletes every
// certificate the previous authority signed.
//
// It returns the path of the retired certificate. The caller passes it to
// Install as UntrustCA and removes it afterwards.
func RotateCA(dir, certDir string) (retired string, err error) {
	old := filepath.Join(dir, "ca.crt")
	if _, err := os.Stat(old); err == nil {
		retired = filepath.Join(dir, "ca.retired.crt")
		if err := os.Rename(old, retired); err != nil {
			return "", err
		}
	}
	if err := os.Remove(filepath.Join(dir, "ca.key")); err != nil && !os.IsNotExist(err) {
		return retired, err
	}
	if err := os.RemoveAll(certDir); err != nil {
		return retired, err
	}
	if _, err := LoadOrCreateCA(dir); err != nil {
		return retired, err
	}
	return retired, nil
}
