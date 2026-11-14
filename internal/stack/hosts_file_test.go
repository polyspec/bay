package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useHostsFile(t *testing.T, content string) string {
	t.Helper()
	saved := hostsPath
	hostsPath = filepath.Join(t.TempDir(), "hosts")
	t.Cleanup(func() { hostsPath = saved })
	os.WriteFile(hostsPath, []byte(content), 0o644)
	return hostsPath
}

// The block is added, replaced and removed; every line outside it is kept, and
// a second sync with the same names writes nothing.
func TestTheHostsBlock(t *testing.T) {
	const system = "127.0.0.1\tlocalhost\n::1\tlocalhost ip6-localhost\n"
	path := useHostsFile(t, system)

	if wrote, err := SyncHostsFile(proxied("a.test", "web.test")); err != nil || !wrote {
		t.Fatalf("wrote %v, %v", wrote, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), system) || !strings.Contains(string(b), "127.0.0.1\tweb.test\n") {
		t.Fatalf("hosts:\n%s", b)
	}
	if !HostsCurrent(proxied("a.test", "web.test")) || HostsCurrent(proxied("a.test")) {
		t.Fatal("current reports the wrong names")
	}
	if wrote, _ := SyncHostsFile(proxied("a.test", "web.test")); wrote {
		t.Fatal("the same names were written again")
	}
	os.WriteFile(path, append(b, "10.0.0.9\tlater\n"...), 0o644)
	if _, err := SyncHostsFile(proxied("b.test")); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "web.test") || !strings.Contains(string(b), "b.test") ||
		!strings.Contains(string(b), "10.0.0.9\tlater\n") || strings.Count(string(b), hostsBegin) != 1 {
		t.Fatalf("hosts:\n%s", b)
	}
	if _, err := SyncHostsFile(nil); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if string(b) != system+"10.0.0.9\tlater\n" {
		t.Fatalf("hosts after removing the block:\n%q", b)
	}
}

func proxied(names ...string) []HostsEntry {
	return hostsEntriesFrom(names, nil)
}

// A running service container on Docker is named at <container>.bay.test with
// its address, after the proxied names; a stopped one, one without an address
// and one on Apple's engine, whose names its DNS agent answers, are not.
func TestServiceContainersAreNamedAtTheirAddresses(t *testing.T) {
	instances := []ServiceInstance{
		{Container: "ss-redis", State: "running", IPv4: "172.18.0.5", Engine: DockerEngine},
		{Container: "ss-mysql", State: "running", IPv4: "172.18.0.4", Engine: DockerEngine},
		{Container: "ss-old", State: "stopped", IPv4: "172.18.0.6", Engine: DockerEngine},
		{Container: "ss-new", State: "running", Engine: DockerEngine},
		{Container: "mac-web", State: "running", IPv4: "192.168.64.3", Engine: AppleEngine},
	}
	var lines []string
	for _, e := range hostsEntriesFrom([]string{"web.test"}, instances) {
		lines = append(lines, e.Address+" "+e.Name)
	}
	want := "127.0.0.1 web.test|172.18.0.4 ss-mysql.bay.test|172.18.0.5 ss-redis.bay.test"
	if got := strings.Join(lines, "|"); got != want {
		t.Fatalf("entries %s, want %s", got, want)
	}
	if n := ServiceHostNames(hostsEntriesFrom([]string{"web.test"}, instances)); n != 2 {
		t.Fatalf("service names %d, want 2", n)
	}
}

// The names are the machine's own route domains and its peers' domains, a name
// the machine serves itself being its own.
func TestHostsNames(t *testing.T) {
	routes := []Route{{Domain: "hello.test"}, {Domain: "Hello.test"}}
	peers := []Peer{{Fingerprint: "f", Name: "mac", Domains: []string{"app.test", "hello.test"}}}
	if names := hostsNamesFrom(routes, peers); strings.Join(names, " ") != "app.test hello.test" {
		t.Fatalf("names = %v", names)
	}
}

// The agent's unit runs bayd with the hosts file kept, for this state
// directory, and changes when any of them does.
func TestTheAgentUnit(t *testing.T) {
	l := LinuxSetup{Dir: "/root/.bay", DNSBin: "/usr/local/bin/bayd", Addr: "127.0.0.1:5354", Domains: []string{"test"}}
	body := l.agentUnitBody()
	if !strings.Contains(body, "ExecStart=/usr/local/bin/bayd -domain test -addr 127.0.0.1:5354 -state /root/.bay -hosts\n") ||
		!strings.Contains(body, "Restart=always") || !strings.Contains(body, "WantedBy=multi-user.target") {
		t.Fatalf("unit:\n%s", body)
	}
	other := l
	other.Domains = []string{"test", "devel"}
	if other.agentUnitBody() == body {
		t.Fatal("another domain list leaves the unit current")
	}
}
