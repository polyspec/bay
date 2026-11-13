package stack

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A machine that is not macOS has a smaller setup: its certificate authority,
// which its proxy's certificates and its peer link are issued from, the Docker
// engine its projects run on, and, when it is asked for, a firewall that
// accepts new connections on the network only from the hosts it names.
//
// The firewall is for a machine that is reached through one other machine,
// such as a board attached to a Mac that shares its network: nothing else on
// the network should reach it, and it should reach the network as before.
// It guards the interface holding the default route and nothing else, so the
// containers on the machine's own bridges are not affected. It is an nftables
// table of its own, loaded by a systemd unit, so it is in place after a restart
// and does not touch the rules Docker writes.

// firewallDir and systemdDir are where the firewall is written. Tests replace
// them.
var (
	firewallDir = "/etc/bay"
	systemdDir  = "/etc/systemd/system"
)

const (
	firewallTable = "bay"
	firewallUnit  = "bay-firewall.service"
)

func firewallPath() string     { return filepath.Join(firewallDir, "firewall.nft") }
func firewallUnitPath() string { return filepath.Join(systemdDir, firewallUnit) }

// LinuxSetup is the setup of a machine that is not macOS.
type LinuxSetup struct {
	// Dir is the state directory.
	Dir string
	// OnlyFrom are the hosts and networks that may open connections to this
	// machine on the guarded interface. Empty means no firewall.
	OnlyFrom []string
	// Interface is the interface guarded, the one holding the default route.
	Interface string
	// DNSBin is the bayd binary the agent runs, Addr where it listens
	// and Domains the domains it serves. An empty DNSBin leaves the agent out,
	// which is what a reader without bayd sees.
	DNSBin  string
	Addr    string
	Domains []string
}

// NewLinuxSetup reads the setup this machine is configured for. dnsBin is the
// bayd the agent runs and addr where it listens.
func NewLinuxSetup(m *Machine, dnsBin, addr string) (LinuxSetup, error) {
	s, err := m.Settings()
	if err != nil {
		return LinuxSetup{}, err
	}
	domains, err := m.Domains()
	if err != nil {
		return LinuxSetup{}, err
	}
	return LinuxSetup{Dir: m.Dir, OnlyFrom: s.OnlyFrom, Interface: defaultRouteInterface(),
		DNSBin: dnsBin, Addr: addr, Domains: domains}, nil
}

// ParseOnlyFrom reads a comma-separated list of addresses and networks. "none"
// is the empty list.
func ParseOnlyFrom(list string) ([]string, error) {
	if strings.TrimSpace(list) == "none" {
		return nil, nil
	}
	var out []string
	for _, a := range strings.Split(list, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if net.ParseIP(a) == nil {
			if _, _, err := net.ParseCIDR(a); err != nil {
				return nil, fmt.Errorf("%q is not an address or a network", a)
			}
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("name at least one address, or none")
	}
	return out, nil
}

// SetOnlyFrom records the hosts the firewall accepts connections from. It
// changes no rule; "bay install" applies it.
func (m *Machine) SetOnlyFrom(hosts []string) error {
	s, err := m.Settings()
	if err != nil {
		return err
	}
	s.OnlyFrom = hosts
	return m.SaveSettings(s)
}

// Items lists every part of this machine's setup and whether it is in place.
func (l LinuxSetup) Items() []SetupItem {
	_, err := os.Stat(filepath.Join(l.Dir, "ca.crt"))
	items := []SetupItem{{
		Name: "certificate authority", OK: err == nil,
		SetupStep: SetupStep{Text: "create local certificate authority"},
	}}
	docker := SetupItem{Name: "docker engine", OK: dockerAnswers()}
	if !docker.OK {
		docker.Name = "docker engine: not answering; install Docker and start it"
	}
	items = append(items, docker, l.trustItem(), TrustItem(l.Dir))
	items = append(items, l.firewallItem())
	if l.DNSBin != "" {
		items = append(items, l.agentItem())
	}
	items = append(items, l.hostsItem())
	if docker.OK {
		items = append(items, l.proxyNamesItem())
	}
	return items
}

// proxyNamesItem is the names the proxy answers to on its Docker network, which
// a container on that network resolves through Docker's DNS. They are the
// names in /etc/hosts. The proxy is rewritten to correct them, which needs no
// root.
func (l LinuxSetup) proxyNamesItem() SetupItem {
	want, err := HostsNames(NewMachine(l.Dir))
	item := SetupItem{
		Name:      "names on the " + proxyNetwork(DockerEngine) + " network (" + strings.Join(want, ", ") + ")",
		SetupStep: SetupStep{Text: "rewrite the proxy so it answers to them"},
	}
	if err != nil {
		return item
	}
	got, err := ProxyNames(DockerEngine)
	if err != nil {
		item.Name = "names on the " + proxyNetwork(DockerEngine) + " network: the proxy is not running"
		item.OK = len(want) == 0
		return item
	}
	item.OK = strings.Join(got, " ") == strings.Join(want, " ")
	return item
}

// agentItem is the resident agent: bayd under systemd. It reads what
// domains the approved peers hold, rewrites the proxy when they change and
// keeps their names in /etc/hosts.
func (l LinuxSetup) agentItem() SetupItem {
	if !systemdRuns() {
		return SetupItem{Name: "agent " + agentUnit + ": systemd does not run this system; install cannot register the agent"}
	}
	return SetupItem{
		Name: "agent " + agentUnit, OK: l.agentCurrent(),
		SetupStep: SetupStep{Text: "register " + agentUnitPath(), Asks: AsksAdministrator},
	}
}

func (l LinuxSetup) agentCurrent() bool {
	unit, err := os.ReadFile(agentUnitPath())
	if err != nil || string(unit) != l.agentUnitBody() {
		return false
	}
	return exec.Command("systemctl", "is-enabled", "--quiet", agentUnit).Run() == nil &&
		exec.Command("systemctl", "is-active", "--quiet", agentUnit).Run() == nil &&
		!residentStale(systemdPID(agentUnit), l.DNSBin)
}

// hostsItem is the block of /etc/hosts naming what the proxy serves.
func (l LinuxSetup) hostsItem() SetupItem {
	names, err := HostsNames(NewMachine(l.Dir))
	item := SetupItem{
		Name:      "names in " + hostsPath + " (" + strings.Join(names, ", ") + ")",
		OK:        err == nil && HostsCurrent(names),
		SetupStep: SetupStep{Text: "write the names the proxy serves in " + hostsPath, Asks: AsksAdministrator},
	}
	if len(names) == 0 {
		item.Name = "names in " + hostsPath + " (none to name)"
	}
	return item
}

const agentUnit = "bay-agent.service"

func agentUnitPath() string { return filepath.Join(systemdDir, agentUnit) }

// agentUnitBody is the agent's systemd unit. It restarts the agent when it
// stops, and starts after Docker, whose proxy it rewrites.
func (l LinuxSetup) agentUnitBody() string {
	args := append(dnsAgentArgs(l.DNSBin, strings.Join(l.Domains, ","), l.Addr, l.Dir), "-hosts")
	return "# Written by \"bay install\".\n" +
		"[Unit]\n" +
		"Description=bay agent: follows the approved peers and names what the proxy serves in /etc/hosts\n" +
		"After=network-online.target docker.service\n" +
		"Wants=network-online.target\n" +
		"\n[Service]\n" +
		"ExecStart=" + strings.Join(args, " ") + "\n" +
		"Restart=always\n" +
		"RestartSec=5\n" +
		"\n[Install]\n" +
		"WantedBy=multi-user.target\n"
}

// A Linux system's certificate store is updated by one of two tools, each
// reading local authorities from its own directory. Which one a system has is
// read from the system, not assumed:
//
//   - update-ca-certificates (Debian, Ubuntu, Alpine) reads
//     /usr/local/share/ca-certificates.
//   - update-ca-trust (Fedora, RHEL and their relatives, Arch) reads
//     /etc/pki/ca-trust/source/anchors, or on Arch
//     /etc/ca-certificates/trust-source/anchors.
//
// A system with neither has no store install knows how to change. check and
// doctor say so and name what to install, and install changes nothing.
type trustStore struct {
	Tool   string   // the command that rebuilds the store
	Args   []string // its arguments
	Anchor string   // where the authority is written
}

// trustStores are the stores install knows, in the order they are looked for.
// Tests replace it.
var trustStores = []struct {
	tool string
	args []string
	dirs []string
}{
	{"update-ca-certificates", nil, []string{"/usr/local/share/ca-certificates"}},
	{"update-ca-trust", []string{"extract"}, []string{"/etc/pki/ca-trust/source/anchors", "/etc/ca-certificates/trust-source/anchors"}},
}

// findTrustStore returns the store this system has. The first tool on the
// search path whose directory exists, or whose first directory's parent does,
// is the one.
func findTrustStore() (trustStore, bool) {
	for _, s := range trustStores {
		tool, err := exec.LookPath(s.tool)
		if err != nil {
			continue
		}
		for _, d := range s.dirs {
			if dirExists(d) || dirExists(filepath.Dir(d)) {
				return trustStore{Tool: tool, Args: s.args, Anchor: filepath.Join(d, "bay.crt")}, true
			}
		}
	}
	return trustStore{}, false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// trustItem is the machine's authority in the system's certificate store, so
// a program on the machine verifies the names its proxy serves. The proxy
// serves a peer's names with certificates from this authority too.
func (l LinuxSetup) trustItem() SetupItem {
	name := "trust " + filepath.Join(l.Dir, "ca.crt")
	store, ok := findTrustStore()
	if !ok {
		return SetupItem{Name: name + ": no certificate store install knows " +
			"(neither update-ca-certificates nor update-ca-trust); install ca-certificates"}
	}
	return SetupItem{
		Name: name + " in " + store.Anchor, OK: l.trusted(store),
		SetupStep: SetupStep{Text: "add the authority to " + store.Anchor + " and run " + filepath.Base(store.Tool), Asks: AsksAdministrator},
	}
}

func (l LinuxSetup) trusted(store trustStore) bool {
	want, err := os.ReadFile(filepath.Join(l.Dir, "ca.crt"))
	if err != nil {
		return false
	}
	got, err := os.ReadFile(store.Anchor)
	return err == nil && string(got) == string(want)
}

func (l LinuxSetup) trust() error {
	store, ok := findTrustStore()
	if !ok {
		return fmt.Errorf("no certificate store install knows")
	}
	ca, err := os.ReadFile(filepath.Join(l.Dir, "ca.crt"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(store.Anchor), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(store.Anchor, ca, 0o644); err != nil {
		return err
	}
	if out, err := exec.Command(store.Tool, store.Args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", filepath.Base(store.Tool), strings.TrimSpace(string(out)))
	}
	fmt.Printf("trusted %s\n", store.Anchor)
	return nil
}

// untrust removes the authority from the system's store. It runs as root.
func untrust() error {
	store, ok := findTrustStore()
	if !ok {
		return nil
	}
	if _, err := os.Stat(store.Anchor); err != nil {
		return nil
	}
	if err := os.Remove(store.Anchor); err != nil {
		return err
	}
	if out, err := exec.Command(store.Tool, store.Args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", filepath.Base(store.Tool), strings.TrimSpace(string(out)))
	}
	fmt.Printf("removed %s\n", store.Anchor)
	return nil
}

// systemdRuns reports whether systemd manages this system, which the firewall
// and the agent are registered with. Tests replace it.
var systemdRuns = func() bool { return dirExists("/run/systemd/system") }

// rootPending reports whether any step that needs root is missing.
func (l LinuxSetup) rootPending() bool {
	for _, it := range l.Items() {
		if !it.OK && it.Asks == AsksAdministrator {
			return true
		}
	}
	return false
}

// dockerAnswers reports whether the Docker engine on this machine answers.
// dockerAnswers asks the engine for its version, which it answers without
// work. "docker info" also sums the disk use and took 20s on a loaded machine,
// past the read bound, so a busy engine was reported as not answering.
func dockerAnswers() bool {
	_, err := runEngineWithin(engineReadWait, DockerEngine, "version", "--format", "{{.Server.Version}}")
	return err == nil
}

func (l LinuxSetup) firewallItem() SetupItem {
	if len(l.OnlyFrom) == 0 {
		_, file := os.Stat(firewallPath())
		_, unit := os.Stat(firewallUnitPath())
		return SetupItem{
			Name: "firewall (none asked for)", OK: file != nil && unit != nil,
			SetupStep: SetupStep{Text: "remove the firewall " + firewallUnit, Asks: AsksAdministrator},
		}
	}
	name := "firewall " + strings.Join(l.OnlyFrom, ", ") + " only"
	if !systemdRuns() {
		return SetupItem{Name: name + ": systemd does not run this system; install cannot load the firewall at boot"}
	}
	if _, err := exec.LookPath("nft"); err != nil {
		return SetupItem{Name: name + ": nft is not installed; install nftables"}
	}
	if l.Interface == "" {
		return SetupItem{Name: name + ": no interface holds the default route"}
	}
	name += " on " + l.Interface
	return SetupItem{
		Name: name, OK: l.firewallCurrent(),
		SetupStep: SetupStep{
			Text: "accept new connections on " + l.Interface + " only from " + strings.Join(l.OnlyFrom, ", "),
			Asks: AsksAdministrator,
		},
	}
}

// firewallCurrent reports whether the written rules and unit are the ones this
// setup asks for, the unit starts at boot and the table is loaded.
func (l LinuxSetup) firewallCurrent() bool {
	rules, err := os.ReadFile(firewallPath())
	if err != nil || string(rules) != firewallRules(l.Interface, l.OnlyFrom) {
		return false
	}
	unit, err := os.ReadFile(firewallUnitPath())
	if err != nil || string(unit) != firewallUnitBody() {
		return false
	}
	if exec.Command("systemctl", "is-enabled", "--quiet", firewallUnit).Run() != nil {
		return false
	}
	return exec.Command("nft", "list", "table", "inet", firewallTable).Run() == nil
}

// Pending lists the steps install takes. An item install cannot fix is not
// one of them.
func (l LinuxSetup) Pending() []SetupItem {
	var out []SetupItem
	for _, item := range l.Items() {
		if !item.OK {
			out = append(out, item)
		}
	}
	return out
}

// Ensure applies what is missing. The authority is created as the user who
// owns the state directory; the firewall needs root and is applied by
// re-running the helper as root when this process is not.
func (l LinuxSetup) Ensure(helper string, elevate Elevator) error {
	if _, err := LoadOrCreateCA(l.Dir); err != nil {
		return err
	}
	if err := l.ensureRoot(helper, elevate); err != nil {
		return err
	}
	// The containers' store is written after the system's, whose roots it
	// carries.
	if err := EnsureTrust(l.Dir); err != nil {
		return err
	}
	if docker := dockerAnswers(); docker {
		if it := l.proxyNamesItem(); !it.OK {
			if _, err := SyncProxy(NewMachine(l.Dir)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l LinuxSetup) ensureRoot(helper string, elevate Elevator) error {
	if !l.rootPending() {
		return nil
	}
	if os.Geteuid() == 0 {
		return l.ApplyRoot()
	}
	fmt.Fprintf(os.Stderr, "administrator rights are needed once to:\n")
	for _, it := range l.Items() {
		if !it.OK && it.Asks == AsksAdministrator {
			fmt.Fprintf(os.Stderr, "  - %s\n", it.Text)
		}
	}
	args := []string{"-privileged-apply", "-state", l.Dir, "-addr", l.Addr}
	if l.DNSBin != "" {
		args = append(args, "-dnsbin", l.DNSBin)
	}
	return Install{Helper: helper, Elevate: elevate}.elevate(args...)
}

// ApplyRoot applies the steps that need root and are missing: the firewall,
// the agent and the names in /etc/hosts. It runs as root.
func (l LinuxSetup) ApplyRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}
	if t := l.trustItem(); !t.OK && t.Text != "" {
		if err := l.trust(); err != nil {
			return err
		}
	}
	if fw := l.firewallItem(); !fw.OK && fw.Text != "" {
		if err := l.ApplyFirewall(); err != nil {
			return err
		}
	}
	if l.DNSBin != "" && !l.agentCurrent() {
		if err := l.installAgent(); err != nil {
			return err
		}
	}
	names, err := HostsNames(NewMachine(l.Dir))
	if err != nil {
		return err
	}
	if wrote, err := SyncHostsFile(names); err != nil {
		return err
	} else if wrote {
		fmt.Printf("named %d domain(s) in %s\n", len(names), hostsPath)
	}
	return nil
}

func (l LinuxSetup) installAgent() error {
	if err := os.WriteFile(agentUnitPath(), []byte(l.agentUnitBody()), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"daemon-reload"}, {"enable", agentUnit}, {"restart", agentUnit},
	} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("registered %s\n", agentUnitPath())
	return nil
}

// RemoveAgent stops the agent, removes its unit, the names it wrote and the
// authority from the system's store. It runs as root.
func RemoveAgent() error {
	if _, err := os.Stat(agentUnitPath()); err == nil {
		exec.Command("systemctl", "disable", "--now", agentUnit).Run()
		if err := os.Remove(agentUnitPath()); err != nil {
			return err
		}
		exec.Command("systemctl", "daemon-reload").Run()
		fmt.Printf("removed %s\n", agentUnitPath())
	}
	if _, err := SyncHostsFile(nil); err != nil {
		return err
	}
	return untrust()
}

// ApplyFirewall writes, loads and enables the firewall this setup asks for, or
// removes it when none is asked for. It runs as root.
func (l LinuxSetup) ApplyFirewall() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}
	if len(l.OnlyFrom) == 0 {
		return removeFirewall()
	}
	if l.Interface == "" {
		return fmt.Errorf("no interface holds the default route")
	}
	if err := os.MkdirAll(firewallDir, 0o755); err != nil {
		return err
	}
	rules := firewallRules(l.Interface, l.OnlyFrom)
	if out, err := checkRules(rules); err != nil {
		return fmt.Errorf("the firewall rules do not load: %s", out)
	}
	if err := os.WriteFile(firewallPath(), []byte(rules), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(firewallUnitPath(), []byte(firewallUnitBody()), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"daemon-reload"}, {"enable", firewallUnit}, {"restart", firewallUnit},
	} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
		}
	}
	fmt.Printf("firewall: %s accepts new connections only from %s\n", l.Interface, strings.Join(l.OnlyFrom, ", "))
	return nil
}

// checkRules asks nft whether the rules load, without loading them.
func checkRules(rules string) (string, error) {
	cmd := exec.Command("nft", "-c", "-f", "-")
	cmd.Stdin = strings.NewReader(rules)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func removeFirewall() error {
	_, file := os.Stat(firewallPath())
	_, unit := os.Stat(firewallUnitPath())
	if file != nil && unit != nil {
		return nil
	}
	exec.Command("systemctl", "disable", "--now", firewallUnit).Run()
	exec.Command("nft", "delete", "table", "inet", firewallTable).Run()
	for _, p := range []string{firewallUnitPath(), firewallPath()} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	exec.Command("systemctl", "daemon-reload").Run()
	fmt.Printf("firewall: removed %s\n", firewallUnit)
	return nil
}

// firewallRules returns the nftables rules guarding one interface. Replies to
// connections this machine opened are accepted, so it reaches the network as
// before; so are neighbour discovery and the answers of DHCP, without which it
// loses its address. Everything else that arrives on the interface is dropped
// unless it comes from a listed host. The forward hook is guarded as well,
// because a published container port is reached through it.
func firewallRules(iface string, allowed []string) string {
	var v4, v6 []string
	for _, a := range allowed {
		if strings.Contains(a, ":") {
			v6 = append(v6, a)
		} else {
			v4 = append(v4, a)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Written by \"bay install\". %s accepts new connections only from %s.\n",
		iface, strings.Join(allowed, ", "))
	b.WriteString("# Change it with \"bay install -only-from\"; an edit here is replaced.\n")
	fmt.Fprintf(&b, "table inet %s\ndelete table inet %s\ntable inet %s {\n", firewallTable, firewallTable, firewallTable)
	b.WriteString("  chain guard {\n")
	b.WriteString("    ct state established,related accept\n")
	if len(v4) > 0 {
		fmt.Fprintf(&b, "    ip saddr { %s } accept\n", strings.Join(v4, ", "))
	}
	if len(v6) > 0 {
		fmt.Fprintf(&b, "    ip6 saddr { %s } accept\n", strings.Join(v6, ", "))
	}
	b.WriteString("    ip6 nexthdr icmpv6 accept\n")
	b.WriteString("    udp sport 67 udp dport 68 accept\n")
	b.WriteString("    udp sport 547 udp dport 546 accept\n")
	b.WriteString("    drop\n  }\n")
	fmt.Fprintf(&b, "  chain input   { type filter hook input   priority -10; policy accept; iifname %q jump guard; }\n", iface)
	fmt.Fprintf(&b, "  chain forward { type filter hook forward priority -10; policy accept; iifname %q jump guard; }\n", iface)
	b.WriteString("}\n")
	return b.String()
}

// firewallUnitBody is the systemd unit that loads the rules at boot, before
// the network comes up.
func firewallUnitBody() string {
	return "# Written by \"bay install\".\n" +
		"[Unit]\n" +
		"Description=bay firewall: accept new connections only from the listed hosts\n" +
		"DefaultDependencies=no\n" +
		"Before=network-pre.target\n" +
		"Wants=network-pre.target\n" +
		"\n[Service]\n" +
		"Type=oneshot\n" +
		"RemainAfterExit=yes\n" +
		"ExecStart=/usr/sbin/nft -f " + firewallPath() + "\n" +
		"ExecStop=/usr/sbin/nft delete table inet " + firewallTable + "\n" +
		"\n[Install]\n" +
		"WantedBy=multi-user.target\n"
}
