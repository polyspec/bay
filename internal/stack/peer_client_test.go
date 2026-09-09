package stack

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

// What a machine presents on a peer's link is a client certificate: a peer
// checks it as a client, and a certificate marked only for servers is refused.
func TestTheClientCertificateIsMarkedForClients(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	peerDir := filepath.Join(dir, "peers")
	if err := ca.IssueClient(peerDir, "max"); err != nil {
		t.Fatal(err)
	}
	crtPEM, err := os.ReadFile(filepath.Join(peerDir, ClientCertName+".crt"))
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(filepath.Join(peerDir, ClientCertName+".key"))
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := parsePair(crtPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	var forClients bool
	for _, u := range cert.ExtKeyUsage {
		if u == x509.ExtKeyUsageClientAuth {
			forClients = true
		}
	}
	if !forClients {
		t.Errorf("the certificate is not marked for clients: %v", cert.ExtKeyUsage)
	}
	if cert.Subject.CommonName != "max" {
		t.Errorf("common name %q, want the machine's name", cert.Subject.CommonName)
	}
}

// Issuing again keeps the certificate, so a peer that approved this machine
// keeps recognising it.
func TestTheClientCertificateIsKept(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	peerDir := filepath.Join(dir, "peers")
	if err := ca.IssueClient(peerDir, "max"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(peerDir, ClientCertName+".crt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ca.IssueClient(peerDir, "max"); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(peerDir, ClientCertName+".crt"))
	if string(first) != string(again) {
		t.Error("the certificate was replaced")
	}
}

// The authorities a link accepts are the approved peers' authorities, in one
// file, and only those.
func TestTheAcceptedAuthoritiesAreTheApprovedOnes(t *testing.T) {
	dir := t.TempDir()
	a, b := testCA(t), testCA(t)
	for _, ca := range []string{a, b} {
		if err := ApprovePeer(dir, Peer{Name: "peer", Address: "10.0.0.1:8443", CA: ca}); err != nil {
			t.Fatal(err)
		}
	}
	peers, _ := Peers(dir)
	if err := WritePeerAuthorities(dir, peers); err != nil {
		t.Fatal(err)
	}
	all, err := os.ReadFile(filepath.Join(dir, "peers", PeerAuthoritiesName))
	if err != nil {
		t.Fatal(err)
	}
	for _, ca := range []string{a, b} {
		if !containsPEM(string(all), ca) {
			t.Error("an approved authority is not accepted")
		}
	}
	for _, p := range peers {
		one, err := os.ReadFile(PeerAuthorityPath(dir, p.Fingerprint))
		if err != nil {
			t.Fatalf("no file for %s: %v", p.Fingerprint, err)
		}
		if !containsPEM(string(one), p.CA) {
			t.Error("a peer's own authority file does not hold its authority")
		}
	}

	if err := RemovePeer(dir, peers[0].Fingerprint); err != nil {
		t.Fatal(err)
	}
	left, _ := Peers(dir)
	if err := WritePeerAuthorities(dir, left); err != nil {
		t.Fatal(err)
	}
	all, _ = os.ReadFile(filepath.Join(dir, "peers", PeerAuthoritiesName))
	if containsPEM(string(all), peers[0].CA) {
		t.Error("a withdrawn authority is still accepted")
	}
}

func containsPEM(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle || contains(haystack, needle))
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
