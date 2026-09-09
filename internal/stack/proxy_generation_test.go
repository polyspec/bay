package stack

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackendRestartWaitsForCurrentProxyGeneration(t *testing.T) {
	for _, change := range []struct{ name, address, started string }{
		{"address changed", "192.168.64.21", "2026-09-08T12:00:00Z"},
		{"restarted at the same address", "192.168.64.20", "2026-09-08T12:01:00Z"},
	} {
		t.Run(change.name, func(t *testing.T) {
			routes := generationContainers(t)
			before := routes("192.168.64.20", "2026-09-08T12:00:00Z")
			after := routes(change.address, change.started)
			oldGeneration, generation := routeGeneration(before), routeGeneration(after)
			if before[0].Backend != after[0].Backend || after[0].Backend != "alpha-web.container.test:8080" {
				t.Fatal("restart must retain the named backend")
			}
			requests := 0
			client := &http.Client{Transport: generationTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != "http://127.0.0.1:80"+HealthPath {
					t.Fatal("generation wait requested an unexpected endpoint")
				}
				requests++
				body := oldGeneration
				if requests > 1 {
					body = generation
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body + "\n")), Header: http.Header{}}, nil
			})}
			if err := waitForGeneration(AppleEngine, generation, time.Second, client); err != nil {
				t.Fatal(err)
			}
			if requests != 2 {
				t.Fatal("restart completed on a worker serving the previous backend instance")
			}
		})
	}
}

func TestUnchangedBackendRetainsProxyGeneration(t *testing.T) {
	routes := generationContainers(t)
	first := routeGeneration(routes("192.168.64.20", "2026-09-08T12:00:00Z"))
	second := routeGeneration(routes("192.168.64.20", "2026-09-08T12:00:00Z"))
	if first == "" || first != second {
		t.Fatal("unchanged route and instance metadata changed the generation")
	}
}

type generationTransport func(*http.Request) (*http.Response, error)

func (f generationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func generationContainers(t *testing.T) func(string, string) []Route {
	t.Helper()
	directory := t.TempDir()
	binary, state := filepath.Join(directory, "container"), filepath.Join(directory, "instances.json")
	script := `#!/bin/sh
if [ "$*" != 'ls --all --format json' ]; then exit 71; fi
cat "$BAY_GENERATION_INSTANCES"
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_BIN", binary)
	// The fixture is the only engine this test has: naming a command that does
	// not run leaves the other contributing nothing.
	t.Setenv("DOCKER_BIN", filepath.Join(directory, "no-docker"))
	t.Setenv("BAY_GENERATION_INSTANCES", state)
	return func(address, started string) []Route {
		t.Helper()
		instance := func(name, ip, since string, labels map[string]string) map[string]any {
			return map[string]any{
				"configuration": map[string]any{"id": name, "labels": labels},
				"status": map[string]any{"state": "running", "startedDate": since,
					"networks": []map[string]string{{"ipv4Address": ip + "/24"}}},
			}
		}
		body, err := json.Marshal([]map[string]any{
			instance(ProxyName, "127.0.0.1", "2026-09-08T10:00:00Z", map[string]string{LabelRole: roleProxy}),
			instance("alpha-web", address, started, map[string]string{LabelRole: roleService, LabelGroup: "alpha", LabelService: "web", LabelDomain: "web.alpha.test", LabelPort: "8080", LabelScheme: "http"}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(state, body, 0600); err != nil {
			t.Fatal(err)
		}
		routes, conflicts, err := Routes()
		if err != nil || len(routes) != 1 || len(conflicts) != 0 {
			t.Fatalf("could not read the isolated routing fixture: %v", err)
		}
		return routes
	}
}
