package stack

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAProxyThatIsNotThereIsRecognised(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		gone bool
	}{
		{"apple container", "container exec bay-edge nginx -s reload: Error: container with ID bay-edge not found", true},
		{"docker no such container", "docker exec bay-edge nginx -s reload: Error response from daemon: No such container: bay-edge", true},
		{"docker not running", "docker exec bay-edge nginx -s reload: Error response from daemon: Container bay-edge is not running", true},
		{"a configuration nginx rejects", "container exec bay-edge nginx -s reload: nginx: [emerg] unknown directive", false},
		{"no error", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var err error
			if c.text != "" {
				err = errors.New(c.text)
			}
			if got := proxyIsGone(err); got != c.gone {
				t.Errorf("proxyIsGone(%q) = %v, want %v", c.text, got, c.gone)
			}
		})
	}
}

// TestAProxyThatWentAwayIsStartedInsteadOfFailingTheSync covers the machine
// losing its proxy between the moment the container list is read and the moment
// the reload is sent. The engine reports the proxy running against the right
// directories, so nothing is created and a reload is sent; by then the
// container is gone and the engine answers that it does not exist.
//
// The configuration is already written at that point, so failing there leaves
// the machine with a configuration nothing serves, and the command that asked
// reports an error for a machine it has already changed. Starting the proxy is
// what a machine with no proxy needs.
func TestAProxyThatWentAwayIsStartedInsteadOfFailingTheSync(t *testing.T) {
	dir := t.TempDir()
	conf, certs := filepath.Join(dir, "conf.d"), filepath.Join(dir, "certs")
	for _, d := range []string{conf, certs} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	calls := filepath.Join(dir, "calls")
	state := filepath.Join(dir, "proxy.json")

	// ls and inspect report a running proxy mounted on the directories the sync
	// is about to serve, so the proxy is reused rather than replaced. exec
	// answers the way an engine answers for a container that is not there.
	// Once the reload has found nothing, the proxy is gone from the list too,
	// which is what the engine reports for a container that was removed.
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$BAY_CALLS"
case "$1" in
  ls|inspect)
    if [ -e "$BAY_CALLS.gone" ]; then printf '[]\n'; else cat "$BAY_PROXY"; fi ;;
  exec)
    : > "$BAY_CALLS.gone"
    printf 'Error: container with ID %s not found\n' "$2" >&2; exit 1 ;;
  *) : ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "container"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_BIN", filepath.Join(dir, "container"))
	t.Setenv("DOCKER_BIN", filepath.Join(dir, "no-docker"))
	t.Setenv("BAY_CALLS", calls)
	t.Setenv("BAY_PROXY", state)

	body, err := json.Marshal([]map[string]any{{
		"configuration": map[string]any{
			"id":     ProxyName,
			"labels": map[string]string{LabelRole: roleProxy},
			"mounts": []map[string]string{
				{"source": conf, "destination": "/etc/nginx/conf.d"},
				{"source": certs, "destination": "/etc/nginx/certs"},
			},
		},
		"status": map[string]any{
			"state":       "running",
			"startedDate": "2026-09-09T10:00:00Z",
			"networks": []map[string]string{
				{"ipv4Address": "192.168.64.2/24", "ipv4Gateway": "192.168.64.1"},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, body, 0600); err != nil {
		t.Fatal(err)
	}

	action, err := applyProxy(AppleEngine, conf, certs, "")
	if err != nil {
		t.Fatalf("a proxy that went away failed the sync instead of being started: %v", err)
	}
	if action != "started" {
		t.Errorf("action is %q, want started", action)
	}
	ran, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ran), "run --detach --name "+ProxyName) {
		t.Errorf("the proxy was not started again; the engine ran:\n%s", ran)
	}
}

func TestAConfigurationTheProxyRejectsStillFailsTheSync(t *testing.T) {
	dir := t.TempDir()
	conf, certs := filepath.Join(dir, "conf.d"), filepath.Join(dir, "certs")
	for _, d := range []string{conf, certs} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	state := filepath.Join(dir, "proxy.json")
	script := `#!/bin/sh
case "$1" in
  ls|inspect) cat "$BAY_PROXY" ;;
  exec) printf 'nginx: [emerg] unknown directive\n' >&2; exit 1 ;;
  *) : ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "container"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_BIN", filepath.Join(dir, "container"))
	t.Setenv("DOCKER_BIN", filepath.Join(dir, "no-docker"))
	t.Setenv("BAY_PROXY", state)

	body, err := json.Marshal([]map[string]any{{
		"configuration": map[string]any{
			"id":     ProxyName,
			"labels": map[string]string{LabelRole: roleProxy},
			"mounts": []map[string]string{
				{"source": conf, "destination": "/etc/nginx/conf.d"},
				{"source": certs, "destination": "/etc/nginx/certs"},
			},
		},
		"status": map[string]any{
			"state":       "running",
			"startedDate": "2026-09-09T10:00:00Z",
			"networks": []map[string]string{
				{"ipv4Address": "192.168.64.2/24", "ipv4Gateway": "192.168.64.1"},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, body, 0600); err != nil {
		t.Fatal(err)
	}

	// A proxy that is there and refuses the configuration is a fault to report,
	// not a proxy to replace: replacing it would hide the configuration error
	// behind a container that starts and then fails the same way.
	if _, err := applyProxy(AppleEngine, conf, certs, ""); err == nil {
		t.Fatal("a configuration the proxy rejected was reported as applied")
	}
}
