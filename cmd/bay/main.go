// bay runs groups of Apple `container` containers behind one nginx
// that terminates TLS for every domain they claim.
//
// A group is one stack file. The proxy, the DNS server, the CA and the resolver
// entries are machine-level and shared: bringing a group up registers its
// routes with them, bringing it down withdraws only its own. The proxy's
// configuration is derived from the labels on the running containers, so no two
// groups can fight over a configuration file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/polyspec/bay/internal/contract"
	"github.com/polyspec/bay/internal/stack"
)

var (
	file   = flag.String("f", ".", "Compose file, or a directory to find one in")
	dir    = flag.String("state", defaultState(), "directory holding the CA, certificates and machine state")
	addr   = flag.String("addr", stack.DefaultDNSAddr, "address bayd listens on")
	dnsBin = flag.String("dnsbin", "", "path to the bayd binary (default: next to this one)")

	// Set when this binary re-runs itself as root; not part of the user interface.
	privApply   = flag.Bool("privileged-apply", false, "internal: apply the privileged setup steps")
	privRemove  = flag.Bool("privileged-remove", false, "internal: remove resolver entries")
	privDomain  = flag.String("domain", "", "internal: comma-separated domains for the privileged steps")
	privRetire  = flag.String("retire", "", "internal: comma-separated domains whose resolver entries are removed")
	privCA      = flag.String("ca", "", "internal: CA certificate for the privileged steps")
	privUntrust = flag.String("untrust", "", "internal: retired CA certificate to stop trusting")
)

func main() {
	flag.Usage = usage
	flag.Parse()

	if *privApply || *privRemove {
		if err := privileged(); err != nil {
			fmt.Fprintf(os.Stderr, "bay: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cmd := flag.Arg(0)
	if cmd == "" {
		usage()
		os.Exit(2)
	}
	if err := dispatch(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "bay: %v\n", err)
		os.Exit(1)
	}
}

func privileged() error {
	domains := strings.Split(*privDomain, ",")
	if *privRemove {
		return stack.UninstallResolver("", nil, domains...)
	}
	var retire []string
	if *privRetire != "" {
		retire = strings.Split(*privRetire, ",")
	}
	return stack.Install{Domains: domains, Retire: retire, Addr: *addr,
		CAPath: *privCA, UntrustCA: *privUntrust}.Apply()
}

func dispatch(cmd string) error {
	m := stack.NewMachine(*dir)
	switch cmd {
	case "up":
		return up(m)
	case "down":
		return down(m)
	case "start", "stop", "restart":
		return serviceAction(m, cmd, flag.Args()[1:])
	case "logs":
		return logs(m, flag.Args()[1:])
	case "status":
		return status(m, flag.Args()[1:])
	case "doctor":
		return doctor(m)
	case "domain":
		return domain(m, flag.Args()[1:])
	case "brief":
		return brief(flag.Args()[1:])
	case "schema":
		return schema(flag.Args()[1:])
	case "help":
		return help(flag.Args()[1:])
	case "peer":
		return peer(m, flag.Args()[1:])
	case "sync":
		return sync(m)
	case "install":
		return install(m)
	case "uninstall":
		return uninstall(m)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func newRuntime(m *stack.Machine) (*stack.Runtime, error) {
	bin, err := resolveDNSBinary()
	if err != nil {
		return nil, err
	}
	return &stack.Runtime{
		Machine:  m,
		Addr:     *addr,
		DNSBin:   bin,
		Progress: func(line string) { fmt.Println(line) },
	}, nil
}

func up(m *stack.Machine) error {
	cfg, err := stack.LoadIn(m, *file)
	if err != nil {
		return err
	}
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	if _, err := rt.Up(cfg); err != nil {
		return err
	}
	fmt.Println()
	for _, s := range cfg.Sorted() {
		if s.Internal {
			continue
		}
		fmt.Printf("  https://%s/\n", s.Domain)
	}
	return nil
}

func down(m *stack.Machine) error {
	cfg, err := stack.LoadIn(m, *file)
	if err != nil {
		return err
	}
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	_, err = rt.Down(cfg)
	return err
}

// serviceAction starts, stops or restarts named services of this group, then
// republishes the routes so the proxy matches what is actually running.
func serviceAction(m *stack.Machine, action string, names []string) error {
	cfg, err := stack.LoadIn(m, *file)
	if err != nil {
		return err
	}
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	switch action {
	case "start":
		_, err = rt.StartServices(cfg, names)
	case "stop":
		_, err = rt.StopServices(cfg, names)
	case "restart":
		_, err = rt.RestartServices(cfg, names)
	}
	return err
}

// logs streams one service's output.
func logs(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "follow the log output")
	tail := fs.Int("n", 0, "show only the last n lines")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := stack.LoadIn(m, *file)
	if err != nil {
		return err
	}
	targets, err := stack.SelectServices(cfg, fs.Args())
	if err != nil {
		return err
	}
	if len(targets) != 1 {
		return fmt.Errorf("logs needs exactly one service; this group has %d", len(targets))
	}
	return stack.Logs(targets[0].ContainerName, *follow, *tail, os.Stdout)
}

// domain lists or changes the machine's domains. A domain is delegated once for
// the machine, so it is managed here rather than in a project.
func domain(m *stack.Machine, args []string) error {
	if len(args) == 0 {
		return listDomains(m)
	}
	action := args[0]
	if len(args) < 2 {
		return fmt.Errorf("%s needs a domain name", action)
	}
	name := args[1]

	// The settings file records the change and the resolver entries apply it.
	// When applying fails, the settings are restored, so a refused
	// authorization does not leave a domain recorded but not delegated.
	before, err := m.Settings()
	if err != nil {
		return err
	}
	// Withdrawing is asked for by name: the entry is removed because this
	// command was told to, and no other command removes one.
	var retire []string
	switch action {
	case "add":
		err = m.AddDomain(name)
	case "remove":
		err = m.RemoveDomain(name)
		retire = []string{name}
	case "default":
		err = m.SetDefaultDomain(name)
	default:
		return fmt.Errorf("unknown domain action %q; use add, remove or default", action)
	}
	if err != nil {
		return err
	}

	if err := applyDomains(m, retire...); err != nil {
		if restore := m.SaveSettings(before); restore != nil {
			return fmt.Errorf("%w (and the settings could not be restored: %v)", err, restore)
		}
		return err
	}
	return listDomains(m)
}

// applyDomains writes the resolver entries for the recorded domains, removes
// the entries of the domains named for retirement, and republishes the proxy.
func applyDomains(m *stack.Machine, retire ...string) error {
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	rt.Retire = retire
	if err := rt.EnsureInstalled(); err != nil {
		return err
	}
	_, err = stack.SyncProxy(m)
	return err
}

func listDomains(m *stack.Machine) error {
	snap, err := stack.Take(m, *addr)
	if err != nil {
		return err
	}
	if len(snap.Machine.DomainList) == 0 {
		fmt.Println("no domains are delegated on this machine")
		return nil
	}
	for _, d := range snap.Machine.DomainList {
		note := "delegated"
		if d.Default {
			note = "default, used by projects that name none"
		} else if len(d.PinnedBy) > 0 {
			note = "pinned by " + strings.Join(d.PinnedBy, ", ")
		}
		fmt.Printf("  *.%-20s %s\n", d.Name, note)
	}
	return nil
}

// brief prints the usage contract. A caller that installed only the binaries
// reads it instead of this repository's documents.
func brief(args []string) error {
	switch {
	case len(args) > 0 && args[0] == "--json":
		return contract.BriefJSON(os.Stdout)
	case len(args) > 0 && args[0] == "--markdown":
		contract.MarkdownCLI(os.Stdout)
	default:
		contract.Brief(os.Stdout)
	}
	return nil
}

func schema(args []string) error {
	switch {
	case len(args) > 0 && args[0] == "--json":
		return contract.SchemaJSON(os.Stdout)
	case len(args) > 0 && args[0] == "--markdown":
		contract.MarkdownSchema(os.Stdout)
	default:
		contract.Schema(os.Stdout)
	}
	return nil
}

func help(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	if !contract.Help(os.Stdout, args[0]) {
		return fmt.Errorf("no command named %q", args[0])
	}
	return nil
}

// install applies the machine setup now, so the first up does not have to ask.
// peer lists the machines whose domains this one reaches, or changes them.
func peer(m *stack.Machine, args []string) error {
	if len(args) == 0 {
		return listPeers(m)
	}
	switch args[0] {
	case "open", "close":
		on := args[0] == "open"
		if err := m.SetPeering(on); err != nil {
			return err
		}
		if !on {
			return syncAfterPeerChange(m, "the link is closed")
		}
		return syncAfterPeerChange(m,
			fmt.Sprintf("the link is open at %s", stack.PeerLinkAddress(stack.LANAddress())))
	case "find":
		return findPeers()
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("peer add needs the address of the other machine, " +
				"for example 192.168.0.42:8443, or the name of one that peer find lists")
		}
		return addPeer(m, args[1])
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("peer remove needs a fingerprint; run \"bay peer\"")
		}
		return removePeer(m, args[1])
	default:
		return fmt.Errorf("unknown peer action %q; use open, close, find, add or remove", args[0])
	}
}

func listPeers(m *stack.Machine) error {
	peers, err := stack.Peers(m.Dir)
	if err != nil {
		return err
	}
	settings, err := m.Settings()
	if err != nil {
		return err
	}
	link := "closed"
	if settings.Peering {
		link = "open at " + stack.PeerLinkAddress(stack.LANAddress())
	}
	fmt.Printf("link   %s\n", link)
	if len(peers) == 0 {
		fmt.Println("peers  none")
		return nil
	}
	for _, p := range peers {
		fmt.Printf("%-16s %-22s %s\n", p.Name, p.Address, short(p.Fingerprint))
		for _, d := range p.Domains {
			fmt.Printf("    %s\n", d)
		}
	}
	return nil
}

// findPeers lists the machines announcing themselves on this network. A machine
// whose announcement does not reach here is reached by its address instead.
func findPeers() error {
	fmt.Printf("listening for %s\n", discoverFor)
	found, err := stack.Discover(context.Background(), 0, discoverFor)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		fmt.Println("nothing announced. Give the address instead: bay peer add <host:port>")
		return nil
	}
	for _, b := range found {
		fmt.Printf("%-16s %-22s %s\n", b.Name, b.Address, short(b.ID))
	}
	return nil
}

// discoverFor is long enough to hear a machine announcing itself twice.
const discoverFor = 4 * time.Second

// resolveAddress turns what was given into an address. A name is looked for
// among the machines announcing themselves; anything holding a colon is already
// an address.
func resolveAddress(given string) (string, error) {
	if strings.Contains(given, ":") {
		return given, nil
	}
	found, err := stack.Discover(context.Background(), 0, discoverFor)
	if err != nil {
		return "", err
	}
	var matches []stack.Beacon
	for _, b := range found {
		if b.Name == given {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].Address, nil
	case 0:
		return "", fmt.Errorf("no machine announcing itself as %q; give its address instead", given)
	default:
		return "", fmt.Errorf("%d machines announce themselves as %q; give the address of the one you mean",
			len(matches), given)
	}
}

// addPeer reads what the machine at the address says about itself and asks
// before approving it. The fingerprint is printed so it can be compared with
// what the other machine reports.
func addPeer(m *stack.Machine, given string) error {
	address, err := resolveAddress(given)
	if err != nil {
		return err
	}
	doc, err := stack.FetchPeer(address)
	if err != nil {
		return err
	}
	fingerprint, err := stack.Fingerprint(doc.CA)
	if err != nil {
		return err
	}
	peers, err := stack.Peers(m.Dir)
	if err != nil {
		return err
	}
	// An address that used to carry another authority is another machine. Say
	// so before the fingerprint, so it is read before the question.
	if known, err := stack.PeerAtAddress(peers, address, doc.CA); err != nil {
		return err
	} else if known != nil {
		fmt.Printf("%s already carried the authority %s, approved as %q.\n",
			doc.Address, short(known.Fingerprint), known.Name)
		fmt.Printf("It now answers with another one. This is a different machine.\n\n")
	}

	fmt.Printf("machine     %s\n", doc.Name)
	fmt.Printf("address     %s\n", address)
	fmt.Printf("fingerprint %s\n", short(fingerprint))
	if len(doc.Domains) == 0 {
		fmt.Println("domains     none yet")
	}
	for i, d := range doc.Domains {
		if i == 0 {
			fmt.Printf("domains     %s\n", d)
			continue
		}
		fmt.Printf("            %s\n", d)
	}
	fmt.Print("\nApprove this machine? Its domains will be reachable here. [y/N] ")
	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("not approved")
		return nil
	}
	// The address this machine was reached at is stored, not the one the other
	// machine states about itself. A machine behind a router states an address
	// on its own network, which is not the address that reaches it from here.
	if err := stack.ApprovePeer(m.Dir, stack.Peer{
		Name: doc.Name, Address: address, Domains: doc.Domains, CA: doc.CA,
	}); err != nil {
		return err
	}
	return syncAfterPeerChange(m, "approved "+doc.Name)
}

func removePeer(m *stack.Machine, fingerprint string) error {
	peers, err := stack.Peers(m.Dir)
	if err != nil {
		return err
	}
	var match stack.Peer
	var found int
	for _, p := range peers {
		if strings.HasPrefix(p.Fingerprint, fingerprint) {
			match, found = p, found+1
		}
	}
	switch found {
	case 0:
		return fmt.Errorf("no peer whose fingerprint starts with %q", fingerprint)
	case 1:
	default:
		return fmt.Errorf("%d peers start with %q; give more of it", found, fingerprint)
	}
	if err := stack.RemovePeer(m.Dir, match.Fingerprint); err != nil {
		return err
	}
	return syncAfterPeerChange(m, "removed "+match.Name)
}

// syncAfterPeerChange rewrites the proxy configuration for the new peer list.
func syncAfterPeerChange(m *stack.Machine, what string) error {
	res, err := stack.SyncProxy(m)
	if err != nil {
		return err
	}
	fmt.Printf("%s; proxy %s with %d routes and %d peers\n",
		what, res.Action, len(res.Routes), len(res.Peers))
	return nil
}

// short returns the first part of a fingerprint, which is what a person
// compares between two machines.
func short(fingerprint string) string {
	if len(fingerprint) > 16 {
		return fingerprint[:16]
	}
	return fingerprint
}

// sync rewrites the proxy configuration from the containers that are running.
// It is how a change to how that configuration is written reaches a machine
// whose containers are already up: every other command that rewrites it also
// starts or stops something.
func sync(m *stack.Machine) error {
	res, err := stack.SyncProxy(m)
	if err != nil {
		return err
	}
	for _, c := range res.Conflicts {
		fmt.Fprintf(os.Stderr, "%s is served by %s; %s also claims it\n",
			c.Domain, c.Kept, c.Dropped)
	}
	fmt.Printf("proxy %s with %d routes\n", res.Action, len(res.Routes))
	for _, r := range res.Routes {
		fmt.Printf("  %-28s %s\n", r.Domain, r.Address)
	}
	return nil
}

func install(m *stack.Machine) error {
	// Register the project in the working directory so install covers it.
	if cfg, err := stack.LoadIn(m, *file); err == nil {
		if err := m.Register(cfg.Ref()); err != nil {
			return err
		}
	}
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	// Taking the machine setup over is what this command is for, so it is the
	// one that may change a setup another state directory owns.
	rt.TakeOwnership = true
	if err := rt.EnsureInstalled(); err != nil {
		return err
	}
	return doctor(m)
}

// uninstall removes the launchd job and every resolver entry this tool wrote.
// The certificate authority is left in the trust settings.
func uninstall(m *stack.Machine) error {
	// Removing the setup changes it, so it belongs to the owner.
	if err := stack.OwnsMachineSetup(m.Dir); err != nil {
		return err
	}
	domains, err := m.Domains()
	if err != nil {
		return err
	}
	if err := stack.UninstallDNSAgent(); err != nil {
		return err
	}
	fmt.Printf("removed dns agent %s\n", stack.DNSAgentLabel)
	if err := stack.UninstallResolver("", nil, domains...); err != nil {
		return err
	}
	for _, d := range domains {
		fmt.Printf("removed %s\n", stack.ResolverPath(d))
	}
	if ca, err := stack.LoadOrCreateCA(m.Dir); err == nil {
		fmt.Fprintf(os.Stderr, "\nthe CA is still trusted; remove it with:\n"+
			"  sudo security remove-trusted-cert -d %s\n", ca.CertPath())
	}
	return nil
}

// status prints the machine snapshot, or emits it as JSON for other programs.
func status(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit the snapshot as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	snap, err := stack.Take(m, *addr)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(snap)
	}
	printSnapshot(snap)
	return nil
}

func printSnapshot(snap stack.Snapshot) {
	for _, p := range snap.Machine.Proxies {
		where := p.State
		if p.IPv4 != "" {
			where += " at " + p.IPv4
		}
		fmt.Printf("proxy  %-22s %s, %s, %d route(s)\n", p.Name, p.Engine, where, p.Routes)
	}

	d := snap.Machine.DNS
	dnsState := "not loaded"
	switch {
	case d.Loaded && d.Current:
		dnsState = "loaded"
	case d.Loaded:
		dnsState = "loaded but out of date"
	}
	fmt.Printf("dns    %-22s %s on %s\n", d.Label, dnsState, d.Addr)
	if authority := snap.Certificates.Authority; authority.Unreadable() != "" {
		fmt.Printf("authority %s\n", authority.Status())
	}
	if len(snap.Machine.Pending) > 0 {
		fmt.Printf("setup  %d step(s) pending; run \"bay install\"\n", len(snap.Machine.Pending))
	}

	if len(snap.Groups) == 0 {
		fmt.Println("\nno groups registered")
		return
	}
	for _, g := range snap.Groups {
		fmt.Printf("\ngroup  %s  (%s)\n", g.Name, g.StackPath)
		if g.Error != "" {
			fmt.Printf("  ! %s\n", g.Error)
		}
		for _, s := range g.Services {
			route, where := "-", s.Domain
			if s.Routed {
				route = "routed"
			}
			if s.Internal {
				route, where = "internal", "reachable at "+s.Container+"."+stack.BackendDomain
			}
			if s.State == "starting" {
				where += " · not accepting connections yet"
			}
			fmt.Printf("  %-8s %-8s %-16s %-16s %s\n", s.State, route, s.Name, s.IPv4, where)
		}
	}
}

// doctor reports what the machine setup would change. It changes nothing and
// requests no privileges.
func doctor(m *stack.Machine) error {
	authority := stack.AuthorityInfo(m.Dir)
	domains, err := m.Domains()
	if err != nil {
		return err
	}
	// Include the project in the working directory even before it is
	// registered, so the report covers the next up.
	if cfg, err := stack.LoadIn(m, *file); err == nil {
		domains = union(domains, cfg.Domains())
		fmt.Printf("group     %s  (%s)\n", cfg.Name, cfg.Path())
	}
	in := stack.Install{Domains: domains, Addr: *addr, CAPath: authority.Path}

	shown := "none"
	if len(domains) > 0 {
		shown = "*." + strings.Join(domains, ", *.")
	}
	fmt.Printf("state     %s\n", m.Dir)
	fmt.Printf("domains   %s\n", shown)
	fmt.Printf("resolver  %s\n", *addr)

	// The machine setup is one per machine. Saying who owns it here is what
	// tells a reader running from another state directory why a command stops.
	owner, owned := stack.MachineOwner()
	switch {
	case !owned:
		fmt.Printf("owner     none yet; the next \"bay install\" takes it\n")
	case stack.OwnsMachineSetup(m.Dir) == nil:
		fmt.Printf("owner     this state directory\n")
	default:
		fmt.Printf("owner     %s  (this command cannot change the machine setup)\n", owner)
	}

	agent := "not loaded"
	switch {
	case !stack.DNSAgentLoaded():
	case stack.DNSAgentServes(domains, *addr, dnsBinary(), m.Dir):
		agent = "loaded and current"
	default:
		agent = "loaded but out of date"
	}
	fmt.Printf("dns agent %s\n", agent)
	if authority.Unreadable() != "" {
		fmt.Printf("authority %s\n", authority.Status())
	}

	pending := in.Pending()
	if _, err := os.Stat(authority.Path); os.IsNotExist(err) {
		pending = append([]stack.SetupStep{{
			Text: "create local certificate authority", Asks: stack.AsksNobody,
		}}, pending...)
	}
	if len(pending) == 0 && agent == "loaded and current" && authority.Unreadable() == "" {
		fmt.Println("\nnothing to do")
		reportUnclaimed(domains)
		return nil
	}
	fmt.Println("\n\"bay up\" would:")
	// Each step says what it puts in front of a person, because the two that do
	// ask for different things and neither can be answered by a script.
	for _, p := range pending {
		if p.Asks == stack.AsksNobody {
			fmt.Println("  - " + p.Text)
			continue
		}
		fmt.Printf("  - %s   (asks for %s)\n", p.Text, p.Asks)
	}
	if agent != "loaded and current" {
		fmt.Println("  - re-register " + stack.DNSAgentLabel)
	}
	reportUnclaimed(domains)
	return nil
}

// reportUnclaimed names the resolver entries that point at this tool and that
// no domain of this machine covers. They are reported rather than removed: one
// may belong to another state directory, and they are removed by
// "bay domain remove" and "bay uninstall" only.
func reportUnclaimed(domains []string) {
	left := stack.UnclaimedResolverEntries(domains, *addr)
	if len(left) == 0 {
		return
	}
	fmt.Println("\nresolver entries no domain of this machine covers:")
	for _, d := range left {
		fmt.Printf("  %s\n", stack.ResolverPath(d))
	}
	fmt.Println("They are left in place. Remove one with " +
		"\"bay domain remove <name>\" if it is yours to withdraw.")
}

// dnsBinary returns the bayd this installation would register, or an
// empty string when it cannot be found, which leaves the settings to decide.
func dnsBinary() string {
	bin, err := resolveDNSBinary()
	if err != nil {
		return ""
	}
	return bin
}

// sameDomains reports whether two domain lists contain the same names.
func sameDomains(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// union merges domain lists, removes repeats and sorts the result.
func union(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, d := range list {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	sort.Strings(out)
	return out
}

// resolveDNSBinary returns the path of bayd next to this executable, or
// on PATH. The launchd job stores an absolute path.
func resolveDNSBinary() (string, error) {
	if *dnsBin != "" {
		return filepath.Abs(*dnsBin)
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "bayd")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand, nil
		}
	}
	path, err := exec.LookPath("bayd")
	if err != nil {
		return "", fmt.Errorf("bayd not found next to bay or on PATH; pass -dnsbin")
	}
	return filepath.Abs(path)
}

func defaultState() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".bay"
	}
	return filepath.Join(home, ".bay")
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: bay [-f compose.yaml] [-state dir] [-addr host:port] <command>

commands:
`)
	contract.CommandList(os.Stderr)
	fmt.Fprint(os.Stderr, `
Run "bay help <command>" for what a command changes, "bay
brief" for the whole usage contract, or "bay schema" for the Compose
file contract.

`)
	flag.PrintDefaults()
}
