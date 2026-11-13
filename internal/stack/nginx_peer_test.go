package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderWith(t *testing.T, c NginxConfig) string {
	t.Helper()
	dir := t.TempDir()
	if err := RenderNginxConfig(dir, c); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "stack.conf"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func base() NginxConfig {
	return NginxConfig{
		Routes: []Route{{Domain: "web.test", Address: "192.168.64.10:80",
			Backend: "g-web.bay.test:80", Scheme: "http"}},
		Resolver:    "192.168.64.1",
		DefaultCert: DefaultCertName,
		Generation:  "gen1",
	}
}

// With no peer link, the configuration is what it was: one port, no client
// certificates.
func TestWithoutAPeerLinkNothingChanges(t *testing.T) {
	conf := renderWith(t, base())
	if strings.Contains(conf, "ssl_verify_client") {
		t.Errorf("a machine with no peers asks for client certificates:\n%s", conf)
	}
	if strings.Contains(conf, "__bay/peer") {
		t.Errorf("a machine with no peers publishes a peer endpoint:\n%s", conf)
	}
}

// The link answers on its own port. What this machine serves is offered there
// too, and only to a client holding a certificate from an approved authority.
func TestThePeerLinkRequiresACertificate(t *testing.T) {
	c := base()
	c.PeerPort = 8443
	c.PeerAuthorities = "/etc/nginx/peers/authorities.pem"
	conf := renderWith(t, c)
	if !strings.Contains(conf, "listen 8443 ssl") {
		t.Errorf("the link does not answer on its port:\n%s", conf)
	}
	if !strings.Contains(conf, "ssl_client_certificate /etc/nginx/peers/authorities.pem") {
		t.Errorf("the link does not name the authorities it accepts:\n%s", conf)
	}
	if !strings.Contains(conf, "ssl_verify_client on") {
		t.Errorf("the link serves without a certificate:\n%s", conf)
	}
}

// The endpoint a machine is approved from has to answer before there is an
// approval, so it cannot ask for a certificate.
func TestThePeerEndpointDoesNotRequireACertificate(t *testing.T) {
	c := base()
	c.PeerPort = 8443
	c.PeerAuthorities = "/etc/nginx/peers/authorities.pem"
	conf := renderWith(t, c)
	if !strings.Contains(conf, "ssl_verify_client optional;") {
		t.Errorf("the endpoint requires a certificate:\n%s", conf)
	}
	if !strings.Contains(conf, "location = "+PeerPath) {
		t.Errorf("no peer endpoint:\n%s", conf)
	}
	before := c
	before.PeerAuthorities = ""
	if !strings.Contains(renderWith(t, before), "ssl_verify_client optional_no_ca") {
		t.Errorf("without an approved authority the endpoint checks a certificate it cannot")
	}
}

// The load is answered only to a client whose certificate an approved
// authority issued, and not at all before there is one.
func TestTheLoadIsServedOnlyToApprovedPeers(t *testing.T) {
	c := base()
	c.PeerPort = 8443
	if strings.Contains(renderWith(t, c), StatsPath) {
		t.Errorf("the load is served with no approved authority")
	}
	c.PeerAuthorities = "/etc/nginx/peers/authorities.pem"
	conf := renderWith(t, c)
	i := strings.Index(conf, "location = "+StatsPath)
	if i < 0 {
		t.Fatalf("no load endpoint:\n%s", conf)
	}
	block := conf[i:]
	block = block[:strings.Index(block, "\n    }\n")]
	if !strings.Contains(block, "$ssl_client_verify != SUCCESS") || !strings.Contains(block, "return 403") {
		t.Errorf("the load endpoint does not check the certificate:\n%s", block)
	}
	if !strings.Contains(block, "/etc/nginx/conf.d/"+StatsName) {
		t.Errorf("the load is not read from the configuration directory:\n%s", block)
	}
}

// A peer's domain is served here, with a certificate this machine issued, and
// forwarded to that peer with this machine's client certificate.
func TestAPeersDomainIsServedHereAndForwarded(t *testing.T) {
	c := base()
	c.ClientCert, c.ClientKey = "/etc/nginx/peers/client.crt", "/etc/nginx/peers/client.key"
	c.PeerRoutes = []PeerRoute{{
		Domain: "api.test", Address: "192.168.0.99:8443",
		Authority: "/etc/nginx/peers/aa.crt",
	}}
	conf := renderWith(t, c)
	for _, want := range []string{
		"server_name api.test;",
		"ssl_certificate     /etc/nginx/certs/api.test.crt;",
		"proxy_pass https://192.168.0.99:8443;",
		"proxy_ssl_certificate           /etc/nginx/peers/client.crt;",
		"proxy_ssl_trusted_certificate   /etc/nginx/peers/aa.crt;",
		"proxy_ssl_verify        on;",
		"proxy_ssl_name          api.test;",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("no %q in the configuration for a peer's domain:\n%s", want, conf)
		}
	}
}
