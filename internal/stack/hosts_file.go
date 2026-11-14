package stack

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A machine that is not macOS names the domains its proxy serves in
// /etc/hosts. It has no per-domain resolver to delegate a domain to, and the
// names are a known list: the domains of its own projects and the domains its
// approved peers hold. Each is given 127.0.0.1, where the proxy listens, and
// the proxy serves a project's name itself and forwards a peer's over the
// peer's link.
//
// The block also names each running service container on Docker at
// <container>.bay.test with its address, as macOS's DNS agent answers it, so a
// program on the host reaches an internal service, such as a database, at the
// name a container uses. Docker's bridge addresses are reachable from the
// host. A container's address changes when it starts again, and the agent
// follows it.
//
// The names are kept in one block between two marker lines. Nothing outside the
// block is changed, the block is rewritten only when its entries change, and a
// machine with no names has no block. The agent keeps it current, because a
// peer's domains change on that peer.

// hostsPath is the system's hosts file. Tests replace it.
var hostsPath = "/etc/hosts"

const (
	hostsBegin   = "# bay begin: the names this machine's proxy serves. Written by bay; an edit here is replaced."
	hostsEnd     = "# bay end"
	hostsAddress = "127.0.0.1"
)

// HostsEntry is one line of the block.
type HostsEntry struct{ Address, Name string }

// HostsNames returns the names this machine's proxy serves: the domains of its
// running routes, which its proxy and its peer document are written from, and
// those its approved peers hold.
func HostsNames(m *Machine) ([]string, error) {
	instances, err := hostsInstances()
	if err != nil {
		return nil, err
	}
	routes, _ := routesFrom(instances)
	peers, err := Peers(m.Dir)
	if err != nil {
		return nil, err
	}
	return hostsNamesFrom(routes, peers), nil
}

// HostsEntries returns the block's lines: the names the proxy serves at
// 127.0.0.1, then each running service container on Docker at its address.
func HostsEntries(m *Machine) ([]HostsEntry, error) {
	instances, err := hostsInstances()
	if err != nil {
		return nil, err
	}
	routes, _ := routesFrom(instances)
	peers, err := Peers(m.Dir)
	if err != nil {
		return nil, err
	}
	return hostsEntriesFrom(hostsNamesFrom(routes, peers), instances), nil
}

// hostsInstances is Instances. Tests replace it.
var hostsInstances = Instances

func hostsEntriesFrom(names []string, instances []ServiceInstance) []HostsEntry {
	entries := make([]HostsEntry, 0, len(names)+len(instances))
	for _, n := range names {
		entries = append(entries, HostsEntry{hostsAddress, n})
	}
	var backends []HostsEntry
	for _, in := range instances {
		if in.Running() && in.Engine == DockerEngine && in.IPv4 != "" {
			backends = append(backends, HostsEntry{in.IPv4, strings.ToLower(in.Container) + "." + BackendDomain})
		}
	}
	sort.Slice(backends, func(i, j int) bool { return backends[i].Name < backends[j].Name })
	return append(entries, backends...)
}

// ServiceHostNames are the entries that name a service container, not a
// domain the proxy serves.
func ServiceHostNames(entries []HostsEntry) int {
	n := 0
	for _, e := range entries {
		if e.Address != hostsAddress {
			n++
		}
	}
	return n
}

func hostsNamesFrom(routes []Route, peers []Peer) []string {
	seen := map[string]bool{}
	var own []string
	for _, r := range routes {
		d := strings.ToLower(r.Domain)
		if !seen[d] {
			seen[d] = true
			own = append(own, d)
		}
	}
	names := append([]string{}, own...)
	for d := range PeerDomains(peers, own) {
		names = append(names, d)
	}
	sort.Strings(names)
	return names
}

// hostsBlock returns the block of these entries, or nothing for none.
func hostsBlock(entries []HostsEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(hostsBegin + "\n")
	for _, e := range entries {
		b.WriteString(e.Address + "\t" + e.Name + "\n")
	}
	b.WriteString(hostsEnd + "\n")
	return b.String()
}

// withHostsBlock returns the hosts file with its block replaced by block. The
// lines outside the block are kept as they are.
func withHostsBlock(current, block string) string {
	var out strings.Builder
	inside := false
	for _, line := range strings.SplitAfter(current, "\n") {
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case trimmed == hostsBegin:
			inside = true
		case inside && trimmed == hostsEnd:
			inside = false
		case !inside:
			out.WriteString(line)
		}
	}
	s := out.String()
	if block == "" {
		return s
	}
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s + block
}

// HostsCurrent reports whether the hosts file holds exactly these entries.
func HostsCurrent(entries []HostsEntry) bool {
	b, err := os.ReadFile(hostsPath)
	if err != nil {
		return false
	}
	return withHostsBlock(string(b), hostsBlock(entries)) == string(b)
}

// SyncHostsFile writes the block for these entries when it differs and reports
// whether it wrote. The file is replaced in one step so no reader sees half of
// it.
func SyncHostsFile(entries []HostsEntry) (bool, error) {
	b, err := os.ReadFile(hostsPath)
	if err != nil {
		return false, err
	}
	next := withHostsBlock(string(b), hostsBlock(entries))
	if next == string(b) {
		return false, nil
	}
	info, err := os.Stat(hostsPath)
	if err != nil {
		return false, err
	}
	tmp := filepath.Join(filepath.Dir(hostsPath), ".hosts.bay")
	if err := os.WriteFile(tmp, []byte(next), info.Mode().Perm()); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, hostsPath); err != nil {
		os.Remove(tmp)
		return false, err
	}
	return true, nil
}
