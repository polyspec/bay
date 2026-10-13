package stack

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// undeclaredProject declares only the web service. The Compose file that named
// php-web was removed, and its containers are still labelled with the project.
func undeclaredProject(t *testing.T) *Config {
	t.Helper()
	return graceProject(t, "30s")
}

// The containers left by removed services are found from their labels, stopped
// when running and removed, and the messages name each one.
func TestUpRemovesTheContainersOfUndeclaredServices(t *testing.T) {
	cfg := undeclaredProject(t)
	e, f := fakeEngine(t)
	var said []string
	e.progress = func(line string) { said = append(said, line) }
	f.instances["app-php-web"] = Instance{Name: "app-php-web", State: "stopped", Labels: map[string]string{
		LabelRole: roleService, LabelGroup: "app", LabelService: "php-web"}}
	f.instances["app-node-web"] = Instance{Name: "app-node-web", State: "running", Started: "2026-10-03T06:00:00Z", Labels: map[string]string{
		LabelRole: roleService, LabelGroup: "app", LabelService: "node-web", LabelStopGrace: "30"}}
	if err := e.removeUndeclared(cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.instances["app-php-web"]; ok {
		t.Error("the stopped undeclared container was not removed")
	}
	if _, ok := f.instances["app-node-web"]; ok {
		t.Error("the running undeclared container was not removed")
	}
	stop := callIndex(f, []string{"stop", "--time", "30", "app-node-web"})
	remove := callIndex(f, []string{"rm", "--force", "app-node-web"})
	if stop < 0 || remove < 0 || stop > remove {
		t.Fatalf("calls = %q, want the recorded stop of app-node-web before its removal", f.calls)
	}
	if callIndex(f, []string{"stop", "app-php-web"}) >= 0 {
		t.Error("a stopped undeclared container was stopped again")
	}
	for _, name := range []string{"php-web", "node-web"} {
		if !containsLine(said, name) {
			t.Errorf("output %q does not report the removal of %s", said, name)
		}
	}
}

// Declared services, containers of other projects and the proxy are not
// removed, and a second run finds nothing left to remove.
func TestUpLeavesDeclaredAndForeignContainersAlone(t *testing.T) {
	cfg := undeclaredProject(t)
	e, f := fakeEngine(t)
	f.instances["app-web"] = Instance{Name: "app-web", State: "running", Labels: map[string]string{
		LabelRole: roleService, LabelGroup: "app", LabelService: "web"}}
	f.instances["other-api"] = Instance{Name: "other-api", State: "running", Labels: map[string]string{
		LabelRole: roleService, LabelGroup: "other", LabelService: "api"}}
	f.instances["bay-edge"] = Instance{Name: "bay-edge", State: "running", Labels: map[string]string{
		LabelRole: roleProxy, LabelGroup: "app", LabelService: "edge"}}
	if err := e.removeUndeclared(cfg); err != nil {
		t.Fatal(err)
	}
	if len(f.instances) != 3 {
		t.Fatalf("instances = %v, want the declared, foreign and proxy containers kept", f.instances)
	}
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
	if err := e.removeUndeclared(cfg); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0][0] != "ls" {
		t.Fatalf("second run calls = %q, want only the listing", f.calls)
	}
}

// A stop that fails keeps the undeclared container and returns the error.
func TestUpKeepsAnUndeclaredContainerWhoseStopFails(t *testing.T) {
	cfg := undeclaredProject(t)
	e, f := fakeEngine(t)
	f.instances["app-php-web"] = Instance{Name: "app-php-web", State: "running", Labels: map[string]string{
		LabelRole: roleService, LabelGroup: "app", LabelService: "php-web"}}
	want := errors.New("stop refused")
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "stop" {
			return nil, want
		}
		return f.command(ctx, args...)
	}
	if err := e.removeUndeclared(cfg); !errors.Is(err, want) {
		t.Fatalf("error %v, want %v", err, want)
	}
	if _, ok := f.instances["app-php-web"]; !ok {
		t.Fatal("the container was removed after its stop failed")
	}
}

func containsLine(lines []string, service string) bool {
	for _, l := range lines {
		if strings.Contains(l, service) && strings.Contains(l, "removing") {
			return true
		}
	}
	return false
}
