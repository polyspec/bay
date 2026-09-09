package main

import (
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
// provides each one. The machines and the domains are separate sections: a
// machine row is removed and a domain row is opened, and one flat list of both
// gives no way to tell them apart when several machines are approved.
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

	var machines, domains *section
	for i := range p.Sections {
		switch p.Sections[i].Header {
		case "APPROVED":
			machines = &p.Sections[i]
		case "DOMAINS":
			domains = &p.Sections[i]
		}
	}
	if machines == nil || domains == nil {
		t.Fatal("the peers screen does not list the machines and the domains separately")
	}
	// A machine row is removed and a domain row is opened, so the machines
	// section holds one row per machine and nothing else.
	if len(machines.Rows) != 2 {
		t.Errorf("the machines section holds %d rows, want one per approved machine",
			len(machines.Rows))
	}

	links := map[string]string{}
	for _, r := range domains.Rows {
		if r.Link != "" {
			links[r.Text] = r.Detail
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
