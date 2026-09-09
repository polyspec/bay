package stack

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCerts creates empty certificate files, which is enough for the unused
// test: it reads the file names, not their contents.
func writeCerts(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n+".crt"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func unused(t *testing.T, dir string, use CertUse) map[string]bool {
	t.Helper()
	list, err := Certificates(dir, use)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, c := range list {
		out[c.Name] = c.Orphaned
	}
	return out
}

// A project that is stopped still asks for its domains, and starting it needs
// their certificates. Only a name nothing asks for is unused, which is what a
// project whose files are gone leaves behind.
func TestOnlyANameNothingAsksForIsUnused(t *testing.T) {
	dir := writeCerts(t, DefaultCertName, "web.test", "leftover.test")
	got := unused(t, dir, CertUse{Declared: []string{"web.test"}})
	if got["web.test"] {
		t.Error("the certificate of a declared domain is called unused")
	}
	if !got["leftover.test"] {
		t.Error("a certificate no project asks for is not called unused")
	}
	if got[DefaultCertName] {
		t.Error("the default certificate is machine state and is never unused")
	}
}

// A domain being served is enough on its own.
func TestAServedDomainsCertificateIsNotUnused(t *testing.T) {
	dir := writeCerts(t, "web.test")
	if unused(t, dir, CertUse{Routed: []string{"web.test"}})["web.test"] {
		t.Error("the certificate of a served domain is called unused")
	}
}

// Names are compared without case, the way domains are.
func TestTheUnusedTestIgnoresCase(t *testing.T) {
	dir := writeCerts(t, "web.test")
	if unused(t, dir, CertUse{Declared: []string{"WEB.TEST"}})["web.test"] {
		t.Error("a declared domain in another case is not recognised")
	}
}

// A project whose file is there but could not be read still has its files. What
// it asks for is not known, so nothing is called unused: the answer would be a
// button offering to remove a certificate the project needs.
func TestNothingIsUnusedWhileAProjectCannotBeRead(t *testing.T) {
	dir := writeCerts(t, "web.test", "leftover.test")
	got := unused(t, dir, CertUse{Unread: true})
	for name, isUnused := range got {
		if isUnused {
			t.Errorf("%s is called unused while a project could not be read", name)
		}
	}
}
