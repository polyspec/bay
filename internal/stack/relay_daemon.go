package stack

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The relay runs as a launchd daemon, not in the DNS agent.
//
// macOS refuses a connection to the local network from a process launchd starts
// for a user, unless the user allowed it in Privacy & Security > Local Network,
// and it asks no one for a program that is not an application: the connection
// fails with "no route to host". A peer on the network the Mac shares, or on
// any network the Mac is directly attached to, is on the local network. A
// launchd daemon is not subject to that rule, so the relay is one.
//
// A daemon runs as root and cannot read the Apple `container` network, which
// belongs to the user's session. The DNS agent reads the gateway of the
// proxy's network and records it in the state directory; the daemon listens
// where the record says, and only on an address of this machine at
// PeerRelayPort.

// RelayDaemonLabel identifies the launchd daemon that runs the relay.
const RelayDaemonLabel = "dev.bay.relay"

// launchDaemonDir is where launchd reads daemons from. Tests replace it.
var launchDaemonDir = "/Library/LaunchDaemons"

// relayLogPath is where the daemon writes. A file root creates in the user's
// state directory would be one the user cannot remove, so the daemon writes
// under /var/log.
const relayLogPath = "/var/log/bay-relay.log"

// relayAddressFile is the record, in the state directory, of where the relay
// listens.
const relayAddressFile = "peer-relay"

// RelayDaemonPath is the daemon's launchd job.
func RelayDaemonPath() string {
	return filepath.Join(launchDaemonDir, RelayDaemonLabel+".plist")
}

// relayDaemonArgs are the arguments the daemon is registered with.
func relayDaemonArgs(bin, state string) []string {
	return []string{bin, "-relay", "-state", state}
}

func relayDaemonPlist(bin, state string) string {
	return buildPlist(RelayDaemonLabel, relayDaemonArgs(bin, state), relayLogPath, relayLogPath)
}

// RelayDaemonCurrent reports whether the registered daemon runs this binary
// for this state directory.
func RelayDaemonCurrent(bin, state string) bool {
	b, err := os.ReadFile(RelayDaemonPath())
	return err == nil && string(b) == relayDaemonPlist(bin, state)
}

// RelayDaemonInstalled reports whether a relay daemon is registered.
func RelayDaemonInstalled() bool {
	_, err := os.Stat(RelayDaemonPath())
	return err == nil
}

// installRelayDaemon writes the daemon's job and loads it. It runs as root.
func installRelayDaemon(bin, state string) error {
	if bin == "" || state == "" {
		return fmt.Errorf("the relay daemon needs the bayd binary and the state directory")
	}
	path := RelayDaemonPath()
	if err := os.WriteFile(path, []byte(relayDaemonPlist(bin, state)), 0o644); err != nil {
		return err
	}
	bootoutDaemon()
	if out, err := exec.Command("launchctl", "bootstrap", "system", path).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap system %s: %s", path, strings.TrimSpace(string(out)))
	}
	fmt.Printf("registered %s\n", path)
	return nil
}

// removeRelayDaemon stops the daemon and removes its job. It runs as root.
func removeRelayDaemon() error {
	if !RelayDaemonInstalled() {
		return nil
	}
	bootoutDaemon()
	if err := os.Remove(RelayDaemonPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("removed %s\n", RelayDaemonPath())
	return nil
}

// UninstallRelayDaemon removes the relay daemon, acquiring root if needed.
// helper is the executable to re-run as root; empty means this one.
func UninstallRelayDaemon(helper string, elevate Elevator) error {
	if !RelayDaemonInstalled() {
		return nil
	}
	if os.Geteuid() == 0 {
		return removeRelayDaemon()
	}
	return Install{Helper: helper, Elevate: elevate}.elevate("-privileged-remove-relay")
}

// bootoutDaemon stops the loaded daemon and waits until launchd no longer
// knows it, because bootstrapping a job that is still there fails.
func bootoutDaemon() {
	target := "system/" + RelayDaemonLabel
	exec.Command("launchctl", "bootout", target).Run()
	deadline := time.Now().Add(5 * time.Second)
	for exec.Command("launchctl", "print", target).Run() == nil && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
}

// RelayDaemonLoaded reports whether launchd runs the daemon.
func RelayDaemonLoaded() bool {
	return exec.Command("launchctl", "print", "system/"+RelayDaemonLabel).Run() == nil
}

// RecordRelayAddress records where the relay listens. An empty address removes
// the record. A record that already says it is left alone.
func RecordRelayAddress(dir, address string) error {
	path := filepath.Join(dir, relayAddressFile)
	if address == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if b, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(b)) == address {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(address+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RecordedRelayAddress returns where the record says the relay listens, or an
// empty string when there is no record or it names an address the relay may
// not listen on.
func RecordedRelayAddress(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, relayAddressFile))
	if err != nil {
		return ""
	}
	address := strings.TrimSpace(string(b))
	if !relayListenAllowed(address, localAddresses()) {
		return ""
	}
	return address
}

// relayListenAllowed reports whether the relay may listen on address: an
// address of this machine at PeerRelayPort. The record is written by the user
// and read by root, so it cannot move root's listener anywhere else.
func relayListenAllowed(address string, local []net.IP) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != strconv.Itoa(PeerRelayPort) {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return false
	}
	for _, l := range local {
		if l.Equal(ip) {
			return true
		}
	}
	return false
}
