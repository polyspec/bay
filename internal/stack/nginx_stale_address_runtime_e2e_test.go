package stack

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

// A container comes back at a different address every time it is recreated.
// These tests run their own nginx rather than the machine's proxy, so whatever
// the machine is serving keeps serving while they run.
const (
	probeEdge    = "bay-stale-edge"
	probeBackend = "bay-stale-backend"
	probeDomain  = "stale.probe.test"
	// probeDeadline is the longest a single request may take. nginx waits a
	// minute for a connection by default, which is the hang being tested for.
	probeDeadline = 8 * time.Second
	// probeRecovery is the longest the proxy may take to find a container that
	// moved without bay. It is bounded by the runtime's DNS, which
	// answers with the previous address for about fifteen seconds.
	probeRecovery = 40 * time.Second
)

// probe holds what the two tests share.
type probe struct {
	confDir string
	certDir string
	client  *http.Client
	url     string
	edgeIP  string
}

func startProbe(t *testing.T) *probe {
	t.Helper()
	if os.Getenv("BAY_E2E") == "" {
		t.Skip("set BAY_E2E=1 to run tests that start containers")
	}
	t.Cleanup(func() { Remove(probeEdge); Remove(probeBackend) })
	Remove(probeEdge)
	Remove(probeBackend)

	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &probe{
		confDir: filepath.Join(dir, "conf"),
		certDir: filepath.Join(dir, "certs"),
		url:     "https://" + probeDomain + "/",
	}
	for _, name := range []string{probeDomain, DefaultCertName} {
		if _, err := ca.Issue(p.certDir, name); err != nil {
			t.Fatal(err)
		}
	}

	startProbeBackend(t, "before")
	p.render(t, waitRunning(t, probeBackend).IPv4, "first")
	startProbeEdge(t, p.confDir, p.certDir)
	p.edgeIP = waitRunning(t, probeEdge).IPv4
	p.client = probeClient(t, ca.CertPath(), p.edgeIP)
	waitForBody(t, p.client, p.url, "before ")
	return p
}

// render writes the configuration SyncProxy would write for a backend at ip.
func (p *probe) render(t *testing.T, ip, generation string) {
	t.Helper()
	r := Route{Domain: probeDomain, Backend: probeBackend + "." + BackendDomain + ":80", Scheme: "http"}
	if ip != "" {
		r.Address = net.JoinHostPort(ip, "80")
	}
	if err := RenderNginx(p.confDir, []Route{r}, "192.168.64.1", DefaultCertName, generation); err != nil {
		t.Fatal(err)
	}
}

// reload asks the edge to take the configuration and waits until it is the one
// being served. Reloading is asynchronous: the workers being replaced hold the
// listening sockets, so a request sent too early is answered by the old
// configuration. This is what SyncProxy and WaitForGeneration do.
func (p *probe) reload(t *testing.T, generation string) {
	t.Helper()
	if out, err := exec.Command(engineBin(ServiceEngine()), "exec", probeEdge,
		"nginx", "-s", "reload").CombinedOutput(); err != nil {
		t.Fatalf("reloading the edge: %v\n%s", err, out)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + net.JoinHostPort(p.edgeIP, "80") + HealthPath
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := client.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if strings.TrimSpace(string(b)) == generation {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the edge never served the configuration %q", generation)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// recreate replaces the backend and returns its old and new addresses.
func recreate(t *testing.T) (string, string) {
	t.Helper()
	before := waitRunning(t, probeBackend).IPv4
	Remove(probeBackend)
	startProbeBackend(t, "after")
	after := waitRunning(t, probeBackend)
	if before == after.IPv4 {
		t.Skipf("the runtime reused %s, so this test proves nothing", before)
	}
	// Wait for the container itself, so what is measured is the proxy reaching
	// it and not the program starting.
	waitForListener(t, after.IPv4)
	return before, after.IPv4
}

// This is the path a person takes: bay recreates the container and
// writes the configuration for it. The proxy has to reach it at once. Naming
// the backend did not achieve that, because the runtime answers with the
// previous address for about fifteen seconds.
func TestRecreatedServiceIsReachedAtOnceActualRuntime(t *testing.T) {
	p := startProbe(t)
	before, after := recreate(t)
	p.render(t, after, "second")
	p.reload(t, "second")

	start := time.Now()
	body, path := fetch(t, p.client, p.url)
	took := time.Since(start)
	if !strings.HasPrefix(body, "after ") {
		t.Fatalf("the proxy answered %q after the container moved from %s to %s in %s",
			body, before, after, took.Round(time.Millisecond))
	}
	// The configuration was written from the new address, so that is what has
	// to answer. Reaching the container through the fallback would mean the
	// address in the configuration was not used.
	if path != "address" {
		t.Errorf("the request was answered through %q, not the address in the configuration", path)
	}
	if took > probeDeadline {
		t.Errorf("the proxy took %s to reach the container at %s",
			took.Round(time.Millisecond), after)
	}
	t.Logf("reached %s through the %s in %s", after, path, took.Round(time.Millisecond))
}

// A container recreated without bay leaves the configuration holding
// an address nothing answers on. The route falls back to the container's name,
// which the runtime answers correctly once it catches up. What must not happen
// is a request that hangs.
func TestRecreatedBehindBayRecoversActualRuntime(t *testing.T) {
	p := startProbe(t)
	before, after := recreate(t)

	began := time.Now()
	deadline := began.Add(probeRecovery)
	var slowest time.Duration
	for {
		start := time.Now()
		got, path := fetch(t, p.client, p.url)
		if took := time.Since(start); took > slowest {
			slowest = took
		}
		if strings.HasPrefix(got, "after ") {
			t.Logf("recovered through the %s in %s; slowest request %s", path,
				time.Since(began).Round(time.Second), slowest.Round(time.Millisecond))
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the proxy never followed the container from %s to %s; last answer %q",
				before, after, got)
		}
		time.Sleep(time.Second)
	}
	if slowest > probeDeadline {
		t.Errorf("a request waited %s; an address that answers nothing has to fail, not hang",
			slowest.Round(time.Millisecond))
	}
}

// fetch returns the body and the target that answered.
func fetch(t *testing.T, c *http.Client, url string) (string, string) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		return "error: " + err.Error(), ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), resp.Header.Get(RouteHeader)
}

func startProbeBackend(t *testing.T, body string) {
	t.Helper()
	args := append([]string{"run", "--detach", "--name", probeBackend,
		"--network", ProxyNetwork, e2eImage}, e2eServer(body)...)
	if out, err := exec.Command(engineBin(ServiceEngine()), args...).CombinedOutput(); err != nil {
		t.Fatalf("starting the backend: %v\n%s", err, out)
	}
}

func startProbeEdge(t *testing.T, confDir, certDir string) {
	t.Helper()
	out, err := exec.Command(engineBin(ServiceEngine()), "run", "--detach", "--name", probeEdge,
		"--network", ProxyNetwork,
		"--volume", confDir+":/etc/nginx/conf.d:ro",
		"--volume", certDir+":/etc/nginx/certs:ro",
		ProxyImage).CombinedOutput()
	if err != nil {
		t.Fatalf("starting the edge: %v\n%s", err, out)
	}
}

// waitForListener returns once the address accepts a connection on port 80.
func waitForListener(t *testing.T, ip string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), time.Second)
		if err == nil {
			c.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never accepted a connection: %v", ip, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func probeClient(t *testing.T, caPath, edgeIP string) *http.Client {
	t.Helper()
	pem, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("could not parse the CA certificate")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{
		// Longer than the deadline the tests assert, so a slow answer is
		// reported as slow rather than as a client error.
		Timeout: 90 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig:   &tls.Config{RootCAs: pool},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, net.JoinHostPort(edgeIP, "443"))
			},
		},
	}
}
