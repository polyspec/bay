package main

import (
	"strings"
	"testing"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// Removing every unused certificate has to take exactly the ones the list marks
// unused, in the order the list holds them, and nothing else.
func TestUnusedCertificatesTakesOnlyTheUnusedOnes(t *testing.T) {
	snap := stack.Snapshot{Certificates: stack.CertStatus{Issued: []stack.CertInfo{
		{Name: "_default"},
		{Name: "web.test"},
		{Name: "leftover.test", Orphaned: true},
		{Name: "api.test"},
		{Name: "another.test", Orphaned: true},
	}}}
	got := unusedCertificates(snap)
	want := []string{"leftover.test", "another.test"}
	if len(got) != len(want) {
		t.Fatalf("unusedCertificates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unusedCertificates = %v, want %v", got, want)
		}
	}
}

// Nothing marked unused means nothing to remove.
func TestUnusedCertificatesIsEmptyWhenNothingIsUnused(t *testing.T) {
	snap := stack.Snapshot{Certificates: stack.CertStatus{Issued: []stack.CertInfo{
		{Name: "web.test"},
	}}}
	if got := unusedCertificates(snap); len(got) != 0 {
		t.Errorf("unusedCertificates = %v, want none", got)
	}
}

// Several names share one resolver directory. A list of whole paths does not
// fit the row, and truncating it in the middle hides a name.
func TestResolverPaths(t *testing.T) {
	for _, c := range []struct {
		domains []string
		want    string
	}{
		{nil, "/etc/resolver/"},
		{[]string{"test"}, "/etc/resolver/test"},
		{[]string{"devel", "staging", "test"}, "/etc/resolver/{devel,staging,test}"},
	} {
		if got := resolverPaths(c.domains); got != c.want {
			t.Errorf("resolverPaths(%v) = %q, want %q", c.domains, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	for _, c := range []struct {
		in   time.Duration
		want string
	}{
		{45 * time.Second, "45 s"},
		{90 * time.Second, "1 m 30 s"},
		{3*time.Hour + 12*time.Minute, "3 h 12 m"},
	} {
		if got := humanDuration(c.in); got != c.want {
			t.Errorf("humanDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A path outside the home directory is left as it is.
func TestShortPathLeavesForeignPaths(t *testing.T) {
	if got := shortPath("/tmp/guidecheck"); got != "/tmp/guidecheck" {
		t.Errorf("shortPath = %q", got)
	}
}

// The peers screen states which domains this machine reaches and which machine
// provides each one. One machine is one section and its domains are the rows of
// that section, so the domains a machine provides are inside it. A list holding
// both machines and domains states that only in a column, which is read as one
// more attribute of the row rather than as which machine provides it.
func TestThePeersScreenListsEachPeersDomains(t *testing.T) {
	snap := stack.Snapshot{}
	snap.Machine.Peering = true
	snap.Machine.Link = "192.168.0.10:8443"
	snap.Machine.Peers = []stack.Peer{
		{Name: "max", Address: "192.168.0.57:8443", Fingerprint: "aaaa1111",
			Domains: []string{"polyspec.test", "registry.soksak.test"}},
		{Name: "lab", Address: "192.168.0.90:8443", Fingerprint: "bbbb2222",
			Domains: []string{"build.test"}},
	}

	p := &panel{}
	peersView(p, snap, false, nil)

	// One machine is one section, so a domain is inside the section of the
	// machine that provides it and nowhere else.
	links := map[string]string{}
	for _, sec := range p.Sections {
		name := sectionTitle(sec)
		if name != "max" && name != "lab" {
			continue
		}
		for _, r := range sec.Rows {
			if r.ID != "" {
				links[r.Text] = name
			}
		}
	}
	for domain, peer := range map[string]string{
		"polyspec.test":        "max",
		"registry.soksak.test": "max",
		"build.test":           "lab",
	} {
		got, ok := links[domain]
		if !ok {
			t.Errorf("%s is not listed, so the screen does not say it is reachable", domain)
			continue
		}
		if got != peer {
			t.Errorf("%s is listed under %q, want %q", domain, got, peer)
		}
	}
}

// The address and the fingerprint state which machine a section belongs to, so
// they are placed in rows the renderer draws. A section field the renderer does
// not read is dropped without any error, and the screen then names a machine and
// states nothing else about it.
func TestAPeerSectionStatesItsAddressAndFingerprint(t *testing.T) {
	snap := stack.Snapshot{}
	snap.Machine.Peering = true
	snap.Machine.Peers = []stack.Peer{
		{Name: "max", Address: "192.168.0.57:8443", Fingerprint: "aaaa1111bbbb2222",
			Domains: []string{"polyspec.test"}},
	}

	p := &panel{}
	peersView(p, snap, false, nil)

	var found *section
	for i := range p.Sections {
		if sectionTitle(p.Sections[i]) == "max" {
			found = &p.Sections[i]
		}
	}
	if found == nil {
		t.Fatal("the peers screen has no section for the approved machine")
	}
	var address, fingerprint, remove bool
	for _, r := range found.Rows {
		if r.Kind == "title" && len(r.Buttons) > 0 {
			remove = true
		}
		if r.Kind != "kv" {
			continue
		}
		if r.Detail == "192.168.0.57:8443" {
			address = true
		}
		if r.Detail == shortFingerprint("aaaa1111bbbb2222") {
			fingerprint = true
		}
	}
	if !address {
		t.Error("the section does not state the machine's address in a row")
	}
	if !fingerprint {
		t.Error("the section does not state the machine's fingerprint in a row")
	}
	if !remove {
		t.Error("the machine's title row carries no remove action")
	}
}

// sectionTitle returns the machine a section stands for, taken from its title
// row. The name is inside the card, so a section with no title row belongs to
// no machine.
func sectionTitle(sec section) string {
	for _, r := range sec.Rows {
		if r.Kind == "title" {
			return r.Text
		}
	}
	return ""
}

// A machine with an approved peer answers that peer's domains through its
// proxy, with or without a container of its own. When the proxy is gone those
// domains are unreachable, so the screen reports it and offers to start the
// proxy again. Counting only the machine's own routes left that machine
// reporting nothing at all.
func TestAMissingProxyIsReportedWhenOnlyAPeersDomainsAreServed(t *testing.T) {
	snap := stack.Snapshot{}
	snap.Machine.Peering = true
	snap.Machine.Peers = []stack.Peer{{
		Name: "max", Address: "192.168.0.59:8443", Fingerprint: "aaaa1111",
		Domains: []string{"polyspec.test", "registry.soksak.test"},
	}}
	snap.Machine.Proxies = []stack.ProxyStatus{{
		Name: "bay-edge", Engine: "docker", State: "absent",
	}}

	if !proxyDown(snap) {
		t.Fatal("a machine whose proxy is gone reports nothing while a peer's domains go unanswered")
	}
	p := &panel{}
	dashboardView(p, snap, false)
	if p.Banner == nil {
		t.Fatal("no banner reports the missing proxy")
	}
	var restart bool
	for _, b := range p.Banner.Buttons {
		if b.ID == "proxy-restart" {
			restart = true
		}
	}
	if !restart {
		t.Errorf("the banner offers no way to start the proxy again: %+v", p.Banner.Buttons)
	}
	// No container of this machine is running, so the banner does not say any
	// are, and what cannot be reached is a name rather than a route: these are
	// a peer's domains.
	if strings.Contains(p.Banner.Text, "containers are up") {
		t.Errorf("the banner says containers are running while none are: %s", p.Banner.Text)
	}
	if !strings.Contains(p.Banner.Text, "2 names") {
		t.Errorf("the banner does not state how many names cannot be reached: %s", p.Banner.Text)
	}
}

// A proxy that is running and answering is not reported as a fault, even with
// no route of its own, because it is what answers the peer's domains.
func TestARunningProxyWithNoRouteIsNotAFault(t *testing.T) {
	snap := stack.Snapshot{}
	snap.Machine.Peers = []stack.Peer{{Name: "max", Domains: []string{"polyspec.test"}}}
	snap.Machine.Proxies = []stack.ProxyStatus{{
		Name: "bay-edge", Engine: "docker", State: "running",
		Generation: "1a1f4ef39c48b311",
	}}
	if proxyDown(snap) {
		t.Error("a running proxy with no route of its own is reported as a fault")
	}
}
