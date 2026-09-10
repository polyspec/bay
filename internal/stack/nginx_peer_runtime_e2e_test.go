package stack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The peer link uses directives the rest of the configuration does not: a
// second listening port, a client certificate check, and a location serving a
// file. nginx is asked whether what is written is a configuration it accepts.
func TestPeerConfigurationIsAcceptedByNginxActualRuntime(t *testing.T) {
	if os.Getenv("BAY_E2E") == "" && os.Getenv("BAY_SERVICE_E2E") == "" {
		t.Skip("set BAY_E2E=1 to have nginx read the configuration")
	}
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	certDir := filepath.Join(dir, "certs")
	peerDir := filepath.Join(dir, "peers")

	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{DefaultCertName, "web.test", "api.test"} {
		if _, err := ca.Issue(certDir, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := ca.IssueClient(peerDir, "max"); err != nil {
		t.Fatal(err)
	}
	caPEM, err := os.ReadFile(ca.CertPath())
	if err != nil {
		t.Fatal(err)
	}
	peer := Peer{Name: "alpha", Address: "192.168.0.99:8443",
		Domains: []string{"api.test"}, CA: string(caPEM)}
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
	if err := WritePeerDocument(confDir, PeerDocument{Name: "max", CA: string(caPEM)}); err != nil {
		t.Fatal(err)
	}
	if err := RenderNginxConfig(confDir, NginxConfig{
		Routes: []Route{{Domain: "web.test", Address: "192.168.64.10:80",
			Backend: "g-web.container.test:80", Scheme: "http"}},
		Resolver:        "192.168.64.1",
		DefaultCert:     DefaultCertName,
		Generation:      "gen1",
		PeerPort:        PeerPort,
		PeerAuthorities: "/etc/nginx/peers/" + PeerAuthoritiesName,
		ClientCert:      "/etc/nginx/peers/" + ClientCertName + ".crt",
		ClientKey:       "/etc/nginx/peers/" + ClientCertName + ".key",
		PeerRoutes: []PeerRoute{{Domain: "api.test", Address: peer.Address,
			Authority: "/etc/nginx/peers/" + peers[0].Fingerprint + ".crt"}},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(engineBin(ServiceEngine()), "run", "--rm",
		"--volume", confDir+":/etc/nginx/conf.d:ro",
		"--volume", certDir+":/etc/nginx/certs:ro",
		"--volume", peerDir+":/etc/nginx/peers:ro",
		ProxyImage, "nginx", "-t").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "test is successful") {
		t.Fatalf("nginx did not accept the configuration: %v\n%s", err, out)
	}
}
