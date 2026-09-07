package stack

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests start real containers. They are the only way to know the routing
// actually works, so they are opt-in rather than skipped silently in CI.
//
// The proxy is a machine singleton, so a test that takes it over disturbs
// whatever is already using it. Rather than quietly tearing down someone's
// working setup, these skip when a proxy belonging to another state directory
// is running.
func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("BAY_E2E") == "" {
		t.Skip("set BAY_E2E=1 to run tests that start containers")
	}
	if os.Getenv("BAY_E2E_FORCE") != "" {
		return
	}
	if _, found, err := Lookup(ProxyName); err == nil && found {
		conf, _, err := ProxyMounts()
		if err == nil && !strings.HasPrefix(conf, os.TempDir()) {
			t.Skipf("%s is running for %s; stop it, or set BAY_E2E_FORCE=1 to take it over",
				ProxyName, filepath.Dir(conf))
		}
	}
}

const e2eImage = "node:26.8.1-trixie-slim"

func e2eServer(body string) []string {
	return []string{"node", "-e",
		fmt.Sprintf("require('http').createServer((q,s)=>s.end(%q+' '+q.headers.host)).listen(80)", body)}
}

// TestTwoGroupsShareOneProxy is the check the group model exists for: two
// groups, each with its own domain, both reachable through the single machine
// proxy, and bringing one down leaving the other untouched.
func TestTwoGroupsShareOneProxy(t *testing.T) {
	requireE2E(t)

	m := NewMachine(t.TempDir())
	alpha := &Service{
		Name: "web", ContainerName: "e2ealpha-web", Image: e2eImage,
		Command: e2eServer("alpha"), Domain: "web.alpha.test", Port: 80, Network: ProxyNetwork,
	}
	beta := &Service{
		Name: "web", ContainerName: "e2ebeta-web", Image: e2eImage,
		Command: e2eServer("beta"), Domain: "web.beta.test", Port: 80, Network: ProxyNetwork,
	}
	t.Cleanup(func() {
		Remove(alpha.ContainerName)
		Remove(beta.ContainerName)
		StopProxy()
	})

	mustRegister(t, m, GroupRef{Name: "e2ealpha", Domains: []string{"alpha.test"}})
	mustRegister(t, m, GroupRef{Name: "e2ebeta", Domains: []string{"beta.test"}})

	if err := StartService("e2ealpha", alpha); err != nil {
		t.Fatal(err)
	}
	if err := StartService("e2ebeta", beta); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, alpha.ContainerName)
	waitRunning(t, beta.ContainerName)

	res, err := SyncProxy(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownRoutes(res.Routes, ".test"); len(ownRoutes(got, "alpha.test")) != 1 ||
		len(ownRoutes(got, "beta.test")) != 1 {
		t.Fatalf("routes = %+v, want one per project", res.Routes)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", res.Conflicts)
	}
	proxy := waitRunning(t, ProxyName)

	// Both groups answer on their own domain, with a certificate our CA signed.
	client := caClient(t, m, proxy.IPv4)
	waitForBody(t, client, "https://web.alpha.test/", "alpha ")
	waitForBody(t, client, "https://web.beta.test/", "beta ")

	// Taking one group down withdraws only its route.
	if err := Remove(beta.ContainerName); err != nil {
		t.Fatal(err)
	}
	if err := m.Unregister("e2ebeta"); err != nil {
		t.Fatal(err)
	}
	res, err = SyncProxy(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownRoutes(res.Routes, "alpha.test"); len(got) != 1 ||
		got[0].Domain != "web.alpha.test" {
		t.Fatalf("routes after taking beta down = %+v", res.Routes)
	}
	if got := ownRoutes(res.Routes, "beta.test"); len(got) != 0 {
		t.Fatalf("beta's route survived: %+v", got)
	}
	if got := get(t, client, "https://web.alpha.test/"); !strings.HasPrefix(got, "alpha ") {
		dumpProxy(t)
		t.Errorf("alpha stopped working after beta went down: %q", got)
	}
	if code := status(t, client, "https://web.beta.test/"); code != http.StatusNotFound {
		t.Errorf("withdrawn route returned %d, want 404", code)
	}

	// The last group leaving removes the proxy entirely.
	if err := Remove(alpha.ContainerName); err != nil {
		t.Fatal(err)
	}
	if err := m.Unregister("e2ealpha"); err != nil {
		t.Fatal(err)
	}
	if res, err = SyncProxy(m); err != nil {
		t.Fatal(err)
	}
	if got := ownRoutes(res.Routes, ".test"); len(ownRoutes(got, "alpha.test")) != 0 {
		t.Fatalf("alpha's route survived: %+v", got)
	}
	// The proxy is removed only when no route remains anywhere on the machine,
	// so this is asserted only on an otherwise idle machine.
	if len(res.Routes) == 0 && res.Action != "stopped" {
		t.Fatalf("action = %q with no routes left, want stopped", res.Action)
	}
}

// TestServiceRestartKeepsRoute covers the reason backends are named rather than
// numbered: the runtime hands out a new address on every start.
func TestServiceRestartKeepsRoute(t *testing.T) {
	requireE2E(t)

	m := NewMachine(t.TempDir())
	svc := &Service{
		Name: "web", ContainerName: "e2ekeep-web", Image: e2eImage,
		Command: e2eServer("before"), Domain: "web.keep.test", Port: 80, Network: ProxyNetwork,
	}
	t.Cleanup(func() { Remove(svc.ContainerName); StopProxy() })

	mustRegister(t, m, GroupRef{Name: "e2ekeep", Domains: []string{"keep.test"}})
	if err := StartService("e2ekeep", svc); err != nil {
		t.Fatal(err)
	}
	first := waitRunning(t, svc.ContainerName)
	if _, err := SyncProxy(m); err != nil {
		t.Fatal(err)
	}
	proxy := waitRunning(t, ProxyName)
	client := caClient(t, m, proxy.IPv4)
	waitForBody(t, client, "https://web.keep.test/", "before ")

	// Recreate the container without touching the proxy configuration.
	svc.Command = e2eServer("after")
	if err := StartService("e2ekeep", svc); err != nil {
		t.Fatal(err)
	}
	second := waitRunning(t, svc.ContainerName)
	if first.IPv4 == second.IPv4 {
		t.Skipf("the runtime reused %s, so this test proves nothing", first.IPv4)
	}

	waitForBody(t, client, "https://web.keep.test/", "after ")
}

// waitForBody polls until the response body starts with want. A container that
// the runtime reports as running has not necessarily bound its port yet, and
// the proxy answers 502 until it has, so a single request proves nothing.
func waitForBody(t *testing.T, c *http.Client, url, want string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	var got string
	for {
		if got = get(t, c, url); strings.HasPrefix(got, want) {
			return
		}
		if time.Now().After(deadline) {
			dumpProxy(t)
			t.Fatalf("%s never returned a body starting with %q; last was %q", url, want, got)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// dumpProxy prints what the proxy is actually serving, for when an assertion
// about routing fails and the reason is not obvious from the response alone.
func dumpProxy(t *testing.T) {
	t.Helper()
	conf, err := exec.Command("container", "exec", ProxyName,
		"cat", "/etc/nginx/conf.d/stack.conf").CombinedOutput()
	t.Logf("conf as the proxy sees it (err=%v):\n%s", err, conf)
	logs, err := exec.Command("container", "logs", ProxyName).CombinedOutput()
	lines := strings.Split(strings.TrimSpace(string(logs)), "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	t.Logf("proxy logs (err=%v):\n%s", err, strings.Join(lines, "\n"))
}

// ownRoutes returns the routes under suffix. The proxy is shared with whatever
// else the machine is running, so a test asserts about its own domains only.
func ownRoutes(routes []Route, suffix string) []Route {
	var out []Route
	for _, r := range routes {
		if strings.HasSuffix(r.Domain, suffix) {
			out = append(out, r)
		}
	}
	return out
}

func mustRegister(t *testing.T, m *Machine, g GroupRef) {
	t.Helper()
	if err := m.Register(g); err != nil {
		t.Fatal(err)
	}
}

func waitRunning(t *testing.T, name string) Instance {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		in, ok, err := Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if ok && in.State == "running" && in.IPv4 != "" {
			return in
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not start within 60s", name)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// caClient trusts only our CA and sends every request to the proxy, which is
// how the routing is exercised without depending on the machine's DNS setup.
func caClient(t *testing.T, m *Machine, proxyIP string) *http.Client {
	t.Helper()
	pem, err := os.ReadFile(filepath.Join(m.Dir, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("could not parse the CA certificate")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// nginx keeps the pre-reload workers alive for existing keep-alive
			// connections, so reusing one would test the configuration that was
			// just replaced rather than the new one.
			DisableKeepAlives: true,
			TLSClientConfig:   &tls.Config{RootCAs: pool},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, net.JoinHostPort(proxyIP, "443"))
			},
		},
	}
}

func get(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func status(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		dumpProxy(t)
		t.Fatalf("%s: %v", url, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestStoppingOneServiceWithdrawsOnlyItsRoute covers the service-level
// controls: a stopped container keeps its filesystem but loses its route, and
// starting it again brings the route back without disturbing its neighbour.
func TestStoppingOneServiceWithdrawsOnlyItsRoute(t *testing.T) {
	requireE2E(t)

	m := NewMachine(t.TempDir())
	one := &Service{
		Name: "one", ContainerName: "e2esvc-one", Image: e2eImage,
		Command: e2eServer("one"), Domain: "one.svc.test", Port: 80, Network: ProxyNetwork,
	}
	two := &Service{
		Name: "two", ContainerName: "e2esvc-two", Image: e2eImage,
		Command: e2eServer("two"), Domain: "two.svc.test", Port: 80, Network: ProxyNetwork,
	}
	t.Cleanup(func() { Remove(one.ContainerName); Remove(two.ContainerName); StopProxy() })

	mustRegister(t, m, GroupRef{Name: "e2esvc", Domains: []string{"svc.test"}})
	for _, s := range []*Service{one, two} {
		if err := StartService("e2esvc", s); err != nil {
			t.Fatal(err)
		}
		waitRunning(t, s.ContainerName)
	}
	if _, err := SyncProxy(m); err != nil {
		t.Fatal(err)
	}
	proxy := waitRunning(t, ProxyName)
	client := caClient(t, m, proxy.IPv4)
	waitForBody(t, client, "https://one.svc.test/", "one ")
	waitForBody(t, client, "https://two.svc.test/", "two ")

	// Stopping one service withdraws its route and leaves the other alone.
	if err := StopContainer(two.ContainerName); err != nil {
		t.Fatal(err)
	}
	res, err := SyncProxy(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownRoutes(res.Routes, ".svc.test"); len(got) != 1 ||
		got[0].Domain != "one.svc.test" {
		t.Fatalf("routes after stopping two = %+v", res.Routes)
	}
	if code := status(t, client, "https://two.svc.test/"); code != http.StatusNotFound {
		t.Errorf("stopped service returned %d, want 404", code)
	}
	if got := get(t, client, "https://one.svc.test/"); !strings.HasPrefix(got, "one ") {
		dumpProxy(t)
		t.Errorf("one broke when two stopped: %q", got)
	}

	// The container survived, so starting it restores the route.
	if err := StartContainer(two.ContainerName); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, two.ContainerName)
	if res, err = SyncProxy(m); err != nil {
		t.Fatal(err)
	}
	if got := ownRoutes(res.Routes, ".svc.test"); len(got) != 2 {
		t.Fatalf("routes after starting two = %+v", res.Routes)
	}
	waitForBody(t, client, "https://two.svc.test/", "two ")
}

// TestLogsReachTheCaller checks the log passthrough, which the web UI will use
// as well as the CLI.
func TestLogsReachTheCaller(t *testing.T) {
	requireE2E(t)

	svc := &Service{
		Name: "log", ContainerName: "e2elog-svc", Image: e2eImage,
		Command: []string{"node", "-e", "console.log('hello-from-logs');setTimeout(()=>{},2000)"},
		Domain:  "log.logs.test", Port: 80, Network: ProxyNetwork,
	}
	t.Cleanup(func() { Remove(svc.ContainerName) })

	if err := StartService("e2elog", svc); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, svc.ContainerName)

	deadline := time.Now().Add(30 * time.Second)
	for {
		var buf strings.Builder
		err := Logs(svc.ContainerName, false, 0, &buf)
		if strings.Contains(buf.String(), "hello-from-logs") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("log line never appeared (err=%v):\n%s", err, buf.String())
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// TestSnapshotReflectsReality is the contract every front end depends on: what
// Take reports has to match what is actually running.
func TestSnapshotReflectsReality(t *testing.T) {
	requireE2E(t)

	dir := t.TempDir()
	m := NewMachine(dir)
	stackPath := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(stackPath, []byte(`name: e2esnap
x-bay:
  domain: snap.test
services:
  up:
    image: `+e2eImage+`
    command: ["node","-e","require('http').createServer((q,s)=>s.end('up')).listen(80)"]
  down:
    image: `+e2eImage+`
    command: ["node","-e","require('http').createServer((q,s)=>s.end('down')).listen(80)"]
  work:
    image: `+e2eImage+`
    command: ["node","-e","setInterval(()=>{},1000)"]
    labels:
      bay.internal: "true"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(stackPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, s := range cfg.Sorted() {
			Remove(s.ContainerName)
		}
		StopProxy()
	})
	mustRegister(t, m, cfg.Ref())

	// Only one of the two services is started, so the snapshot has to
	// distinguish "absent" from "running" rather than assume the stack file.
	started := cfg.Services["up"]
	if err := StartService(cfg.Name, started); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, started.ContainerName)
	if _, err := SyncProxy(m); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, ProxyName)

	snap, err := Take(m, DefaultDNSAddr)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Machine.Proxy.State != "running" || snap.Machine.Proxy.Routes < 1 {
		t.Errorf("proxy status = %+v", snap.Machine.Proxy)
	}
	if snap.Machine.Proxy.Generation == "" {
		t.Error("proxy generation is empty; the health endpoint was not reachable")
	}
	var g GroupStatus
	for _, candidate := range snap.Groups {
		if candidate.Name == "e2esnap" {
			g = candidate
		}
	}
	if g.Name == "" {
		t.Fatalf("groups = %+v, want one named e2esnap", snap.Groups)
	}
	if g.Error != "" {
		t.Fatalf("group error: %s", g.Error)
	}
	if len(g.Services) != 3 {
		t.Fatalf("services = %+v, want the started, the absent and the internal one", g.Services)
	}
	byName := map[string]ServiceStatus{}
	for _, s := range g.Services {
		byName[s.Name] = s
	}
	if s := byName["up"]; s.State != "running" || !s.Routed || s.IPv4 == "" || s.URL != "https://up.snap.test/" {
		t.Errorf("started service = %+v", s)
	}
	if s := byName["down"]; s.State != "absent" || s.Routed {
		t.Errorf("never-started service = %+v", s)
	}
	// An internal service has no domain, no URL and no route, but it does have
	// an address other services can reach it by.
	if s := byName["work"]; !s.Internal || s.Domain != "" || s.URL != "" || s.Routed {
		t.Errorf("internal service = %+v", s)
	} else if s.Address != "e2esnap-work."+BackendDomain+":80" {
		t.Errorf("internal service address = %q", s.Address)
	}
}

// TestUpRefusesADomainAnotherProjectServes covers the failure that removed a
// running project's route: a second project claiming the same domain used to
// take it and leave a warning.
func TestUpRefusesADomainAnotherProjectServes(t *testing.T) {
	requireE2E(t)

	m := NewMachine(t.TempDir())
	rt := &Runtime{Machine: m, Addr: DefaultDNSAddr}

	holder := &Service{
		Name: "web", ContainerName: "e2ehold-web", Image: e2eImage,
		Command: e2eServer("holder"), Domain: "web.hold.test", Port: 80, Network: ProxyNetwork,
	}
	t.Cleanup(func() { Remove(holder.ContainerName); StopProxy() })

	mustRegister(t, m, GroupRef{Name: "e2ehold", Domains: []string{"hold.test"}})
	if err := StartService("e2ehold", holder); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, holder.ContainerName)
	if _, err := SyncProxy(m); err != nil {
		t.Fatal(err)
	}

	// A second project claiming the same domain is refused before it starts.
	dir := filepath.Join(t.TempDir(), "e2etake")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(compose, []byte(`name: e2etake
x-bay:
  domain: hold.test
services:
  web:
    image: `+e2eImage+`
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(compose)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.Up(cfg)
	if err == nil {
		Remove("e2etake-web")
		t.Fatal("the second project was allowed to claim a served domain")
	}
	if !strings.Contains(err.Error(), "e2ehold") {
		t.Fatalf("error does not name the project holding the domain: %v", err)
	}
	if _, found, _ := Lookup("e2etake-web"); found {
		Remove("e2etake-web")
		t.Fatal("a container was created despite the refusal")
	}

	// The holder's route is untouched.
	routes, _, err := Routes()
	if err != nil {
		t.Fatal(err)
	}
	want := holder.ContainerName + "." + BackendDomain + ":80"
	if got := ownRoutes(routes, "hold.test"); len(got) != 1 || got[0].Backend != want {
		t.Fatalf("routes = %+v, want the holder's route only", got)
	}
}

// TestStartingIsDistinctFromRunning covers the window a container is running
// but the process inside is not listening. The proxy returns 502 during it, so
// the state has to be reported separately.
func TestStartingIsDistinctFromRunning(t *testing.T) {
	requireE2E(t)

	dir := t.TempDir()
	m := NewMachine(dir)
	// The process listens after a delay, so the container is running well
	// before it accepts a connection.
	svc := &Service{
		Name: "late", ContainerName: "e2elate-web", Image: e2eImage,
		Command: []string{"node", "-e",
			"setTimeout(()=>require('http').createServer((q,s)=>s.end('late')).listen(80),20000)"},
		Domain: "late.start.test", Port: 80, Network: ProxyNetwork,
	}
	t.Cleanup(func() { Remove(svc.ContainerName); StopProxy() })

	mustRegister(t, m, GroupRef{Name: "e2elate", Domains: []string{"start.test"}})
	if err := StartService("e2elate", svc); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, svc.ContainerName)

	// The container is running and has an address, and is not ready.
	instances, err := Instances()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, in := range instances {
		if in.Container != svc.ContainerName {
			continue
		}
		found = true
		if !in.Running() {
			t.Fatalf("container state = %q, want running", in.State)
		}
		if in.Ready() {
			t.Fatal("a container that is not listening reported as ready")
		}
	}
	if !found {
		t.Fatal("the container is not listed")
	}

	// The snapshot reports it as starting, not running.
	snap, err := Take(m, DefaultDNSAddr)
	if err != nil {
		t.Fatal(err)
	}
	var status ServiceStatus
	for _, g := range snap.Groups {
		for _, s := range g.Services {
			if s.Container == svc.ContainerName {
				status = s
			}
		}
	}
	if status.State != "starting" {
		t.Fatalf("state = %q, want starting", status.State)
	}
	if status.Running() {
		t.Error("a starting service reported as running")
	}
	if !status.Live() {
		t.Error("a starting service reported as not live")
	}

	// WaitReady reports it as pending rather than blocking until it listens.
	pending, err := WaitReady([]string{svc.ContainerName}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0] != svc.ContainerName {
		t.Fatalf("pending = %v, want the container", pending)
	}

	// Once the process listens, the same container reports ready.
	if pending, err := WaitReady([]string{svc.ContainerName}, 40*time.Second); err != nil {
		t.Fatal(err)
	} else if len(pending) != 0 {
		t.Fatalf("still not ready after the process listens: %v", pending)
	}
}
