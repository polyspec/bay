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
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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
	return stack.Install{Domains: domains, Addr: *addr, CAPath: *privCA, UntrustCA: *privUntrust}.Apply()
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
	switch action {
	case "add":
		err = m.AddDomain(name)
	case "remove":
		err = m.RemoveDomain(name)
	case "default":
		err = m.SetDefaultDomain(name)
	default:
		return fmt.Errorf("unknown domain action %q; use add, remove or default", action)
	}
	if err != nil {
		return err
	}

	if err := applyDomains(m); err != nil {
		if restore := m.SaveSettings(before); restore != nil {
			return fmt.Errorf("%w (and the settings could not be restored: %v)", err, restore)
		}
		return err
	}
	return listDomains(m)
}

// applyDomains writes the resolver entries for the recorded domains and
// republishes the proxy.
func applyDomains(m *stack.Machine) error {
	rt, err := newRuntime(m)
	if err != nil {
		return err
	}
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
	if err := rt.EnsureInstalled(); err != nil {
		return err
	}
	return doctor(m)
}

// uninstall removes the launchd job and every resolver entry this tool wrote.
// The certificate authority is left in the trust settings.
func uninstall(m *stack.Machine) error {
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
	// Register the project in the working directory so status includes it
	// before its first up.
	// Register again when the recorded path or domains differ, so the entry
	// matches the file on disk.
	if cfg, err := stack.LoadIn(m, *file); err == nil {
		if known, ok := groupKnown(m, cfg.Name); !ok || known.StackPath != cfg.Path() ||
			!sameDomains(known.Domains, cfg.Ref().Domains) {
			if err := m.Register(cfg.Ref()); err != nil {
				return err
			}
		}
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

func groupKnown(m *stack.Machine, name string) (stack.GroupRef, bool) {
	groups, err := m.Groups()
	if err != nil {
		return stack.GroupRef{}, false
	}
	for _, g := range groups {
		if g.Name == name {
			return g, true
		}
	}
	return stack.GroupRef{}, false
}

func printSnapshot(snap stack.Snapshot) {
	p := snap.Machine.Proxy
	where := p.State
	if p.IPv4 != "" {
		where += " at " + p.IPv4
	}
	fmt.Printf("proxy  %-22s %s, %d route(s)\n", p.Name, where, p.Routes)

	d := snap.Machine.DNS
	dnsState := "not loaded"
	switch {
	case d.Loaded && d.Current:
		dnsState = "loaded"
	case d.Loaded:
		dnsState = "loaded but out of date"
	}
	fmt.Printf("dns    %-22s %s on %s\n", d.Label, dnsState, d.Addr)
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
			fmt.Printf("  %-8s %-8s %-16s %-16s %s\n", s.State, route, s.Name, s.IPv4, where)
		}
	}
}

// doctor reports what the machine setup would change. It changes nothing and
// requests no privileges.
func doctor(m *stack.Machine) error {
	ca, err := stack.LoadOrCreateCA(m.Dir)
	if err != nil {
		return err
	}
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
	in := stack.Install{Domains: domains, Addr: *addr, CAPath: ca.CertPath()}

	shown := "none"
	if len(domains) > 0 {
		shown = "*." + strings.Join(domains, ", *.")
	}
	fmt.Printf("state     %s\n", m.Dir)
	fmt.Printf("domains   %s\n", shown)
	fmt.Printf("resolver  %s\n", *addr)

	agent := "not loaded"
	switch {
	case !stack.DNSAgentLoaded():
	case stack.DNSAgentServes(domains, *addr, stack.ProxyName):
		agent = "loaded and current"
	default:
		agent = "loaded but out of date"
	}
	fmt.Printf("dns agent %s\n", agent)

	pending := in.Pending()
	if len(pending) == 0 && agent == "loaded and current" {
		fmt.Println("\nnothing to do")
		return nil
	}
	fmt.Println("\n\"bay up\" would:")
	for _, p := range pending {
		fmt.Println("  - " + p + "   (needs your password)")
	}
	if agent != "loaded and current" {
		fmt.Println("  - re-register " + stack.DNSAgentLabel)
	}
	return nil
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
