package stack

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// maxConnectSeconds is how long the proxy may wait for a connection. An address
// a container no longer holds answers nothing, so this is what a person waits
// before anything at all happens. nginx waits a minute by default.
const maxConnectSeconds = 5

// route returns a route with the address of a running container, which is what
// SyncProxy renders from.
func route() Route {
	return Route{
		Domain:  "web.test",
		Address: "192.168.64.10:80",
		Backend: "g-web.container.test:80",
		Scheme:  "http",
	}
}

func render(t *testing.T, routes ...Route) string {
	t.Helper()
	dir := t.TempDir()
	if err := RenderNginx(dir, routes, "192.168.64.1", DefaultCertName, "gen1"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "stack.conf"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The runtime's DNS answers with a container's previous address for about
// fifteen seconds after it is recreated, so a name is not how a route reaches a
// container it already knows the address of.
func TestARouteIsSentToTheKnownAddress(t *testing.T) {
	conf := render(t, route())
	if !strings.Contains(conf, `set $backend "http://192.168.64.10:80"`) {
		t.Errorf("the route does not use the address of the container it was built from:\n%s", conf)
	}
}

// The address is only as fresh as the configuration. A container recreated
// without bay keeps its name, so the name is what the route falls back
// to when the address stops answering.
func TestARouteFallsBackToTheName(t *testing.T) {
	conf := render(t, route())
	if !strings.Contains(conf, `set $byname "http://g-web.container.test:80"`) {
		t.Errorf("the route has no name to fall back to:\n%s", conf)
	}
	if !strings.Contains(conf, "error_page 502 504 = @byname") {
		t.Errorf("an address that stops answering is not followed by the name:\n%s", conf)
	}
	if got, want := strings.Count(conf, "location @byname"), strings.Count(conf, "server_name "); got != want {
		t.Errorf("%d fallback locations for %d routed servers", got, want)
	}
}

// A route built before the container had an address has only the name.
func TestARouteWithoutAnAddressUsesTheName(t *testing.T) {
	r := route()
	r.Address = ""
	conf := render(t, r)
	if !strings.Contains(conf, `set $backend "http://g-web.container.test:80"`) {
		t.Errorf("a route with no address does not use the name:\n%s", conf)
	}
	if strings.Contains(conf, "@byname") {
		t.Errorf("a route with no address has nothing to fall back to:\n%s", conf)
	}
}

// A slow answer is only diagnosable if the proxy says which of the two targets
// answered.
func TestTheAnsweringTargetIsNamed(t *testing.T) {
	conf := render(t, route())
	for _, want := range []string{
		"add_header " + RouteHeader + " address always",
		"add_header " + RouteHeader + " name always",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("no %q:\n%s", want, conf)
		}
	}
}

// Without a bound, an address that answers nothing hangs for a minute.
func TestConnectTimeoutIsBounded(t *testing.T) {
	conf := render(t, route())
	m := regexp.MustCompile(`proxy_connect_timeout (\d+)s`).FindStringSubmatch(conf)
	if m == nil {
		t.Fatalf("no proxy_connect_timeout:\n%s", conf)
	}
	wait, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if wait > maxConnectSeconds {
		t.Errorf("the proxy waits %ds for a connection; a stale address hangs that long", wait)
	}
	if got, want := strings.Count(conf, "proxy_connect_timeout"),
		strings.Count(conf, "proxy_pass "); got != want {
		t.Errorf("%d of %d proxy_pass directives are bounded", got, want)
	}
}
