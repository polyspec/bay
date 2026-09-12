package stack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolverBody(t *testing.T) {
	in := Install{Domains: []string{"test"}, Addr: "127.0.0.1:5354"}
	want := "domain test\nsearch test\nnameserver 127.0.0.1\nport 5354\n"
	if got := in.resolverBody("test"); got != want {
		t.Fatalf("resolverBody() = %q, want %q", got, want)
	}
	if got := (Install{Addr: "bogus"}).resolverBody("dev.test"); got != "" {
		t.Fatalf("resolverBody() on a bad address = %q, want empty", got)
	}
}

// useTempResolverDir points the resolver helpers at a scratch directory so the
// tests never read or write the real /etc/resolver.
func useTempResolverDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := resolverDir
	resolverDir = dir
	t.Cleanup(func() { resolverDir = old })
	return dir
}

func TestResolverPath(t *testing.T) {
	useTempResolverDir(t)
	if got := ResolverPath(".dev.test."); got != filepath.Join(resolverDir, "dev.test") {
		t.Fatalf("ResolverPath() = %q", got)
	}
}

func TestPendingListsMissingSteps(t *testing.T) {
	useTempResolverDir(t)
	in := Install{
		Domains: []string{"definitely-not-installed", "also-not-installed"},
		Addr:    "127.0.0.1:5354",
		CAPath:  "/nonexistent.crt",
	}
	steps := in.Pending()
	if len(steps) != 3 {
		t.Fatalf("Pending() = %v, want a step per domain plus the keychain", steps)
	}
	if !strings.HasSuffix(steps[0].Text, "/definitely-not-installed") {
		t.Errorf("first step = %q", steps[0].Text)
	}
	if !strings.HasSuffix(steps[1].Text, "/also-not-installed") {
		t.Errorf("second step = %q", steps[1].Text)
	}
	if !strings.Contains(steps[2].Text, "keychain") {
		t.Errorf("third step = %q", steps[2].Text)
	}
	// Trusting a certificate is not one of the steps that need root.
	if got := in.privilegedSteps(); len(got) != 2 {
		t.Errorf("privilegedSteps() = %v, want only the two resolver writes", got)
	}
}

// TestCATrustedRejectsUntrusted checks the trust probe against a freshly
// created CA, which by definition is not in the keychain yet.
func TestCATrustedRejectsUntrusted(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if CATrusted(ca.CertPath()) {
		t.Fatal("a brand new CA reported as trusted")
	}
}

func TestQuoting(t *testing.T) {
	if got := shellQuote(`it's`); got != `'it'\''s'` {
		t.Errorf("shellQuote = %s", got)
	}
	if got := appleScriptQuote(`a"b\c`); got != `"a\"b\\c"` {
		t.Errorf("appleScriptQuote = %s", got)
	}
}

func TestBuildPlistIsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.plist")
	body := buildPlist("dev.bay.dns",
		[]string{"/usr/local/bin/bayd", "-domain", "test", "-proxy", "edge & co"},
		filepath.Join(dir, "out.log"), filepath.Join(dir, "err.log"))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint: %v\n%s\n%s", err, out, body)
	}
	if !strings.Contains(body, "edge &amp; co") {
		t.Errorf("argument was not XML-escaped:\n%s", body)
	}
	out, err := exec.Command("plutil", "-extract", "ProgramArguments.4", "raw", "-o", "-", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "edge & co" {
		t.Errorf("round-tripped argument = %q", out)
	}
}

// TestHasTerminalRejectsDevNull covers the case that made the menu bar app's
// setup button do nothing: /dev/null is a character device, so a check that
// stops there sends the app down the sudo path, where it cannot ask for a
// password and fails silently.
func TestHasTerminalRejectsDevNull(t *testing.T) {
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()

	saved := os.Stdin
	os.Stdin = devnull
	t.Cleanup(func() { os.Stdin = saved })

	if hasTerminal() {
		t.Fatal("hasTerminal() said yes with stdin on /dev/null")
	}
}
