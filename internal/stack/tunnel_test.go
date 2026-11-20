package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tunnelConfig(t *testing.T) *Config {
	t.Helper()
	return loadTunnelConfig(t, "proj")
}

func loadTunnelConfig(t *testing.T, name string) *Config {
	t.Helper()
	cfg, err := Load(write(t, "name: "+name+"\nx-bay:\n  domain: shop.test\nservices:\n  web:\n    image: nginx\n    ports: [\"3000\"]\n    x-bay:\n      domains: [web.shop.test]\n"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestTunnelRouteNamesWhatItForwardsTo(t *testing.T) {
	cfg := tunnelConfig(t)
	route, service, err := tunnelRoute(cfg, "web.shop.test", []Route{{
		Domain: "web.shop.test", Backend: "proj-web.bay.test:3000", Scheme: "http", Engine: AppleEngine,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if route.Backend != "proj-web.bay.test:3000" || service.ContainerName != "proj-web" {
		t.Fatalf("route forwards to %q of %q, want proj-web.bay.test:3000 of proj-web", route.Backend, service.ContainerName)
	}
}

// The domain must be one the proxy serves for this project. A domain with no
// route, a domain lost to a conflict and a domain served by another project's
// container are all refused with the domain named.
func TestTunnelRefusesWhatTheProxyDoesNotServe(t *testing.T) {
	cfg := tunnelConfig(t)
	cases := []struct {
		name      string
		routes    []Route
		conflicts []DomainConflict
		want      string
	}{
		{"no route", nil, nil, "serves no route"},
		{"conflict", nil, []DomainConflict{{Domain: "web.shop.test"}}, "claimed by more than one container"},
		{"another project", []Route{{Domain: "web.shop.test", Backend: "other-web.bay.test:3000", Engine: AppleEngine}}, nil,
			"not a container of this project"},
	}
	for _, c := range cases {
		_, _, err := tunnelRoute(cfg, "web.shop.test", c.routes, c.conflicts)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error is %v, want it to say %q", c.name, err, c.want)
		}
	}
}

// tunnelWebContainer is the one entry of the apple list the stand-in engine
// answers with: a running service container that claims web.shop.test on port
// 3000.
const tunnelWebContainer = `{"configuration":{"id":"proj-web","labels":{
  "bay.role":"service","bay.group":"proj","bay.service":"web",
  "bay.domain":"web.shop.test","bay.port":"3000","bay.scheme":"http"},
  "image":{"descriptor":{"digest":"sha256:abc"}}},
  "status":{"state":"running","startedDate":"2026-11-20T09:00:01Z",
  "networks":[{"ipv4Address":"192.168.64.9/24","ipv4Gateway":"192.168.64.1"}]}}`

// tunnelEngine stands in for the apple engine. It keeps every command in log,
// answers the container list with the web container plus the publication's
// container once it is created, and ends the followed output after the public
// address appears, as the client's own end does. When fromStart is set, the
// publication's container is already running when the command begins.
func tunnelEngine(t *testing.T, fromStart bool) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "engine.log")
	dir := t.TempDir()
	bin := filepath.Join(dir, "container")
	tunnel := `{"configuration":{"id":"proj-tunnel","labels":{
  "bay.role":"service","bay.group":"proj","bay.service":"tunnel","bay.domain":""},
  "image":{"descriptor":{"digest":"sha256:def"}}},
  "status":{"state":"running","networks":[{"ipv4Address":"192.168.64.10/24"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "web.json"), []byte("["+tunnelWebContainer+"]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "both.json"), []byte("["+tunnelWebContainer+","+tunnel+"]"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "created")
	if fromStart {
		os.WriteFile(state, nil, 0o600)
	}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + log + "'\n" +
		"if [ \"$1\" = ls ]; then" +
		" if [ -f '" + state + "' ]; then cat '" + filepath.Join(dir, "both.json") + "';" +
		" else cat '" + filepath.Join(dir, "web.json") + "'; fi; exit 0; fi\n" +
		"if [ \"$1\" = create ]; then : > '" + state + "'; exit 0; fi\n" +
		"if [ \"$1\" = rm ]; then rm -f '" + state + "'; exit 0; fi\n" +
		"if [ \"$1\" = image ]; then exit 0; fi\n" +
		"if [ \"$1\" = logs ]; then" +
		" printf '%s\\n' 'INF |  https://quiet-moments-turn.trycloudflare.com |'" +
		" printf '%s\\n' 'INF Registered tunnel connection'; exit 0; fi\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_BIN", bin)
	return log
}

// The publication runs the tunnel client in a container of the project,
// forwards to the service's own address, prints the public address the client
// names, and ends by removing its container when the client ends.
func TestTunnelPublishesAndServeUntilTheClientEnds(t *testing.T) {
	log := tunnelEngine(t, false)
	dir := t.TempDir()
	if _, err := LoadOrCreateCA(dir); err != nil {
		t.Fatal(err)
	}
	cfg := tunnelConfig(t)

	var out strings.Builder
	rt := &Runtime{Machine: &Machine{Dir: dir}}
	if err := rt.Tunnel(cfg, "web.shop.test", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "https://quiet-moments-turn.trycloudflare.com") {
		t.Errorf("output is %q, want the public address", out.String())
	}
	asked, _ := os.ReadFile(log)
	askedFor := string(asked)
	for _, want := range []string{
		"create --name proj-tunnel",
		"--label bay.service=tunnel",
		"--url http://proj-web.bay.test:3000",
		TunnelImage,
		"start proj-tunnel",
		"stop proj-tunnel",
		"rm --force proj-tunnel",
	} {
		if !strings.Contains(askedFor, want) {
			t.Errorf("the engine was asked:\n%s\nwant it to include %q", askedFor, want)
		}
	}
}

// A publication that is already running is refused: two clients would publish
// one domain twice.
func TestTunnelRefusesASecondPublication(t *testing.T) {
	tunnelEngine(t, true)
	dir := t.TempDir()
	if _, err := LoadOrCreateCA(dir); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := &Runtime{Machine: &Machine{Dir: dir}}
	err := rt.Tunnel(tunnelConfig(t), "web.shop.test", &out)
	if err == nil || !strings.Contains(err.Error(), "end the publication it serves first") {
		t.Fatalf("error is %v, want it to refuse the running publication", err)
	}
}
