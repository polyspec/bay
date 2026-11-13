package stack

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useFirewallDirs points the firewall's files at a temporary directory.
func useFirewallDirs(t *testing.T) {
	t.Helper()
	fw, sd, ts, sr := firewallDir, systemdDir, trustStores, systemdRuns
	firewallDir, systemdDir = t.TempDir(), t.TempDir()
	trustStores = trustStores[:0:0]
	systemdRuns = func() bool { return true }
	t.Cleanup(func() { firewallDir, systemdDir, trustStores, systemdRuns = fw, sd, ts, sr })
}

// The record the user writes cannot move root's listener off this machine or
// to another port.
func TestTheRelayListensOnlyOnThisMachineAtItsPort(t *testing.T) {
	local := []net.IP{net.ParseIP("192.168.65.1"), net.ParseIP("127.0.0.1")}
	for address, want := range map[string]bool{
		"192.168.65.1:8444": true,
		"192.168.65.1:22":   false,
		"192.168.2.3:8444":  false,
		"0.0.0.0:8444":      false,
		"":                  false,
		"192.168.65.1":      false,
	} {
		if got := relayListenAllowed(address, local); got != want {
			t.Errorf("%q: got %v, want %v", address, got, want)
		}
	}
}

func TestTheRelayAddressRecord(t *testing.T) {
	dir := t.TempDir()
	if err := RecordRelayAddress(dir, "127.0.0.1:8444"); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(dir, relayAddressFile))
	if err := RecordRelayAddress(dir, "127.0.0.1:8444"); err != nil {
		t.Fatal(err)
	}
	again, _ := os.Stat(filepath.Join(dir, relayAddressFile))
	if !info.ModTime().Equal(again.ModTime()) {
		t.Fatal("recording the same address rewrote the record")
	}
	if got := RecordedRelayAddress(dir); got != "127.0.0.1:8444" {
		t.Fatalf("got %q", got)
	}
	if err := RecordRelayAddress(dir, ""); err != nil {
		t.Fatal(err)
	}
	if got := RecordedRelayAddress(dir); got != "" {
		t.Fatalf("a removed record still says %q", got)
	}
}

// The daemon's job is compared byte for byte, so a moved binary or another
// state directory is out of date and install registers it again.
func TestTheRelayDaemonIsCurrentOnlyForItsBinaryAndState(t *testing.T) {
	saved := launchDaemonDir
	launchDaemonDir = t.TempDir()
	t.Cleanup(func() { launchDaemonDir = saved })

	if RelayDaemonInstalled() || RelayDaemonCurrent("/opt/bin/bayd", "/u/.bay") {
		t.Fatal("a daemon is reported with no job written")
	}
	os.WriteFile(RelayDaemonPath(), []byte(relayDaemonPlist("/opt/bin/bayd", "/u/.bay")), 0o644)
	if !RelayDaemonCurrent("/opt/bin/bayd", "/u/.bay") {
		t.Fatal("the written job is not current")
	}
	if RelayDaemonCurrent("/usr/local/bin/bayd", "/u/.bay") ||
		RelayDaemonCurrent("/opt/bin/bayd", "/v/.bay") {
		t.Fatal("a job for another binary or state directory is current")
	}
	if !strings.Contains(relayDaemonPlist("/b", "/s"), "<string>-relay</string>") {
		t.Fatal("the job does not run the relay")
	}
}

// Without RelayBin the setup leaves the daemon out, which is what a reader
// with no bayd sees; with it the daemon is a step that needs root.
func TestTheRelayIsPartOfTheMacSetup(t *testing.T) {
	saved := launchDaemonDir
	launchDaemonDir = t.TempDir()
	t.Cleanup(func() { launchDaemonDir = saved })

	for _, it := range (Install{}).Items() {
		if strings.HasPrefix(it.Name, "relay") {
			t.Fatal("the relay is listed without a binary")
		}
	}
	in := Install{RelayBin: "/b/bayd", State: "/s"}
	var relay *SetupItem
	for _, it := range in.Items() {
		if strings.HasPrefix(it.Name, "relay") {
			it := it
			relay = &it
		}
	}
	if relay == nil || relay.OK || relay.Asks != AsksAdministrator {
		t.Fatalf("relay item = %+v", relay)
	}
	if steps := in.privilegedSteps(); len(steps) != 1 || !strings.Contains(steps[0], RelayDaemonLabel) {
		t.Fatalf("privileged steps = %v", steps)
	}
}

// The administrator dialog runs the helper with each argument quoted, so a
// path with a space or a quote reaches the helper as one argument.
func TestAdminScriptQuotesEveryArgument(t *testing.T) {
	got := adminScript("/opt/homebrew/bin/bay", []string{"-state", `/Users/a b/it's`, "-domain", "test"})
	want := `do shell script "'/opt/homebrew/bin/bay' '-state' '/Users/a b/it'\\''s' '-domain' 'test' 2>&1" with administrator privileges`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
