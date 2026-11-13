package stack

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// useRoots makes a directory with an authority and system roots made of
// another authority.
func useRoots(t *testing.T) (dir string, roots []byte) {
	t.Helper()
	rootsDir := t.TempDir()
	if _, err := LoadOrCreateCA(rootsDir); err != nil {
		t.Fatal(err)
	}
	roots, _ = os.ReadFile(filepath.Join(rootsDir, "ca.crt"))
	saved := systemRoots
	systemRoots = []string{filepath.Join(rootsDir, "missing.pem"), filepath.Join(rootsDir, "ca.crt")}
	t.Cleanup(func() { systemRoots = saved })
	dir = t.TempDir()
	if _, err := LoadOrCreateCA(dir); err != nil {
		t.Fatal(err)
	}
	return dir, roots
}

// The bundle is the roots and the authority once, the Java store holds both,
// and a second run changes no file.
func TestEnsureTrust(t *testing.T) {
	dir, roots := useRoots(t)
	if it := TrustItem(dir); it.OK || it.Text == "" {
		t.Fatalf("before: %+v", it)
	}
	if err := EnsureTrust(dir); err != nil {
		t.Fatal(err)
	}
	if it := TrustItem(dir); !it.OK {
		t.Fatalf("after: %+v", it)
	}
	ca, _ := os.ReadFile(filepath.Join(dir, "ca.crt"))
	bundle, _ := os.ReadFile(filepath.Join(TrustDir(dir), "ca-bundle.crt"))
	if string(bundle) != string(roots)+string(ca) {
		t.Fatalf("bundle is not the roots and the authority")
	}
	p12, _ := os.ReadFile(filepath.Join(TrustDir(dir), "truststore.p12"))
	certs, err := pkcs12.DecodeTrustStore(p12, trustStorePassword)
	if err != nil || len(certs) != 2 {
		t.Fatalf("trust store holds %d, %v", len(certs), err)
	}
	if err := EnsureTrust(dir); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(TrustDir(dir), "truststore.p12"))
	if string(again) != string(p12) {
		t.Fatal("a second run rewrote the trust store")
	}
}

// A system that already trusts the authority carries it in its roots, and the
// bundle does not repeat it.
func TestTheBundleCarriesTheAuthorityOnce(t *testing.T) {
	files, _ := trustFiles([]byte("ROOTS\n-----BEGIN CERTIFICATE-----\nAB\nCD\n-----END CERTIFICATE-----\n"),
		[]byte("-----BEGIN CERTIFICATE-----\nABCD\n-----END CERTIFICATE-----\n"))
	if strings.Count(string(files["ca-bundle.crt"]), "BEGIN CERTIFICATE") != 1 {
		t.Fatalf("bundle:\n%s", files["ca-bundle.crt"])
	}
}

// A file rewritten in place keeps its identity, so a bind mount of it reads
// the new content.
func TestWriteInPlaceKeepsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeInPlace(path, []byte("one"))
	before, _ := os.Stat(path)
	writeInPlace(path, []byte("two"))
	after, _ := os.Stat(path)
	if before.Sys().(*syscall.Stat_t).Ino != after.Sys().(*syscall.Stat_t).Ino {
		t.Fatal("the file was replaced")
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Fatalf("content = %q", b)
	}
}

func TestTrustArguments(t *testing.T) {
	dir, _ := useRoots(t)
	s := &Service{Trust: true}
	if args := trustArguments(dir, s); args != nil {
		t.Fatalf("arguments before the store is written: %v", args)
	}
	EnsureTrust(dir)
	joined := strings.Join(trustArguments(dir, s), " ")
	if strings.Contains(joined, TrustDir(dir)+":") {
		t.Fatalf("the store's directory is mounted whole: %s", joined)
	}
	for _, want := range []string{
		filepath.Join(TrustDir(dir), "ca-bundle.crt") + ":/etc/ssl/certs/ca-certificates.crt:ro",
		":/etc/pki/tls/certs/ca-bundle.crt:ro",
		":/etc/ssl/cert.pem:ro",
		filepath.Join(TrustDir(dir), "truststore.p12") + ":/etc/bay/truststore.p12:ro",
		"SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
		"NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt",
		"JAVA_TOOL_OPTIONS=-Djavax.net.ssl.trustStore=/etc/bay/truststore.p12",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("arguments lack %q", want)
		}
	}
	own := &Service{Trust: true, Env: map[string]string{"SSL_CERT_FILE": "/mine"},
		Volumes: []string{"/x:/etc/ssl/cert.pem:ro"}}
	joined = strings.Join(trustArguments(dir, own), " ")
	if strings.Contains(joined, "SSL_CERT_FILE=") || strings.Contains(joined, ":/etc/ssl/cert.pem:ro") {
		t.Fatalf("the service's own variable or mount was overridden: %s", joined)
	}
	if args := trustArguments(dir, &Service{Trust: false}); args != nil {
		t.Fatalf("trust: false gave %v", args)
	}
}

func TestTrustIsOnUnlessTurnedOff(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		want bool
	}{
		{"", true},
		{"\n    x-bay:\n      trust: false", false},
		{"\n    x-bay:\n      trust: true", true},
	} {
		path := filepath.Join(t.TempDir(), "compose.yaml")
		os.WriteFile(path, []byte("name: app\nservices:\n  web:\n    image: nginx"+tc.yaml+"\n"), 0o644)
		cfg, err := LoadIn(NewMachine(t.TempDir()), path)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.Services["web"].Trust; got != tc.want {
			t.Errorf("%q: trust = %v", tc.yaml, got)
		}
	}
}

// A change to the arguments that give a container the store changes its
// configuration identity, so the container is created again with them.
func TestTrustArgumentsArePartOfTheIdentity(t *testing.T) {
	s := &Service{Name: "web", Trust: true}
	none := serviceFingerprint("app", s, "sha256:a", nil)
	some := serviceFingerprint("app", s, "sha256:a", []string{"--volume", "/a:/etc/ssl/cert.pem:ro"})
	other := serviceFingerprint("app", s, "sha256:a", []string{"--volume", "/b:/etc/ssl/cert.pem:ro"})
	if none == some || some == other {
		t.Fatal("the trust arguments do not change the identity")
	}
}
