package stack

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CA is the local signing authority stored in the state directory. Adding its
// certificate to the user's trust settings trusts every certificate it
// issues.
type CA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	dir  string
}

// LoadOrCreateCA reads the authority from dir, creating it when absent.
func LoadOrCreateCA(dir string) (*CA, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	crtPath, keyPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")

	crtPEM, err := os.ReadFile(crtPath)
	if err == nil {
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
		cert, key, err := parsePair(crtPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", crtPath, err)
		}
		return &CA{cert: cert, key: key, dir: dir}, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "bay local CA", Organization: []string{"bay"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	if err := writePEM(crtPath, "CERTIFICATE", der, 0o644); err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return nil, err
	}
	return &CA{cert: cert, key: key, dir: dir}, nil
}

func (c *CA) CertPath() string { return filepath.Join(c.dir, "ca.crt") }

// Issue writes a leaf certificate for domain into certDir, covering the name
// and one level of subdomain.
func (c *CA) Issue(certDir, domain string) (bool, error) {
	return c.IssueWithNames(certDir, domain, []string{domain, "*." + domain})
}

// IssueWithNames writes a leaf certificate stored under name and valid for
// dnsNames. It reuses an existing certificate that has more than 30 days left
// and covers every requested name.
func (c *CA) IssueWithNames(certDir, name string, dnsNames []string) (bool, error) {
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		return false, err
	}
	crtPath := filepath.Join(certDir, name+".crt")
	keyPath := filepath.Join(certDir, name+".key")

	if crtPEM, err := os.ReadFile(crtPath); err == nil {
		if keyPEM, err := os.ReadFile(keyPath); err == nil {
			if cert, _, err := parsePair(crtPEM, keyPEM); err == nil {
				if time.Until(cert.NotAfter) > 30*24*time.Hour && covers(cert, dnsNames) {
					return false, nil
				}
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return false, err
	}
	if err := writePEM(crtPath, "CERTIFICATE", der, 0o644); err != nil {
		return false, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return false, err
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// covers reports whether cert lists every name in its subject alternative
// names.
func covers(cert *x509.Certificate, names []string) bool {
	have := make(map[string]bool, len(cert.DNSNames))
	for _, n := range cert.DNSNames {
		have[n] = true
	}
	for _, n := range names {
		if !have[n] {
			return false
		}
	}
	return true
}

// parseCertOnly reads a certificate from PEM without its key.
func parseCertOnly(crtPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(crtPEM)
	if block == nil {
		return nil, fmt.Errorf("malformed PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parsePair(crtPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cb, _ := pem.Decode(crtPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, nil, fmt.Errorf("malformed PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), mode)
}

func serial() *big.Int {
	max := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(err)
	}
	return n
}
