package stack

import (
	"strings"
	"testing"
)

func TestServedNames(t *testing.T) {
	c := NginxConfig{
		Routes:     []Route{{Domain: "web.test"}, {Domain: "Web.test"}},
		PeerRoutes: []PeerRoute{{Domain: "mac.test"}, {Domain: "web.test"}},
	}
	if got := strings.Join(servedNames(c), " "); got != "mac.test web.test" {
		t.Fatalf("got %q", got)
	}
}

// Docker adds the container's name and ID as aliases; only the domain names
// are the proxy's.
func TestDecodeProxyAliases(t *testing.T) {
	out := []byte(`[{"NetworkSettings":{"Networks":{"bay":{"Aliases":["bay-edge","1811aad283c2","Mac.test","odroid.test"]},"bridge":{"Aliases":["x.test"]}}}}]`)
	if got := strings.Join(decodeProxyAliases(out, "bay"), " "); got != "mac.test odroid.test" {
		t.Fatalf("got %q", got)
	}
	if got := decodeProxyAliases([]byte(`[]`), "bay"); got != nil {
		t.Fatalf("got %v", got)
	}
}

func TestConnectArguments(t *testing.T) {
	got := strings.Join(connectArguments("bay", []string{"a.test", "b.test"}), " ")
	if got != "network connect --alias a.test --alias b.test bay bay-edge" {
		t.Fatalf("got %q", got)
	}
}
