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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/polyspec/bay/internal/contract"
	"github.com/polyspec/bay/internal/stack"
)

var (
	file    = flag.String("f", ".", "Compose file, or a directory to find one in")
	project = flag.String("p", "", "project name, instead of the Compose file's name or directory; two runs of one file under two names do not overlap")
	dir     = flag.String("state", defaultState(), "directory holding the CA, certificates and machine state")
	addr    = flag.String("addr", stack.DefaultDNSAddr, "address bayd listens on")
	dnsBin  = flag.String("dnsbin", "", "path to the bayd binary (default: next to this one)")

	// Set when this binary re-runs itself as root; not part of the user interface.
	privApply   = flag.Bool("privileged-apply", false, "internal: apply the privileged setup steps")
	privRemove  = flag.Bool("privileged-remove", false, "internal: remove resolver entries")
	privDomain  = flag.String("domain", "", "internal: comma-separated domains for the privileged steps")
	privRetire  = flag.String("retire", "", "internal: comma-separated domains whose resolver entries are removed")
	privCA      = flag.String("ca", "", "internal: CA certificate for the privileged steps")
	privUntrust = flag.String("untrust", "", "internal: retired CA certificate to stop trusting")
	privRelay   = flag.String("relaybin", "", "internal: bayd binary the relay daemon runs")
	privApple   = flag.String("appledomain", "", "internal: the Apple container DNS domain to create")
	privNoRelay = flag.Bool("privileged-remove-relay", false, "internal: remove the relay daemon")
)

func main() {
	flag.Usage = usage
	flag.Parse()

	if *privApply || *privRemove || *privNoRelay {
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
		var status exitStatus
		if errors.As(err, &status) {
			os.Exit(int(status))
		}
		fmt.Fprintf(os.Stderr, "bay: %v\n", err)
		var usage usageError
		if errors.As(err, &usage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func privileged() error {
	if *privNoRelay {
		return stack.UninstallRelayDaemon("", nil)
	}
	// A machine that is not macOS has one step that needs root, its firewall,
	// and it is read from the state directory.
	if !stack.HostSetsUpNames() {
		bin := ""
		if *dnsBin != "" {
			bin = *dnsBin
		}
		l, err := stack.NewLinuxSetup(stack.NewMachine(*dir), bin, *addr)
		if err != nil {
			return err
		}
		if *privRemove {
			if err := stack.RemoveAgent(); err != nil {
				return err
			}
			return l.ApplyFirewall()
		}
		return l.ApplyRoot()
	}
	domains := strings.Split(*privDomain, ",")
	if *privRemove {
		return stack.UninstallResolver("", nil, domains...)
	}
	var retire []string
	if *privRetire != "" {
		retire = strings.Split(*privRetire, ",")
	}
	return stack.Install{Domains: domains, Retire: retire, Addr: *addr,
		CAPath: *privCA, UntrustCA: *privUntrust,
		RelayBin: *privRelay, State: *dir, AppleDomain: *privApple}.Apply()
}

func dispatch(cmd string) error {
	m := stack.NewMachine(*dir)
	switch cmd {
	case "up":
		return up(m, flag.Args()[1:])
	case "pull":
		return pull(m, flag.Args()[1:])
	case "cert":
		return cert(m, flag.Args()[1:])
	case "version":
		return version(flag.Args()[1:])
	case "down":
		return down(m)
	case "start", "stop", "restart":
		return serviceAction(m, cmd, flag.Args()[1:])
	case "logs":
		return logs(m, flag.Args()[1:])
	case "exec":
		return execIn(m, flag.Args()[1:])
	case "status":
		return status(m, flag.Args()[1:])
	case "doctor":
		return doctor(m)
	case "check":
		return check(m, flag.Args()[1:])
	case "images":
		return images(m, flag.Args()[1:])
	case "plan":
		return plan(m, flag.Args()[1:])
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
		return install(m, flag.Args()[1:])
	case "uninstall":
		return uninstall(m)
	case "skill":
		return skill(flag.Args()[1:])
	case "mcp":
		return mcpCommand(flag.Args()[1:])
	default:
		return usageError{fmt.Sprintf("unknown command %q; \"bay help\" lists them", cmd)}
	}
}

// queryRuntime is the runtime of a command that changes nothing. It needs no
// bayd: one that is missing is reported as missing setup.
func queryRuntime(m *stack.Machine) *stack.Runtime {
	return &stack.Runtime{Machine: m, Addr: *addr, DNSBin: dnsBinary()}
}

func newRuntime(m *stack.Machine) (*stack.Runtime, error) {
	// The agent is part of every machine's setup: the DNS agent on macOS, the
	// systemd agent elsewhere.
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

func up(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	noPull := fs.Bool("no-pull", false, "fail instead of pulling an image the local store does not hold")
	asJSON := fs.Bool("json", false, "print the services as JSON on standard output; progress goes to standard error")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	rt.NoPull = *noPull
	if *asJSON {
		rt.Progress = func(line string) { fmt.Fprintln(os.Stderr, line) }
	}
	stop := cleanupOnSignal(rt, cfg)
	_, err = rt.Up(cfg)
	stop()
	if err != nil {
		return err
	}
	if *asJSON {
		report, err := stack.ReportProject(m, *addr, cfg)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Println()
	for _, s := range cfg.Sorted() {
		for _, d := range s.Domains {
			fmt.Printf("  https://%s/\n", d)
		}
	}
	return nil
}

// pull fetches the images of the selected services.
func pull(m *stack.Machine, names []string) error {
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	return rt.Pull(cfg, names)
}

func down(m *stack.Machine) error {
	// What to remove is read from the containers, so only the project's name is
	// needed, and it is found even when the file is gone.
	name, err := projectName(m)
	if err != nil {
		return err
	}
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	_, err = rt.Down(name)
	return err
}

// serviceAction starts, stops or restarts named services of this group, then
// republishes the routes so the proxy matches what is actually running.
func serviceAction(m *stack.Machine, action string, names []string) error {
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
	// Stopping is read from the containers and needs only the project's name;
	// starting needs the file, because the file says what to start.
	if action == "stop" {
		name, err := projectName(m)
		if err != nil {
			return err
		}
		_, err = rt.StopServices(name, names)
		return err
	}
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	switch action {
	case "start":
		_, err = rt.StartServices(cfg, names)
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
	cfg, err := loadProject(m)
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

// exitStatus is the exit status of a command bay ran for the caller, which bay
// exits with unchanged.
type exitStatus int

func (s exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(s)) }

// listFlag collects a flag given more than once.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// execIn runs a command in a service's container: bay exec <service> <command...>.
func execIn(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	user := fs.String("u", "", "run as this user (name or uid[:gid])")
	workdir := fs.String("w", "", "run in this directory inside the container")
	var env listFlag
	fs.Var(&env, "e", "set an environment variable, KEY=VALUE; repeatable")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	if fs.NArg() < 2 {
		return usageError{"exec needs a service and a command: bay exec <service> <command...>"}
	}
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	targets, err := stack.SelectServices(cfg, fs.Args()[:1])
	if err != nil {
		return err
	}
	code, err := stack.Exec(targets[0].ContainerName, fs.Args()[1:], *user, *workdir, env)
	if err != nil {
		return err
	}
	if code != 0 {
		return exitStatus(code)
	}
	return nil
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

// install applies the machine setup that is missing and changes nothing that
// is in place, so running it again does nothing. -dry-run prints what it would
// do and changes nothing. -only-from, on a machine that is not macOS, records
// the hosts its firewall accepts connections from before applying.
func install(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print what would change and change nothing")
	onlyFrom := fs.String("only-from", "", "comma-separated hosts or networks this machine accepts new connections from, or none (not macOS)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var hosts []string
	if *onlyFrom != "" {
		if stack.HostSetsUpNames() {
			return fmt.Errorf("-only-from is for a machine that is not macOS")
		}
		var err error
		if hosts, err = stack.ParseOnlyFrom(*onlyFrom); err != nil {
			return err
		}
	}
	if *dryRun {
		items, err := queryRuntime(m).SetupItems()
		if err != nil {
			return err
		}
		// The firewall -only-from names is the one reported, as if recorded.
		if *onlyFrom != "" {
			l, err := stack.NewLinuxSetup(m, dnsBinary(), *addr)
			if err != nil {
				return err
			}
			l.OnlyFrom = hosts
			items = l.Items()
			fmt.Printf("would record: accept new connections only from %s\n", describeHosts(hosts))
		}
		printPlan(items, "\"bay install\" would:")
		return nil
	}
	if *onlyFrom != "" {
		if err := m.SetOnlyFrom(hosts); err != nil {
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
	// The proxy is written with the relay once the relay is registered.
	if stack.HostSetsUpNames() {
		if peers, _ := stack.Peers(m.Dir); len(peers) > 0 {
			if _, err := stack.SyncProxy(m); err != nil {
				return err
			}
		}
	}
	return check(m, nil)
}

func describeHosts(hosts []string) string {
	if len(hosts) == 0 {
		return "anyone (no firewall)"
	}
	return strings.Join(hosts, ", ")
}

// check prints every part of the machine setup and whether it is in place, and
// fails when any is not. It changes nothing.
func check(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	scope := fs.String("scope", "all", "all: every part; run: only the parts running a project needs, without the agent skill and MCP registration")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	if *scope != "all" && *scope != "run" {
		return usageError{fmt.Sprintf("unknown scope %q; use all or run", *scope)}
	}
	all, err := queryRuntime(m).SetupItems()
	if err != nil {
		return err
	}
	var items []stack.SetupItem
	for _, it := range all {
		if *scope == "run" && it.Agent {
			continue
		}
		items = append(items, it)
	}
	missing := 0
	for _, it := range items {
		if it.OK {
			fmt.Printf("ok       %s\n", it.Name)
			continue
		}
		missing++
		if it.Text == "" {
			fmt.Printf("missing  %s\n", it.Name)
			continue
		}
		fmt.Printf("missing  %s  (install: %s)\n", it.Name, it.Text)
	}
	if missing > 0 {
		return fmt.Errorf("%d part(s) of the setup are not in place; \"bay install -dry-run\" shows what install changes", missing)
	}
	return nil
}

// printPlan prints what install changes, or that it changes nothing. A part
// install cannot put in place is listed under its own heading.
func printPlan(items []stack.SetupItem, heading string) {
	var does, cannot []stack.SetupItem
	for _, it := range items {
		switch {
		case it.OK:
		case it.Text == "":
			cannot = append(cannot, it)
		default:
			does = append(does, it)
		}
	}
	if len(does) == 0 {
		fmt.Println("nothing to do")
	} else {
		fmt.Println(heading)
		// Each step says what it puts in front of a person, because the two
		// that do ask for different things and neither can be answered by a
		// script.
		for _, p := range does {
			if p.Asks == stack.AsksNobody {
				fmt.Println("  - " + p.Text)
				continue
			}
			fmt.Printf("  - %s   (asks for %s)\n", p.Text, p.Asks)
		}
	}
	if len(cannot) > 0 {
		fmt.Println("install cannot fix:")
		for _, it := range cannot {
			fmt.Println("  - " + it.Name)
		}
	}
}

// uninstall removes the machine setup this tool wrote. On macOS that is the
// launchd jobs and every resolver entry; the certificate authority is left in
// the trust settings. On a machine that is not macOS it is the firewall.
func uninstall(m *stack.Machine) error {
	// Removing the setup changes it, so it belongs to the owner.
	if err := stack.OwnsMachineSetup(m.Dir); err != nil {
		return err
	}
	if !stack.HostSetsUpNames() {
		if err := m.SetOnlyFrom(nil); err != nil {
			return err
		}
		if os.Geteuid() != 0 {
			return stack.Install{}.ElevateArgs("-privileged-remove", "-state", m.Dir)
		}
		if err := stack.RemoveAgent(); err != nil {
			return err
		}
		l, err := stack.NewLinuxSetup(m, "", *addr)
		if err != nil {
			return err
		}
		return l.ApplyFirewall()
	}
	domains, err := m.Domains()
	if err != nil {
		return err
	}
	if err := stack.UninstallDNSAgent(); err != nil {
		return err
	}
	fmt.Printf("removed dns agent %s\n", stack.DNSAgentLabel)
	if err := stack.UninstallRelayDaemon("", nil); err != nil {
		return err
	}
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

	// A machine that is not macOS has no DNS agent and no setup to report. Its
	// authority is created by the first command that publishes the proxy.
	if !stack.HostSetsUpNames() {
		fmt.Println("setup  none on this system; this machine serves its names to approved peers only")
		printGroups(snap)
		return
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
	printGroups(snap)
}

// printGroups writes each registered project and its services.
func printGroups(snap stack.Snapshot) {
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
			route, where := "-", strings.Join(s.Domains, " ")
			if s.Routed {
				route = "routed"
			}
			if s.Internal {
				route, where = "internal", "reachable at "+s.Container+"."+stack.BackendDomain
			}
			// The state is the engine's. A closed declared port is a measurement
			// beside it, named with its number.
			if s.Closed() {
				where += fmt.Sprintf(" · port %d does not accept connections", s.Port)
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
	var extra []string
	if cfg, err := loadProject(m); err == nil {
		extra = cfg.Domains()
		domains = union(domains, extra)
		fmt.Printf("group     %s  (%s)\n", cfg.Name, cfg.Path())
	}
	shown := "none"
	if len(domains) > 0 {
		shown = "*." + strings.Join(domains, ", *.")
	}
	fmt.Printf("state     %s\n", m.Dir)
	fmt.Printf("domains   %s\n", shown)
	if stack.HostSetsUpNames() {
		fmt.Printf("resolver  %s\n", *addr)
	} else {
		// A machine that is not macOS resolves no names itself: it serves them
		// to the peers that approve it.
		fmt.Println("names     served to approved peers only; this machine resolves none itself")
	}

	// The machine setup is one per machine. Saying who owns it here is what
	// tells a reader running from another state directory why a command stops.
	// Ownership is recorded by the DNS agent, which only macOS has.
	owner, owned := stack.MachineOwner()
	switch {
	case !stack.HostSetsUpNames():
	case !owned:
		fmt.Printf("owner     none yet; the next \"bay install\" takes it\n")
	case stack.OwnsMachineSetup(m.Dir) == nil:
		fmt.Printf("owner     this state directory\n")
	default:
		fmt.Printf("owner     %s  (this command cannot change the machine setup)\n", owner)
	}
	if authority.Unreadable() != "" {
		fmt.Printf("authority %s\n", authority.Status())
	}

	items, err := queryRuntime(m).SetupItems(extra...)
	if err != nil {
		return err
	}
	fmt.Println()
	for _, it := range items {
		mark := "ok     "
		if !it.OK {
			mark = "missing"
		}
		fmt.Printf("%s  %s\n", mark, it.Name)
	}
	fmt.Println()
	printPlan(items, "\"bay install\" would:")
	if stack.HostSetsUpNames() {
		reportUnclaimed(domains)
	}
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
	fmt.Fprint(os.Stderr, `usage: bay [-f compose.yaml] [-p name] [-state dir] [-addr host:port] <command>

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

// loadProject reads the selected Compose file under the name -p gives, if any.
func loadProject(m *stack.Machine) (*stack.Config, error) {
	return stack.LoadNamed(m, *file, *project)
}

// projectName is the selected project's name: the one -p gives, or the one its
// Compose file or registration has.
func projectName(m *stack.Machine) (string, error) {
	if *project != "" {
		return *project, nil
	}
	return stack.ProjectNameAt(m, *file)
}

// cleanupOnSignal makes an interrupted up leave nothing it made: on SIGINT or
// SIGTERM the engine commands that run are ended, the containers this up
// created are removed, and the process exits with 128 plus the signal. The
// containers that were there before are left as they were. A second signal
// during the cleanup exits at once. The returned function stops watching.
func cleanupOnSignal(rt *stack.Runtime, cfg *stack.Config) func() {
	before, err := rt.ProjectContainers(cfg.Name)
	if err != nil {
		before = nil
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	var got atomic.Bool
	go func() {
		var sig os.Signal
		select {
		case sig = <-signals:
		case <-done:
			return
		}
		got.Store(true)
		stack.Interrupt()
		go func() {
			<-signals
			os.Exit(exitFor(sig))
		}()
		// A container being created when the signal came is created by the
		// engine whatever the client does, so the cleanup waits for it and
		// lists it. The interrupted up stays refused while the cleanup runs.
		if !stack.Settle(2 * time.Minute) {
			fmt.Fprintf(os.Stderr, "bay: %s: an engine command did not finish in 2m; cleaning up what exists\n", sig)
		}
		fmt.Fprintf(os.Stderr, "bay: %s: removing the containers this up created\n", sig)
		if err := rt.RemoveCreatedSince(cfg.Name, before); err != nil {
			fmt.Fprintf(os.Stderr, "bay: cleanup: %v\n", err)
		}
		os.Exit(exitFor(sig))
	}()
	return func() {
		signal.Stop(signals)
		select {
		case <-done:
		default:
			close(done)
		}
		// A signal that arrived is being cleaned up; the cleanup exits.
		if got.Load() {
			select {}
		}
	}
}

func exitFor(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 1
}

// cert writes a client certificate: bay cert client <name> [-out dir] [--json].
func cert(m *stack.Machine, args []string) error {
	if len(args) < 2 || args[0] != "client" {
		return usageError{"usage: bay cert client <name> [-out dir] [--json]"}
	}
	fs := flag.NewFlagSet("cert", flag.ContinueOnError)
	out := fs.String("out", ".", "directory to write cert.pem, key.pem and ca.pem to")
	asJSON := fs.Bool("json", false, "print the paths as JSON")
	if err := fs.Parse(args[2:]); err != nil {
		return usageError{err.Error()}
	}
	dir, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	c, err := stack.IssueClient(m.Dir, args[1], dir)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(c)
	}
	fmt.Printf("cert  %s\nkey   %s\nca    %s\n", c.Cert, c.Key, c.CA)
	return nil
}

// images says whether the local store holds each service's image, and fails
// when one is missing. It fetches nothing.
func images(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("images", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	list, err := queryRuntime(m).Images(cfg, fs.Args())
	if err != nil {
		return err
	}
	missing := 0
	for _, it := range list {
		if !it.Present {
			missing++
		}
	}
	if *asJSON {
		if err := json.NewEncoder(os.Stdout).Encode(list); err != nil {
			return err
		}
	} else {
		for _, it := range list {
			state := "present"
			if !it.Present {
				state = "missing"
			}
			fmt.Printf("%-8s %-20s %s\n", state, it.Service, it.Image)
		}
	}
	if missing > 0 {
		return fmt.Errorf("%d image(s) are not in the local store; \"bay pull\" fetches them", missing)
	}
	return nil
}

// plan prints each service as up will create it: its name, port, domains and
// certificate paths. It reads the Compose file only.
func plan(m *stack.Machine, args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the plan as JSON")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	cfg, err := loadProject(m)
	if err != nil {
		return err
	}
	p := stack.PlanProject(m, cfg)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(p)
	}
	for _, s := range p.Services {
		addr := s.Host
		if s.Port > 0 {
			addr += ":" + strconv.Itoa(s.Port)
		}
		fmt.Printf("%-20s %s\n", s.Name, addr)
		for _, d := range s.Domains {
			fmt.Printf("%-20s https://%s/\n", "", d)
		}
		if c := s.Certificate; c != nil {
			fmt.Printf("%-20s certificate %s (in the container %s)\n", "", c.Cert, c.ContainerCert)
		}
	}
	return nil
}
