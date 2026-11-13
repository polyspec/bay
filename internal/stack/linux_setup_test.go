package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOnlyFrom(t *testing.T) {
	got, err := ParseOnlyFrom("192.168.2.1, 10.0.0.0/8,fd00::1")
	if err != nil || strings.Join(got, " ") != "192.168.2.1 10.0.0.0/8 fd00::1" {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := ParseOnlyFrom("none"); err != nil || got != nil {
		t.Fatalf("none: got %v, %v", got, err)
	}
	for _, bad := range []string{"", ",", "odroid.local", "192.168.2.300"} {
		if _, err := ParseOnlyFrom(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// The rules guard the named interface only, keep replies to this machine's own
// connections and the answers it needs to keep its address, and accept new
// connections from the listed hosts and nothing else.
func TestFirewallRules(t *testing.T) {
	rules := firewallRules("eth0", []string{"192.168.2.1", "fd00::1"})
	for _, want := range []string{
		"table inet bay\ndelete table inet bay\n",
		"ct state established,related accept",
		"ip saddr { 192.168.2.1 } accept",
		"ip6 saddr { fd00::1 } accept",
		"udp sport 67 udp dport 68 accept",
		"    drop\n",
		`hook input   priority -10; policy accept; iifname "eth0" jump guard;`,
		`hook forward priority -10; policy accept; iifname "eth0" jump guard;`,
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("rules lack %q:\n%s", want, rules)
		}
	}
	if firewallRules("eth0", []string{"192.168.2.1"}) != firewallRules("eth0", []string{"192.168.2.1"}) {
		t.Fatal("the same setting writes different rules")
	}
	if firewallRules("eth1", []string{"192.168.2.1"}) == firewallRules("eth0", []string{"192.168.2.1"}) {
		t.Fatal("a moved default route leaves the rules current")
	}
}

// With no firewall asked for, a machine with none is in place; one left behind
// is a step to remove it.
func TestNoFirewallAskedFor(t *testing.T) {
	useFirewallDirs(t)
	l := LinuxSetup{Dir: t.TempDir(), Interface: "eth0"}
	if it := l.firewallItem(); !it.OK {
		t.Fatalf("item = %+v", it)
	}
	os.WriteFile(firewallUnitPath(), []byte(firewallUnitBody()), 0o644)
	if it := l.firewallItem(); it.OK || !strings.Contains(it.Text, "remove") {
		t.Fatalf("item = %+v", it)
	}
}

// The setting is recorded in the machine's settings, so the next install
// applies the same firewall without being told again.
func TestOnlyFromIsRecorded(t *testing.T) {
	m := NewMachine(t.TempDir())
	if err := m.SetOnlyFrom([]string{"192.168.2.1"}); err != nil {
		t.Fatal(err)
	}
	l, err := NewLinuxSetup(m, "", "")
	if err != nil || strings.Join(l.OnlyFrom, ",") != "192.168.2.1" {
		t.Fatalf("setup = %+v, %v", l, err)
	}
}

// The store is read from the system: the first known tool on the search path
// whose directory is there. A system with none is reported, with no step.
func TestTheTrustStoreIsReadFromTheSystem(t *testing.T) {
	useFirewallDirs(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	root := t.TempDir()
	debian := filepath.Join(root, "usr/local/share/ca-certificates")
	fedora := filepath.Join(root, "etc/pki/ca-trust/source/anchors")
	trustStores = []struct {
		tool string
		args []string
		dirs []string
	}{
		{"update-ca-certificates", nil, []string{debian}},
		{"update-ca-trust", []string{"extract"}, []string{fedora}},
	}
	l := LinuxSetup{Dir: t.TempDir()}

	if it := l.trustItem(); it.OK || it.Text != "" || !strings.Contains(it.Name, "install ca-certificates") {
		t.Fatalf("no store: %+v", it)
	}
	tool := func(name string) {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755)
	}
	tool("update-ca-trust")
	os.MkdirAll(fedora, 0o755)
	if s, ok := findTrustStore(); !ok || s.Anchor != filepath.Join(fedora, "bay.crt") || s.Args[0] != "extract" {
		t.Fatalf("fedora: %+v %v", s, ok)
	}
	tool("update-ca-certificates")
	if _, ok := findTrustStore(); !ok {
		t.Fatal("store lost")
	}
	if s, _ := findTrustStore(); !strings.HasPrefix(s.Anchor, filepath.Dir(debian)) && !strings.HasPrefix(s.Anchor, fedora) {
		t.Fatalf("anchor = %s", s.Anchor)
	}
	os.MkdirAll(debian, 0o755)
	if s, _ := findTrustStore(); s.Anchor != filepath.Join(debian, "bay.crt") {
		t.Fatalf("debian first: %s", s.Anchor)
	}

	os.WriteFile(filepath.Join(l.Dir, "ca.crt"), []byte("CA"), 0o644)
	if it := l.trustItem(); it.OK || !strings.Contains(it.Text, "update-ca-certificates") {
		t.Fatalf("untrusted: %+v", it)
	}
	os.WriteFile(filepath.Join(debian, "bay.crt"), []byte("CA"), 0o644)
	if it := l.trustItem(); !it.OK {
		t.Fatalf("trusted: %+v", it)
	}
}

// Without systemd the firewall and the agent are reported, with no step.
func TestWithoutSystemd(t *testing.T) {
	useFirewallDirs(t)
	systemdRuns = func() bool { return false }
	l := LinuxSetup{Dir: t.TempDir(), OnlyFrom: []string{"192.168.2.1"}, Interface: "eth0", DNSBin: "/b"}
	for _, it := range []SetupItem{l.firewallItem(), l.agentItem()} {
		if it.OK || it.Text != "" || !strings.Contains(it.Name, "systemd") {
			t.Fatalf("item = %+v", it)
		}
	}
}
