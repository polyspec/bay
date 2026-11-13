package stack

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The relay reads the name a TLS client asks for and passes the handshake on
// unchanged, so the client completes it with the server behind the relay and
// checks that server's certificate.
func TestTheRelayPassesTheSessionToThePeerServingTheName(t *testing.T) {
	peer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "served for "+r.Host)
	}))
	defer peer.Close()
	peerAddress := strings.TrimPrefix(peer.URL, "https://")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	asked := make(chan string, 4)
	go ServeRelay(ctx, ln, func(name string) string {
		asked <- name
		if name == "web.test" {
			return peerAddress
		}
		return ""
	}, t.Logf)

	roots := x509.NewCertPool()
	roots.AddCert(peer.Certificate())
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com"},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("tcp", ln.Addr().String())
		},
	}}

	// httptest's certificate names example.com; the relay is asked for the name
	// in the handshake, which the client sets from its own configuration.
	client.Transport.(*http.Transport).TLSClientConfig.ServerName = "example.com"
	if _, err := client.Get("https://web.test/"); err == nil {
		t.Fatal("a name no peer serves was relayed")
	}
	if got := <-asked; got != "example.com" {
		t.Fatalf("relay was asked for %q", got)
	}

	client.Transport.(*http.Transport).TLSClientConfig.ServerName = "web.test"
	client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
	resp, err := client.Get("https://web.test/")
	if err != nil {
		t.Fatalf("relayed request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "served for web.test" {
		t.Fatalf("body = %q", body)
	}
	if got := <-asked; got != "web.test" {
		t.Fatalf("relay was asked for %q", got)
	}
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 ||
		!resp.TLS.PeerCertificates[0].Equal(peer.Certificate()) {
		t.Fatal("the client did not complete the handshake with the peer")
	}
}

// A connection that sends no TLS handshake names nothing and is closed.
func TestTheRelayClosesAConnectionThatNamesNothing(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ServeRelay(ctx, ln, func(string) string {
		t.Error("a connection without a name was looked up")
		return ""
	}, t.Logf)
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: web.test\r\n\r\n")
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := conn.Read(make([]byte, 16)); err == nil || n > 0 {
		t.Fatalf("read %d bytes, err %v; want the connection closed", n, err)
	}
}

func TestDecodeAppleGateway(t *testing.T) {
	out := []byte(`[{"configuration":{"name":"default","mode":"nat"},"id":"default",
		"status":{"ipv4Gateway":"192.168.65.1","ipv4Subnet":"192.168.65.0/24"}}]`)
	got, err := decodeAppleGateway(out)
	if err != nil || got != "192.168.65.1" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := decodeAppleGateway([]byte(`[{"status":{}}]`)); err == nil {
		t.Fatal("a network without a gateway was accepted")
	}
}

// The Apple proxy is written to reach every peer through the relay; a Docker
// proxy reaches each peer directly.
func TestTheAppleProxyReachesPeersThroughTheRelay(t *testing.T) {
	saved := peerRelayAddress
	peerRelayAddress = func() string { return "192.168.65.1:8444" }
	t.Cleanup(func() { peerRelayAddress = saved })

	routes := []PeerRoute{{Domain: "web.test", Address: "192.168.2.3:8443", Authority: "/a.crt"}}
	apple := viaRelay(AppleEngine, routes)
	if apple[0].Via != "192.168.65.1:8444" || apple[0].Address != "192.168.2.3:8443" {
		t.Fatalf("apple route = %+v", apple[0])
	}
	if routes[0].Via != "" {
		t.Fatal("the routes given were changed")
	}
	if docker := viaRelay(DockerEngine, routes); docker[0].Via != "" {
		t.Fatalf("docker route = %+v", docker[0])
	}

	var b strings.Builder
	writePeerRoutes(&b, NginxConfig{PeerRoutes: apple, ClientCert: "/c.crt", ClientKey: "/c.key"})
	conf := b.String()
	if !strings.Contains(conf, "proxy_pass https://192.168.65.1:8444;") ||
		!strings.Contains(conf, "proxy_ssl_name          web.test;") {
		t.Fatalf("configuration does not send web.test through the relay:\n%s", conf)
	}
	if configGeneration(NginxConfig{PeerRoutes: apple}) == configGeneration(NginxConfig{PeerRoutes: routes}) {
		t.Fatal("moving a route to the relay does not change the generation")
	}
}

func TestDockerLifecycleState(t *testing.T) {
	for state, want := range map[string]string{
		"created": "stopped", "exited": "stopped", "dead": "stopped",
		"running": "running", "paused": "paused", "restarting": "restarting",
	} {
		if got := lifecycleState(Instance{State: state}).State; got != want {
			t.Errorf("%s: got %s, want %s", state, got, want)
		}
	}
	if got := lifecycleState(Instance{Started: dockerNeverStarted}).Started; got != "" {
		t.Errorf("never started: got %q", got)
	}
}

func TestDockerImageAndVolumeDecoding(t *testing.T) {
	id, err := dockerImageID([]byte(`[{"Id":"sha256:abc","RepoTags":["traefik/whoami:v1.11"]}]`), "traefik/whoami:v1.11")
	if err != nil || id != "sha256:abc" {
		t.Fatalf("image id = %q, %v", id, err)
	}
	volumes, err := decodeDockerVolumes([]byte(`[{"Name":"app-data","Driver":"local","Labels":{"bay.role":"volume"}}]`))
	if err != nil || len(volumes) != 1 || volumes[0].Configuration.Name != "app-data" ||
		volumes[0].Configuration.Labels[LabelRole] != "volume" {
		t.Fatalf("volumes = %+v, %v", volumes, err)
	}
	v := &Volume{Key: "data", Name: "app-data"}
	volumes[0].Configuration.Labels = map[string]string{
		LabelRole: "volume", LabelGroup: "app", LabelVolume: "data", LabelConfig: volumeFingerprint("app", v),
	}
	if err := checkVolumeOn(DockerEngine, "app", v, volumes[0]); err != nil {
		t.Fatalf("a Docker volume without a size: %v", err)
	}
	if err := checkVolumeOn(AppleEngine, "app", v, volumes[0]); err == nil {
		t.Fatal("an Apple volume without a size was accepted")
	}
	if !imageMissing(errorString("Error response from daemon: No such image: x")) ||
		!volumeMissing(errorString("Error response from daemon: get x: no such volume")) {
		t.Fatal("Docker's missing image or volume was not recognised")
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

// On Docker a service is reachable by the name the proxy falls back to and is
// started again after the machine restarts; an initializer is not.
func TestDockerCreateArguments(t *testing.T) {
	s := &Service{Name: "web", ContainerName: "app-web", Image: "nginx", Command: []string{"nginx", "-g", "daemon off;"}, Port: 80, Network: ProxyNetwork}
	e := &serviceEngine{engine: DockerEngine}
	args := e.createArguments("app", s, "f")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--network-alias app-web.bay.test") || !strings.Contains(joined, "--restart unless-stopped") {
		t.Fatalf("args = %v", args)
	}
	if tail := strings.Join(args[len(args)-4:], " "); tail != "nginx nginx -g daemon off;" {
		t.Fatalf("the image and command are not last: %v", args)
	}
	once := *s
	once.OneShot = true
	if strings.Contains(strings.Join(e.createArguments("app", &once, "f"), " "), "--restart") {
		t.Fatal("an initializer was given a restart policy")
	}
	apple := (&serviceEngine{engine: AppleEngine}).createArguments("app", s, "f")
	if strings.Contains(strings.Join(apple, " "), "--network-alias") {
		t.Fatalf("apple args = %v", apple)
	}
}

func TestDockerRestartPolicy(t *testing.T) {
	if got := dockerRestartPolicy([]byte(`[{"HostConfig":{"RestartPolicy":{"Name":"unless-stopped"}}}]`)); got != "unless-stopped" {
		t.Fatalf("got %q", got)
	}
	if got := dockerRestartPolicy([]byte(`[{"HostConfig":{"RestartPolicy":{"Name":""}}}]`)); got != "" {
		t.Fatalf("got %q", got)
	}
}
