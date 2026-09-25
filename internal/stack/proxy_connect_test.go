package stack

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// connectFixture is an Apple engine whose proxy opens a connection to a
// container on the attempt named by succeedAt, and never when it is zero. It
// records every exec and whether the configuration served the route when a
// connection was attempted.
type connectFixture struct {
	machine *Machine
	calls   string
	route   Route
	conf    NginxConfig
}

const connectProbe = "exec " + ProxyName + " nc -z -w 2 192.168.64.30 8080"

func newConnectFixture(t *testing.T, proxyRunning bool, succeedAt int) connectFixture {
	t.Helper()
	dir := t.TempDir()
	m := NewMachine(filepath.Join(dir, "state"))
	f := connectFixture{
		machine: m,
		calls:   filepath.Join(dir, "calls"),
		route: Route{
			Domain: "web.first.test", Address: "192.168.64.30:8080",
			Backend: "first-web." + BackendDomain + ":8080", Scheme: "http",
			Engine: AppleEngine, container: "first-web",
		},
	}
	f.conf = NginxConfig{Routes: []Route{f.route}, DefaultCert: DefaultCertName, Generation: "g"}
	listing := "[]"
	if proxyRunning {
		listing = `[{"configuration":{"id":"` + ProxyName + `","labels":{}},` +
			`"status":{"state":"running","networks":[{"ipv4Address":"192.168.64.2/24"}]}}]`
	}
	mounts := `[{"configuration":{"mounts":[` +
		`{"source":"` + m.ConfDir(AppleEngine) + `","destination":"/etc/nginx/conf.d"},` +
		`{"source":"` + m.CertDir() + `","destination":"/etc/nginx/certs"}]}}]`
	body := `#!/bin/sh
case "$1" in
ls) printf '%s' '` + listing + `' ;;
inspect) printf '%s' '` + mounts + `' ;;
run) printf 'run\n' >> "` + f.calls + `" ;;
exec)
    if [ "$3" = nc ]; then
        published=no
        grep -q web.first.test "` + filepath.Join(m.ConfDir(AppleEngine), "stack.conf") + `" 2>/dev/null && published=yes
        printf '%s published=%s\n' "$*" "$published" >> "` + f.calls + `"
        attempts=$(grep -c ' nc ' "` + f.calls + `")
        if [ ` + strconv.Itoa(succeedAt) + ` -eq 0 ] || [ "$attempts" -lt ` + strconv.Itoa(succeedAt) + ` ]; then
            printf '%s\n' 'nc: timeout' >&2
            exit 1
        fi
    else
        printf '%s\n' "$*" >> "` + f.calls + `"
    fi
    ;;
esac
`
	binary := filepath.Join(dir, "container")
	if err := os.WriteFile(binary, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTAINER_BIN", binary)
	t.Setenv("DOCKER_BIN", filepath.Join(dir, "no-docker"))
	return f
}

func (f connectFixture) recorded(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(f.calls)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(body)), "\n")
}

// The proxy of a running engine is given a new route only after the proxy
// itself opens a connection to the container's address. A healthcheck runs
// inside the service's container and the readiness report connects from the
// host, so neither observes the proxy's own path to the container.
func TestARouteIsPublishedAfterTheProxyConnectsToIt(t *testing.T) {
	f := newConnectFixture(t, true, 3)
	var said []string
	created, err := publishProxy(f.machine, AppleEngine, f.conf, "", []Route{f.route}, 5*time.Second,
		func(line string) { said = append(said, line) })
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("a running proxy was reported as created")
	}
	want := []string{
		connectProbe + " published=no",
		connectProbe + " published=no",
		connectProbe + " published=no",
		"exec " + ProxyName + " nginx -s reload",
	}
	if got := f.recorded(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// The wait can last the whole bound, so it is reported before it begins.
	if wantSaid := []string{ProxyName + ": connecting to 192.168.64.30:8080 for web.first.test, up to 5s"}; !reflect.DeepEqual(said, wantSaid) {
		t.Fatalf("reported %q, want %q", said, wantSaid)
	}
}

// A proxy that cannot connect to the container within the bound leaves its
// configuration as it was, and the error names the route and what the proxy
// reported.
func TestAProxyThatCannotConnectPublishesNothing(t *testing.T) {
	f := newConnectFixture(t, true, 0)
	_, err := publishProxy(f.machine, AppleEngine, f.conf, "", []Route{f.route}, 300*time.Millisecond, nil)
	if err == nil {
		t.Fatal("a route the proxy cannot connect to was published")
	}
	for _, want := range []string{ProxyName, "192.168.64.30:8080", "web.first.test", "nc: timeout"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %q: %v", want, err)
		}
	}
	for _, call := range f.recorded(t) {
		if call != connectProbe+" published=no" {
			t.Errorf("the proxy was changed before it connected: %s", call)
		}
	}
	if _, err := os.Stat(filepath.Join(f.machine.ConfDir(AppleEngine), "stack.conf")); !os.IsNotExist(err) {
		t.Errorf("the configuration was written before the proxy connected: %v", err)
	}
}

// A proxy this call creates starts with the route, and the call returns only
// after that proxy opens a connection to the container.
func TestACreatedProxyIsReturnedAfterItConnects(t *testing.T) {
	f := newConnectFixture(t, false, 2)
	created, err := publishProxy(f.machine, AppleEngine, f.conf, "", []Route{f.route}, 5*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("the proxy was not created")
	}
	want := []string{"run", connectProbe + " published=yes", connectProbe + " published=yes"}
	if got := f.recorded(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A command waits for the containers it started and for no other. Several
// domains of one service share one address, which is connected to once.
func TestOnlyTheStartedContainersAreConnectedTo(t *testing.T) {
	routes := []Route{
		{Domain: "a.first.test", Address: "192.168.64.30:8080", container: "first-web"},
		{Domain: "b.first.test", Address: "192.168.64.30:8080", container: "first-web"},
		{Domain: "other.test", Address: "192.168.64.31:80", container: "other-web"},
	}
	got := verifiedRoutes(routes, []string{"first-web"})
	if len(got) != 1 || got[0].Address != "192.168.64.30:8080" {
		t.Fatalf("verified routes = %+v, want the one address of first-web", got)
	}
	if got := verifiedRoutes(routes, nil); len(got) != 0 {
		t.Fatalf("a command that started nothing waits for %+v", got)
	}
}
