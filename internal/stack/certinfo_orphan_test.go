package stack

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCerts creates empty certificate files, which is enough for the orphan
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

func orphans(t *testing.T, dir string, routed, declared []string) map[string]bool {
	t.Helper()
	list, err := Certificates(dir, routed, declared)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, c := range list {
		out[c.Name] = c.Orphaned
	}
	return out
}

// A project that is stopped still declares its domains, and starting it needs
// the certificate. Reporting it as unused offers to remove something the next
// start requires.
func TestAStoppedProjectsCertificateIsNotOrphaned(t *testing.T) {
	dir := writeCerts(t, DefaultCertName, "web.test", "leftover.test")
	got := orphans(t, dir, nil, []string{"web.test"})
	if got["web.test"] {
		t.Error("the certificate of a declared domain is reported as unused")
	}
	if !got["leftover.test"] {
		t.Error("a certificate no project declares is not reported as unused")
	}
	if got[DefaultCertName] {
		t.Error("the default certificate is machine state and is never unused")
	}
}

// A domain being routed is enough on its own.
func TestARoutedDomainsCertificateIsNotOrphaned(t *testing.T) {
	dir := writeCerts(t, "web.test")
	if orphans(t, dir, []string{"web.test"}, nil)["web.test"] {
		t.Error("the certificate of a routed domain is reported as unused")
	}
}

// Names are compared without case, the way domains are.
func TestOrphanTestIgnoresCase(t *testing.T) {
	dir := writeCerts(t, "web.test")
	if orphans(t, dir, nil, []string{"WEB.TEST"})["web.test"] {
		t.Error("a declared domain in another case is not recognised")
	}
}
