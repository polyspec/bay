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

// A response that passes through a peer's link arrives whole. The proxy answers
// 200 whether or not the peer finished sending, so a body shorter than its
// content length is reported as success and the loss is silent. This stands up
// both ends: a container serving a file over a link that checks the client
// certificate, and the proxy this program renders, with a route to it.
func TestAPeerRouteDeliversTheWholeBodyActualRuntime(t *testing.T) {
	if os.Getenv("BAY_E2E") == "" {
		t.Skip("set BAY_E2E=1 to start containers")
	}
	engine := ServiceEngine()
	bin := engineBin(engine)
	if bin == "" {
		t.Skipf("%s is not installed", engine)
	}

	const (
		domain = "peerbody.test"
		size   = 2 << 20 // larger than nginx's proxy buffers, so it streams
		port   = "18443" // the proxy's own port on the loopback address
	)
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	network := "bay-test-" + stamp
	peerName := "bay-test-peer-" + stamp
	edgeName := "bay-test-edge-" + stamp
	originName := "bay-test-origin-" + stamp

	run := func(args ...string) ([]byte, error) {
		return exec.Command(bin, args...).CombinedOutput()
	}
	t.Cleanup(func() {
		run("rm", "--force", edgeName)
		run("rm", "--force", peerName)
		run("rm", "--force", originName)
		run("network", "rm", network)
	})
	if out, err := run("network", "create", network); err != nil {
		t.Skipf("cannot create a network: %v\n%s", err, out)
	}

	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	certDir := filepath.Join(dir, "certs")
	peerDir := filepath.Join(dir, "peers")
	peerConf := filepath.Join(dir, "peerconf")
	originConf := filepath.Join(dir, "originconf")
	data := filepath.Join(dir, "data")
	for _, d := range []string{confDir, certDir, peerDir, peerConf, originConf, data} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ca, err := LoadOrCreateCA(dir)
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
	if err := ApprovePeer(dir, peer); err != nil {
		t.Fatal(err)
	}
	peers, err := Peers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePeerAuthorities(dir, peers); err != nil {
		t.Fatal(err)
	}

	// The body is larger than the proxy's buffers, so it is streamed rather
	// than answered from memory.
	body := strings.Repeat("bay.", size/13)
	if err := os.WriteFile(filepath.Join(data, "body.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// What the far end serves. A peer holds a domain and its proxy sends to a
	// container, so the file is served by one rather than by the link itself.
	origin := fmt.Sprintf("server {\n    listen 80 default_server;\n    location / { root /data; }\n}\n")
	if err := os.WriteFile(filepath.Join(originConf, "origin.conf"), []byte(origin), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run("run", "--detach", "--name", originName, "--network", network,
		"--volume", originConf+":/etc/nginx/conf.d:ro",
		"--volume", data+":/data:ro", ProxyImage); err != nil {
		t.Fatalf("the origin did not start: %v\n%s", err, out)
	}

	// The far end is this program's own configuration with the link open, so
	// both ends of the link are what a machine actually runs.
	// The route sends to a name, which the engine's own resolver answers.
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
	if out, err := run("run", "--detach", "--name", peerName, "--network", network,
		"--volume", peerConf+":/etc/nginx/conf.d:ro",
		"--volume", certDir+":/etc/nginx/certs:ro",
		"--volume", peerDir+":/etc/nginx/peers:ro", ProxyImage); err != nil {
		t.Fatalf("the far end did not start: %v\n%s", err, out)
	}

	if err := RenderNginxConfig(confDir, NginxConfig{
		DefaultCert: DefaultCertName,
		Generation:  "gen",
		PeerRoutes: []PeerRoute{{Domain: domain, Address: peer.Address,
			Authority: "/etc/nginx/peers/" + peers[0].Fingerprint + ".crt"}},
		ClientCert: "/etc/nginx/peers/" + ClientCertName + ".crt",
		ClientKey:  "/etc/nginx/peers/" + ClientCertName + ".key",
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := run("run", "--detach", "--name", edgeName, "--network", network,
		"--publish", "127.0.0.1:"+port+":443",
		"--volume", confDir+":/etc/nginx/conf.d:ro",
		"--volume", certDir+":/etc/nginx/certs:ro",
		"--volume", peerDir+":/etc/nginx/peers:ro", ProxyImage); err != nil {
		t.Fatalf("the proxy did not start: %v\n%s", err, out)
	}

	// Both ends answer before anything is asked of them.
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err := exec.Command("curl", "-sk", "--max-time", "5",
			"--resolve", domain+":"+port+":127.0.0.1",
			"-o", "/dev/null", "-w", "%{http_code}",
			"https://"+domain+":"+port+"/body.txt").Output()
		if err == nil && strings.TrimSpace(string(out)) == "200" {
			break
		}
		if time.Now().After(deadline) {
			logs, _ := run("logs", edgeName)
			far, _ := run("logs", peerName)
			t.Fatalf("the link did not answer within 30s: %s\nproxy:\n%s\nfar end:\n%s",
				out, logs, far)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// A truncated body is answered 200, so the length is what reports it. It is
	// asked for several times because the fault appeared on some connections
	// and not others.
	for i := 1; i <= 5; i++ {
		out, err := exec.Command("curl", "-sk", "--max-time", "30",
			"--resolve", domain+":"+port+":127.0.0.1",
			"-o", "/dev/null", "-w", "%{size_download}",
			"https://"+domain+":"+port+"/body.txt").Output()
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		got, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
		if convErr != nil {
			t.Fatalf("request %d returned %q", i, out)
		}
		if got != len(body) {
			logs, _ := run("logs", edgeName)
			t.Fatalf("request %d received %d bytes of %d, and was answered 200:\n%s",
				i, got, len(body), logs)
		}
	}
}
