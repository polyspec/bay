package stack

import (
	"os"
	"path/filepath"
	"testing"
)

// A peer's domain is answered here with a certificate this machine issued, so
// the browser is offered one from an authority it already trusts. That
// certificate is in use. Reporting it as unused offers to remove the one thing
// that lets the name be opened at all.
func TestAPeersDomainCertificateIsInUse(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	certDir := filepath.Join(dir, "certs")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"polyspec.test", "own.test"} {
		if _, err := ca.Issue(certDir, d); err != nil {
			t.Fatal(err)
		}
	}

	list, err := Certificates(certDir, CertUse{
		Routed: []string{"own.test"},
		Peered: []string{"polyspec.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.Name == "polyspec.test" && c.Orphaned {
			t.Error("a peer's domain certificate is reported unused while it is what serves that name")
		}
		if c.Name == "own.test" && c.Orphaned {
			t.Error("a routed domain's certificate is reported unused")
		}
	}
}

// A name that no route, project or peer asks for is still reported, because
// that is what a removed project leaves behind.
func TestANameNothingAsksForIsStillUnused(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	certDir := filepath.Join(dir, "certs")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ca.Issue(certDir, "gone.test"); err != nil {
		t.Fatal(err)
	}
	list, err := Certificates(certDir, CertUse{Peered: []string{"polyspec.test"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Orphaned {
		t.Errorf("a name nothing asks for is no longer reported unused: %+v", list)
	}
}
