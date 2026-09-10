package stack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// link is both ends of a peer's link, each running this program's own
// configuration, and the proxy published on the loopback address. origin is the
// command the far end sends to, which is what a peer's own container is.
type link struct {
	t      *testing.T
	engine string
	bin    string
	port   string
	domain string
	dir    string
	edge   string
	run    func(...string) ([]byte, error)
}

// startLink stands up an origin running originCmd, the far end of the link, and
// the proxy with a route to it. It returns once a request over the link is
// answered.
func startLink(t *testing.T, originConf string, originCmd ...string) *link {
	t.Helper()
	if os.Getenv("BAY_E2E") == "" {
		t.Skip("set BAY_E2E=1 to start containers")
	}
	engine := ServiceEngine()
	bin := engineBin(engine)
	if bin == "" {
		t.Skipf("%s is not installed", engine)
	}

	const domain = "peerbody.test"
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	l := &link{t: t, engine: engine, bin: bin, port: "18443", domain: domain,
		dir: t.TempDir(), edge: "bay-test-edge-" + stamp}
	network := "bay-test-" + stamp
	peerName := "bay-test-peer-" + stamp
	originName := "bay-test-origin-" + stamp
	l.run = func(args ...string) ([]byte, error) {
		return exec.Command(bin, args...).CombinedOutput()
	}
	t.Cleanup(func() {
		l.run("rm", "--force", l.edge)
		l.run("rm", "--force", peerName)
		l.run("rm", "--force", originName)
		l.run("network", "rm", network)
	})
	if out, err := l.run("network", "create", network); err != nil {
		t.Skipf("cannot create a network: %v\n%s", err, out)
	}

	confDir := filepath.Join(l.dir, "conf")
	certDir := filepath.Join(l.dir, "certs")
	peerDir := filepath.Join(l.dir, "peers")
	peerConf := filepath.Join(l.dir, "peerconf")
	originConfDir := filepath.Join(l.dir, "originconf")
	data := filepath.Join(l.dir, "data")
	for _, d := range []string{confDir, certDir, peerDir, peerConf, originConfDir, data} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ca, err := LoadOrCreateCA(l.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{DefaultCertName, domain} {
		if _, err := ca.Issue(certDir, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := ca.IssueClient(peerDir, "tester"); err != nil {
		t.Fatal(err)
	}
	caPEM, err := os.ReadFile(ca.CertPath())
	if err != nil {
		t.Fatal(err)
	}
	peer := Peer{Name: "peer", Address: peerName + ":" + strconv.Itoa(PeerPort),
		Domains: []string{domain}, CA: string(caPEM)}
	if err := ApprovePeer(l.dir, peer); err != nil {
		t.Fatal(err)
	}
	peers, err := Peers(l.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePeerAuthorities(l.dir, peers); err != nil {
		t.Fatal(err)
	}

	if originConf != "" {
		if err := os.WriteFile(filepath.Join(originConfDir, "origin.conf"),
			[]byte(originConf), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	args := append([]string{"run", "--detach", "--name", originName, "--network", network,
		"--volume", originConfDir + ":/etc/nginx/conf.d:ro",
		"--volume", data + ":/data:ro", ProxyImage}, originCmd...)
	if out, err := l.run(args...); err != nil {
		t.Fatalf("the origin did not start: %v\n%s", err, out)
	}

	gateway, _ := NetworkGateway(engine)
	if err := RenderNginxConfig(peerConf, NginxConfig{
		Routes: []Route{{Domain: domain, Address: originName + ":80",
			Backend: originName + ":80", Scheme: "http"}},
		Resolver:        engineResolver(engine, gateway),
		DefaultCert:     DefaultCertName,
		Generation:      "far",
		PeerPort:        PeerPort,
		PeerAuthorities: "/etc/nginx/peers/" + PeerAuthoritiesName,
		ClientCert:      "/etc/nginx/peers/" + ClientCertName + ".crt",
		ClientKey:       "/etc/nginx/peers/" + ClientCertName + ".key",
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := l.run("run", "--detach", "--name", peerName, "--network", network,
		"--volume", peerConf+":/etc/nginx/conf.d:ro",
		"--volume", certDir+":/etc/nginx/certs:ro",
		"--volume", peerDir+":/etc/nginx/peers:ro", ProxyImage); err != nil {
		t.Fatalf("the far end did not start: %v\n%s", err, out)
	}

	if err := RenderNginxConfig(confDir, NginxConfig{
		DefaultCert: DefaultCertName,
		Generation:  "gen",
		LinkLog:     true,
		PeerRoutes: []PeerRoute{{Domain: domain, Address: peer.Address,
			Authority: "/etc/nginx/peers/" + peers[0].Fingerprint + ".crt"}},
		ClientCert: "/etc/nginx/peers/" + ClientCertName + ".crt",
		ClientKey:  "/etc/nginx/peers/" + ClientCertName + ".key",
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := l.run("run", "--detach", "--name", l.edge, "--network", network,
		"--publish", "127.0.0.1:"+l.port+":443",
		"--volume", confDir+":/etc/nginx/conf.d:ro",
		"--volume", certDir+":/etc/nginx/certs:ro",
		"--volume", peerDir+":/etc/nginx/peers:ro", ProxyImage); err != nil {
		t.Fatalf("the proxy did not start: %v\n%s", err, out)
	}
	return l
}

// get asks for a path over the link and returns the body that arrived.
func (l *link) get(path string) []byte {
	l.t.Helper()
	out, err := exec.Command("curl", "-sk", "--max-time", "60",
		"--resolve", l.domain+":"+l.port+":127.0.0.1",
		"https://"+l.domain+":"+l.port+path).Output()
	if err != nil {
		// curl reports a body that ended early, and the body it did receive is
		// what the record is checked against.
		return out
	}
	return out
}

// waitReady asks for a path until the link answers it.
func (l *link) waitReady(path string, want int) {
	l.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, _ := exec.Command("curl", "-sk", "--max-time", "5",
			"--resolve", l.domain+":"+l.port+":127.0.0.1",
			"-o", "/dev/null", "-w", "%{http_code}",
			"https://"+l.domain+":"+l.port+path).Output()
		if strings.TrimSpace(string(out)) == strconv.Itoa(want) {
			return
		}
		if time.Now().After(deadline) {
			logs, _ := l.run("logs", l.edge)
			l.t.Fatalf("the link did not answer %s within 30s: %s\n%s", path, out, logs)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (l *link) records() string {
	l.t.Helper()
	out, _ := l.run("logs", l.edge)
	return string(out)
}

// A response that crosses a peer's link arrives whole and is recorded as
// complete. The proxy answers 200 whether or not the far end finished, so the
// status does not report the difference and the record has to.
func TestAPeerRouteDeliversTheWholeBodyActualRuntime(t *testing.T) {
	l := startLink(t, "server {\n    listen 80 default_server;\n    location / { root /data; }\n}\n")
	// A body larger than the proxy's buffers, so it is streamed.
	body := strings.Repeat("bay.", (2<<20)/13)
	if err := os.WriteFile(filepath.Join(l.dir, "data", "body.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	l.waitReady("/body.txt", 200)

	for i := 1; i <= 5; i++ {
		if got := len(l.get("/body.txt")); got != len(body) {
			t.Fatalf("request %d received %d bytes of %d:\n%s", i, got, len(body), l.records())
		}
	}
	// Nothing was lost, so the length the far end declared and the length that
	// reached the client are the same number.
	var found string
	for _, line := range strings.Split(l.records(), "\n") {
		if strings.HasPrefix(line, "link ") && strings.Contains(line, "uri=/body.txt") {
			found = line
		}
	}
	whole := strconv.Itoa(len(body))
	if !strings.Contains(found, "declared="+whole) || !strings.Contains(found, "sent="+whole) {
		t.Fatalf("a whole response was not recorded as whole:\n%s", found)
	}
}

// A far end that declares a length and sends less is answered 200 by the proxy.
// nginx reports the request itself as complete, so that is not what tells the
// two apart. Two things do: the length the far end declared beside the length
// that reached the client, both counted in body bytes, and the error the proxy
// writes for that request. The far end here always sends less, so the result
// does not depend on when anything is stopped.
func TestAPeerRouteRecordsAResponseCutShortActualRuntime(t *testing.T) {
	const declared, sent = 100000, 5
	short := fmt.Sprintf(
		`while true; do printf 'HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\nSHORT' | nc -l -p 80; done`,
		declared)
	l := startLink(t, "", "sh", "-c", short)
	l.waitReady("/short", 200)

	if got := len(l.get("/short")); got >= declared {
		t.Fatalf("the far end sent %d bytes, so nothing was cut short", got)
	}
	var found string
	for _, line := range strings.Split(l.records(), "\n") {
		if strings.HasPrefix(line, "link ") && strings.Contains(line, "uri=/short") {
			found = line
		}
	}
	if found == "" {
		t.Fatalf("the request was not recorded:\n%s", l.records())
	}
	if !strings.Contains(found, "declared="+strconv.Itoa(declared)) {
		t.Errorf("the record does not state the length the far end declared: %s", found)
	}
	if !strings.Contains(found, "sent="+strconv.Itoa(sent)) {
		t.Errorf("the record does not state what reached the client: %s", found)
	}
	// The proxy answered 200, so the error it wrote for this request is what
	// reports the loss when the far end declares no length at all.
	if !strings.Contains(l.records(), "upstream prematurely closed connection") {
		t.Errorf("no error was written for a response cut short:\n%s", l.records())
	}
}
