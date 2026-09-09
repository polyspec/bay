package stack

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// publishedAddrs returns the host address of each --publish argument.
func publishedAddrs(args []string) []string {
	var out []string
	for i, a := range args {
		if a != "--publish" || i+1 >= len(args) {
			continue
		}
		parts := strings.Split(args[i+1], ":")
		if len(parts) == 3 {
			out = append(out, parts[0])
		} else {
			out = append(out, "")
		}
	}
	return out
}

// Rule 9: only /etc/resolver writes require administrator rights. macOS assigns
// one address to lo0, so publishing on any other loopback address requires an
// alias added as root and the proxy fails to start without it.
func TestThePublishedProxyAddressBindsWithoutRoot(t *testing.T) {
	args := proxyRunArgs(DockerEngine, "/conf", "/certs", "/peers")
	addrs := publishedAddrs(args)
	if len(addrs) == 0 {
		t.Fatal("the docker proxy publishes nothing, so the host cannot reach it")
	}
	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(addr, "0"))
		if err != nil {
			t.Fatalf("the proxy publishes on %s, which this machine cannot bind: %v", addr, err)
		}
		ln.Close()
	}
}

// The host has a route to an Apple container, so the Apple proxy publishes only
// the peer link.
func TestTheAppleProxyPublishesOnlyThePeerLink(t *testing.T) {
	args := proxyRunArgs(AppleEngine, "/conf", "/certs", "/peers")
	published := strings.Join(args, " ")
	for _, port := range []string{":80:80", ":443:443", "80:80", "443:443"} {
		if strings.Contains(published, "--publish "+port) {
			t.Errorf("the apple proxy publishes %s; the host reaches it at its own address", port)
		}
	}
	if !strings.Contains(published, "--publish 8443:8443") {
		t.Error("the apple proxy does not publish the peer link")
	}
}

// A machine with no peer link publishes no port to the network.
func TestNoPeerLinkPublishesNoNetworkPort(t *testing.T) {
	for _, engine := range []string{AppleEngine, DockerEngine} {
		args := proxyRunArgs(engine, "/conf", "/certs", "")
		if strings.Contains(strings.Join(args, " "), "8443") {
			t.Errorf("%s publishes the peer link with no link configured", engine)
		}
		for _, addr := range publishedAddrs(args) {
			if addr != "" && addr != "127.0.0.1" {
				t.Errorf("%s publishes on %s, which is not the loopback", engine, addr)
			}
		}
	}
}

// nginx fails to start when the resolver directive has no address, so a
// configuration with no name to resolve omits the directive.
func TestNoResolverWritesNoResolverDirective(t *testing.T) {
	dir := t.TempDir()
	conf := NginxConfig{DefaultCert: DefaultCertName, PeerPort: PeerPort, Generation: "g"}
	if err := RenderNginxConfig(dir, conf); err != nil {
		t.Fatal(err)
	}
	body := readConf(t, dir)
	if strings.Contains(body, "resolver ;") || strings.Contains(body, "resolver  ") {
		t.Errorf("an empty resolver was written, which nginx refuses to start on:\n%s", body)
	}
	if strings.Contains(body, "resolver") {
		t.Errorf("a resolver was written with nothing to resolve:\n%s", body)
	}
}

// Docker resolves a container name on its own resolver address, not on the
// network gateway.
func TestTheDockerResolverIsDockersOwn(t *testing.T) {
	if got := engineResolver(DockerEngine, "172.20.0.1"); got != DockerResolver {
		t.Errorf("docker resolver = %q, want %q", got, DockerResolver)
	}
	if got := engineResolver(AppleEngine, "192.168.64.1"); got != "192.168.64.1" {
		t.Errorf("apple resolver = %q, want the network gateway", got)
	}
}

func readConf(t *testing.T, dir string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "stack.conf"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// A request that is not a protocol upgrade must not send Connection: close to
// the upstream. Measured against a peer link, that header made the upstream end
// the response before the body was complete: the same 327197-byte file was
// received in full without it and truncated at a different offset with it,
// returning status 200 in both cases. An upgrade request still requires the
// header, so only the empty case of the map changes.
func TestAnOrdinaryRequestCarriesNoConnectionClose(t *testing.T) {
	dir := t.TempDir()
	conf := NginxConfig{
		Routes:      []Route{{Domain: "a.test", Address: "10.0.0.2:80", Backend: "c.container.test:80", Scheme: "http"}},
		Resolver:    "10.0.0.1",
		DefaultCert: DefaultCertName,
		Generation:  "g",
	}
	if err := RenderNginxConfig(dir, conf); err != nil {
		t.Fatal(err)
	}
	body := readConf(t, dir)
	if strings.Contains(body, "''      close;") {
		t.Errorf("a request with no Upgrade header is sent Connection: close:\n%s",
			body[:strings.Index(body, "\n\n")+1])
	}
	if !strings.Contains(body, "default upgrade;") {
		t.Error("an upgrade request no longer carries Connection: upgrade")
	}
}
