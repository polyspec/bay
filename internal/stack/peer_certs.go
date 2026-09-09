package stack

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The files the proxy reads for the peer link. They are kept apart from the
// certificates the proxy serves, because they are about who may connect rather
// than about a name.
const (
	// PeerDirName holds them inside the state directory.
	PeerDirName = "peers"
	// ClientCertName is what this machine presents on a peer's link.
	ClientCertName = "client"
	// PeerAuthoritiesName holds every approved authority, which is what a
	// client certificate arriving on this machine's link is checked against.
	PeerAuthoritiesName = "authorities.pem"
)

// PeerDir returns the directory the peer link's files are kept in.
func PeerDir(dir string) string { return filepath.Join(dir, PeerDirName) }

// PeerAuthorityPath returns the file holding one peer's authority. A peer's own
// certificate is checked against it when this machine connects to that peer.
func PeerAuthorityPath(dir, fingerprint string) string {
	return filepath.Join(PeerDir(dir), fingerprint+".crt")
}

// IssueClient writes the certificate this machine presents on a peer's link.
// name is the machine's name and is what a peer sees in the certificate.
//
// The certificate is kept once it exists, so a peer that approved this machine
// keeps recognising it.
func (c *CA) IssueClient(peerDir, name string) error {
	if err := os.MkdirAll(peerDir, 0o700); err != nil {
		return err
	}
	crtPath := filepath.Join(peerDir, ClientCertName+".crt")
	keyPath := filepath.Join(peerDir, ClientCertName+".key")

	if crtPEM, err := os.ReadFile(crtPath); err == nil {
		if keyPEM, err := os.ReadFile(keyPath); err == nil {
			if cert, _, err := parsePair(crtPEM, keyPEM); err == nil {
				if time.Until(cert.NotAfter) > 30*24*time.Hour {
					return nil
				}
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return err
	}
	if err := writePEM(crtPath, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600)
}

// WritePeerAuthorities writes what the proxy reads about the approved peers:
// one file per peer, and one file holding all of them. A peer whose approval
// was withdrawn is left out of both.
func WritePeerAuthorities(dir string, peers []Peer) error {
	peerDir := PeerDir(dir)
	if err := os.MkdirAll(peerDir, 0o700); err != nil {
		return err
	}
	var all strings.Builder
	keep := map[string]bool{}
	for _, p := range peers {
		pem := p.CA
		if !strings.HasSuffix(pem, "\n") {
			pem += "\n"
		}
		all.WriteString(pem)
		keep[p.Fingerprint+".crt"] = true
		if err := os.WriteFile(PeerAuthorityPath(dir, p.Fingerprint), []byte(pem), 0o644); err != nil {
			return err
		}
	}
	// nginx refuses to start on an empty ssl_client_certificate file, so the
	// link is left out entirely when there is no peer. The file is still
	// written, so nothing reads a stale one.
	if err := os.WriteFile(filepath.Join(peerDir, PeerAuthoritiesName),
		[]byte(all.String()), 0o644); err != nil {
		return err
	}

	entries, err := os.ReadDir(peerDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".crt") || name == ClientCertName+".crt" || keep[name] {
			continue
		}
		if err := os.Remove(filepath.Join(peerDir, name)); err != nil {
			return err
		}
	}
	return nil
}
