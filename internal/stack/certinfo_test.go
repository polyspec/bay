package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newCA(t *testing.T) (*CA, string, string) {
	t.Helper()
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ca, dir, filepath.Join(dir, "certs")
}

func TestCertificatesReportUseAndExpiry(t *testing.T) {
	ca, _, certDir := newCA(t)
	for _, d := range []string{"a.test", "b.test", DefaultCertName} {
		if _, err := ca.Issue(certDir, d); err != nil {
			t.Fatal(err)
		}
	}

	list, err := Certificates(certDir, CertUse{Routed: []string{"a.test"}})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]CertInfo{}
	for _, c := range list {
		byName[c.Name] = c
	}
	if len(byName) != 3 {
		t.Fatalf("listed %d certificates, want 3", len(byName))
	}
	if byName["a.test"].Orphaned {
		t.Error("a routed domain was reported as orphaned")
	}
	if !byName["b.test"].Orphaned {
		t.Error("a domain no route uses should be reported as orphaned")
	}
	// The default certificate belongs to the machine, not to a route.
	if byName[DefaultCertName].Orphaned {
		t.Error("the default certificate was reported as orphaned")
	}
	if got := byName["a.test"]; !strings.HasPrefix(got.Status(), "valid until") {
		t.Errorf("status = %q", got.Status())
	}
	if got := byName["a.test"].NotAfter; time.Until(got) < 300*24*time.Hour {
		t.Errorf("a fresh certificate expires at %v", got)
	}
}

func TestCertificatesReportsUnreadableFiles(t *testing.T) {
	_, _, certDir := newCA(t)
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "broken.crt"), []byte("not pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := Certificates(certDir, CertUse{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !strings.HasPrefix(list[0].Status(), "unreadable") {
		t.Fatalf("list = %+v", list)
	}
	if !list[0].NeedsAttention() {
		t.Error("an unreadable certificate should need attention")
	}
}

func TestRemoveCertificateTakesTheKeyToo(t *testing.T) {
	ca, _, certDir := newCA(t)
	if _, err := ca.Issue(certDir, "gone.test"); err != nil {
		t.Fatal(err)
	}
	if err := ca.RemoveCertificate(certDir, "gone.test"); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".crt", ".key"} {
		if _, err := os.Stat(filepath.Join(certDir, "gone.test"+ext)); !os.IsNotExist(err) {
			t.Errorf("%s survived removal", ext)
		}
	}
	if err := ca.RemoveCertificate(certDir, "gone.test"); err == nil {
		t.Error("removing a missing certificate reported success")
	}
	if err := ca.RemoveCertificate(certDir, "../escape"); err == nil {
		t.Error("a path was accepted as a certificate name")
	}
}

// Rotating has to invalidate everything the old authority signed, or the proxy
// keeps serving certificates that no longer chain to a trusted root.
func TestRotateCAReplacesEverything(t *testing.T) {
	ca, dir, certDir := newCA(t)
	if _, err := ca.Issue(certDir, "a.test"); err != nil {
		t.Fatal(err)
	}
	before := AuthorityInfo(dir)

	retired, err := RotateCA(dir, certDir)
	if err != nil {
		t.Fatal(err)
	}
	if retired == "" {
		t.Fatal("the old authority was not kept for untrusting")
	}
	if _, err := os.Stat(retired); err != nil {
		t.Fatalf("retired certificate missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(certDir, "a.test.crt")); !os.IsNotExist(err) {
		t.Error("a certificate signed by the old authority survived")
	}

	next, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if AuthorityInfo(dir).NotAfter.Equal(before.NotAfter) && next.cert.SerialNumber.Cmp(ca.cert.SerialNumber) == 0 {
		t.Fatal("the authority was not actually replaced")
	}
}
