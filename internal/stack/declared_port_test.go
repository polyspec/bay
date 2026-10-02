package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadCompose(t *testing.T, body string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

// A service that declares no port has none. node in soksakim-hyper-test runs
// `sleep infinity`; port 80 was assumed for it and it was reported as not
// accepting connections on a port it never declared.
func TestAServiceThatDeclaresNoPortHasNone(t *testing.T) {
	cfg, err := loadCompose(t, "services:\n  node:\n    image: node\n    command: [\"sleep\", \"infinity\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services["node"].Port; got != 0 {
		t.Errorf("port %d, want none", got)
	}
}

func TestADeclaredPortIsKept(t *testing.T) {
	cfg, err := loadCompose(t, "services:\n  php:\n    image: php\n    expose: [\"9000\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services["php"].Port; got != 9000 {
		t.Errorf("port %d, want 9000", got)
	}
}

// A service with a domain needs a port to be forwarded to, and one that declares
// none is refused with the service and the domain named.
func TestADomainWithNoDeclaredPortIsRefused(t *testing.T) {
	_, err := loadCompose(t, "services:\n  web:\n    image: node\n    x-bay:\n      domains: [web.test]\n")
	if err == nil {
		t.Fatal("a service with a domain and no port was accepted")
	}
	if !strings.Contains(err.Error(), "web") || !strings.Contains(err.Error(), "web.test") {
		t.Errorf("the refusal does not name the service and the domain: %v", err)
	}
}

// A running service with no port is running; it is not checked for
// connections.
func TestARunningServiceWithNoPortIsRunning(t *testing.T) {
	in := ServiceInstance{State: "running", IPv4: "192.0.2.10"}
	if got := runningState(in, 0); got != "running" {
		t.Errorf("state %q, want running", got)
	}
}

// With a declared port the check stands: nothing listens on this address, so
// the service is running and not accepting connections.
func TestARunningServiceIsCheckedOnItsDeclaredPort(t *testing.T) {
	in := ServiceInstance{State: "running", IPv4: "127.0.0.1", Port: 80}
	if got := runningState(in, 1); got != "starting" {
		t.Errorf("state %q, want the service reported as not accepting connections", got)
	}
}
