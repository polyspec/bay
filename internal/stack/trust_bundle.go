package stack

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// Every service container trusts this machine's authority, as the machine
// does. A program in a container verifies a certificate against the store of
// its image, not the host's, so the name of a project on this machine or on a
// peer, which the proxy serves with a certificate from this authority, would
// fail verification there.
//
// The state directory holds what the containers read, in trust/:
//
//   - ca-bundle.crt: the system's public roots and this machine's authority, in
//     PEM, mounted read-only at /etc/bay/ca-bundle.crt and named by
//     SSL_CERT_FILE, CURL_CA_BUNDLE, REQUESTS_CA_BUNDLE and
//     NODE_EXTRA_CA_CERTS, which Node adds to its own roots.
//   - ca.crt: the authority alone, mounted at /etc/bay/ca.crt, which bayinit
//     adds to the image's own store before the service's command runs (see
//     bayinit.go), for programs that read only the store, such as libcurl,
//     PHP and GnuTLS.
//   - truststore.p12: the same certificates as a PKCS #12 trust store, mounted
//     at /etc/bay/truststore.p12 and named by JAVA_TOOL_OPTIONS,
//     because Java reads neither of the above.
//   - bayinit: the program above, for this machine's architecture.
//
// Nothing is mounted over the image's store, so a package that rewrites it,
// such as ca-certificates, installs as it does without bay.
//
// The files are rewritten in place when their content changes, so a running
// container's mount reads the new content. A service the Compose file marks
// with x-bay.trust: false is given none of this, and an environment
// variable or a mount path the service sets itself is left to the service.

// TrustDir is the directory of the store containers read.
func TrustDir(dir string) string { return filepath.Join(dir, "trust") }

const (
	javaTrustStorePath = "/etc/bay/truststore.p12"
	trustStorePassword = "changeit"
)

// systemRoots are where a system keeps its public roots in PEM, in the order
// they are looked for: Debian, Ubuntu and Alpine; Fedora and RHEL; macOS. Tests
// replace it.
var systemRoots = []string{
	"/etc/ssl/certs/ca-certificates.crt",
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/ssl/cert.pem",
}

// findSystemRoots returns the system's public roots and where they were read.
func findSystemRoots() ([]byte, string, bool) {
	for _, p := range systemRoots {
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte("BEGIN CERTIFICATE")) {
			return b, p, true
		}
	}
	return nil, "", false
}

// trustFiles returns the content of each file of the store. The bundle carries
// the authority once: a system that already trusts it holds it in its roots.
func trustFiles(roots, ca []byte) (map[string][]byte, error) {
	bundle := append([]byte{}, roots...)
	if !bytes.Contains(normalizePEM(roots), normalizePEM(ca)) {
		if len(bundle) > 0 && !bytes.HasSuffix(bundle, []byte("\n")) {
			bundle = append(bundle, '\n')
		}
		bundle = append(bundle, ca...)
	}
	return map[string][]byte{"ca-bundle.crt": bundle, "ca.crt": ca}, nil
}

// normalizePEM removes whitespace, so the same certificate compares equal
// however its lines are broken.
func normalizePEM(b []byte) []byte {
	return []byte(strings.Join(strings.Fields(string(b)), ""))
}

// TrustItem is the store containers read, as a setup part.
func TrustItem(dir string) SetupItem {
	name := "trust in containers " + TrustDir(dir)
	roots, from, ok := findSystemRoots()
	if !ok {
		return SetupItem{Name: name + ": the system has no public roots in PEM at " +
			strings.Join(systemRoots, ", ") + "; install ca-certificates"}
	}
	if _, ok := initBinary(); !ok {
		name += ", without bayinit in this build: programs that read only the image's store do not trust it; build with make"
	}
	return SetupItem{
		Name: name + " (roots from " + from + ")", OK: trustCurrent(dir, roots) && initCurrent(dir),
		SetupStep: SetupStep{Text: "write the certificates containers trust in " + TrustDir(dir)},
	}
}

func trustCurrent(dir string, roots []byte) bool {
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return false
	}
	files, err := trustFiles(roots, ca)
	if err != nil {
		return false
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(TrustDir(dir), name))
		if err != nil || !bytes.Equal(got, want) {
			return false
		}
	}
	got, err := os.ReadFile(filepath.Join(TrustDir(dir), "truststore.sha256"))
	return err == nil && strings.TrimSpace(string(got)) == bundleSum(files["ca-bundle.crt"])
}

func bundleSum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// EnsureTrust writes the store containers read when it differs from what the
// system and the authority hold. It changes nothing when the store is current.
func EnsureTrust(dir string) error {
	return withLock(dir, stateLockName, stateLockWait, func() error { return ensureTrust(dir) })
}

func ensureTrust(dir string) error {
	roots, _, ok := findSystemRoots()
	if !ok {
		return nil
	}
	if err := ensureInit(dir); err != nil {
		return err
	}
	if trustCurrent(dir, roots) {
		return nil
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return err
	}
	files, err := trustFiles(roots, ca)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(TrustDir(dir), 0o755); err != nil {
		return err
	}
	for name, content := range files {
		if err := writeInPlace(filepath.Join(TrustDir(dir), name), content); err != nil {
			return err
		}
	}
	// PKCS #12 is encrypted with a random salt, so the same certificates give
	// other bytes each time. It is written only when the bundle changed.
	store, err := javaTrustStore(files["ca-bundle.crt"])
	if err != nil {
		return err
	}
	if err := writeInPlace(filepath.Join(TrustDir(dir), "truststore.p12"), store); err != nil {
		return err
	}
	return writeInPlace(filepath.Join(TrustDir(dir), "truststore.sha256"),
		[]byte(bundleSum(files["ca-bundle.crt"])+"\n"))
}

// writeInPlace replaces a file's content without replacing the file, so a
// bind mount of it reads the new content.
func writeInPlace(path string, content []byte) error {
	if got, err := os.ReadFile(path); err == nil && bytes.Equal(got, content) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// javaTrustStore encodes the certificates of a PEM bundle as a PKCS #12 trust
// store. A block that is not a certificate Go parses is left out.
func javaTrustStore(bundle []byte) ([]byte, error) {
	var certs []*x509.Certificate
	for rest := bundle; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if c, err := x509.ParseCertificate(block.Bytes); err == nil {
			certs = append(certs, c)
		}
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("the bundle holds no certificate")
	}
	return pkcs12.Modern.EncodeTrustStore(certs, trustStorePassword)
}

// trustArguments are the arguments that give a service the store. A service
// that turned trust off, a store not written yet, a path the service mounts
// itself and a variable it sets itself are left out.
//
// Every mount is of one file. Apple `container` drops a directory mount whose
// host directory is also the source of a file mount in the same container
// (observed: the configuration listed both, and the guest held only the file
// mounts), so the store's directory is never mounted as a whole.
func trustArguments(dir string, s *Service) []string {
	if !s.Trust || dir == "" {
		return nil
	}
	store := filepath.Join(TrustDir(dir), "truststore.p12")
	if _, err := os.Stat(store); err != nil {
		return nil
	}
	taken := map[string]bool{}
	for _, v := range s.Volumes {
		parts := strings.Split(v, ":")
		if len(parts) >= 2 {
			taken[parts[1]] = true
		}
	}
	var args []string
	for _, m := range [][2]string{
		{filepath.Join(TrustDir(dir), "ca-bundle.crt"), bundleContainerPath},
		{filepath.Join(TrustDir(dir), "ca.crt"), caContainerPath},
		{store, javaTrustStorePath},
	} {
		if !taken[m[1]] {
			args = append(args, "--volume", m[0]+":"+m[1]+":ro")
		}
	}
	env := [][2]string{
		{"SSL_CERT_FILE", bundleContainerPath},
		{"CURL_CA_BUNDLE", bundleContainerPath},
		{"REQUESTS_CA_BUNDLE", bundleContainerPath},
		{"NODE_EXTRA_CA_CERTS", bundleContainerPath},
		{"JAVA_TOOL_OPTIONS", "-Djavax.net.ssl.trustStore=" + javaTrustStorePath +
			" -Djavax.net.ssl.trustStoreType=PKCS12 -Djavax.net.ssl.trustStorePassword=" + trustStorePassword},
	}
	for _, kv := range env {
		if _, set := s.Env[kv[0]]; !set {
			args = append(args, "--env", kv[0]+"="+kv[1])
		}
	}
	return args
}
