package stack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Runtime performs the operations that change what is running. The command line
// and the application both call it, so both apply the same behavior.
type Runtime struct {
	Machine *Machine
	// Addr is where bayd listens.
	Addr string
	// DNSBin is the bayd executable the launchd job should run.
	DNSBin string
	// HelperBin is the bay executable re-run as root for the setup
	// steps. Empty means the running executable, which is right for bay
	// itself and wrong for anything else.
	HelperBin string
	// RetiredCA is a former authority to stop trusting on the next setup pass,
	// set while rotating the CA.
	RetiredCA string
	// Retire names domains whose resolver entries are removed by this pass. It
	// is set by the command asked to stop delegating them and by nothing else.
	Retire []string
	// TakeOwnership lets this runtime change a machine setup another state
	// directory owns. Only "bay install" sets it, because taking the
	// setup over is what that command is for.
	TakeOwnership bool
	// Elevate runs the helper with administrator rights. Nil uses sudo, which
	// only works from a terminal.
	Elevate Elevator
	// Progress, when set, is called with a line describing each step.
	Progress func(string)
	// NoPull makes a missing image an error instead of a pull, so a run that
	// must not fetch anything fails before it creates a container.
	NoPull bool
}

// ownsSetup stops an operation that would change a machine setup this state
// directory does not own. It is the first statement of every such operation,
// because a refusal that arrives later leaves containers removed, volumes
// created and the project registered, and reports failure for a machine the
// command has already changed.
//
// TakeOwnership exempts the one command whose purpose is to take the setup
// over.
func (r *Runtime) ownsSetup() error {
	if r.TakeOwnership {
		return nil
	}
	return OwnsMachineSetup(r.Machine.Dir)
}

// engine returns a service engine that reports each step it begins through
// this runtime's Progress, so a command stopped in a long step is readable from
// what it has already printed.
func (r *Runtime) engine() *serviceEngine {
	e := newServiceEngine(r.Machine.Dir)
	e.progress = r.Progress
	e.noPull = r.NoPull
	return e
}

func (r *Runtime) say(format string, args ...any) {
	if r.Progress != nil {
		r.Progress(fmt.Sprintf(format, args...))
	}
}

// Up registers the project, applies any missing machine setup, starts every
// service and republishes the routes.
func (r *Runtime) Up(cfg *Config) (res SyncResult, err error) {
	if err := r.ownsSetup(); err != nil {
		return SyncResult{}, err
	}
	// Registering is what makes the project this machine's, and the window
	// draws from the registry, so it comes before the work: the project is on
	// the screen while its images are pulled. A first up that fails before it
	// made a container withdraws the registration again, so a refused run
	// leaves nothing behind.
	registered := false
	if groups, err := r.Machine.Groups(); err == nil {
		for _, g := range groups {
			registered = registered || g.Name == cfg.Name
		}
	}
	if err := r.Machine.Register(cfg.Ref()); err != nil {
		return SyncResult{}, err
	}
	defer func() {
		if err == nil || registered {
			return
		}
		if made, lerr := r.ProjectContainers(cfg.Name); lerr == nil && len(made) == 0 {
			_ = r.Machine.Unregister(cfg.Name)
		}
	}()
	engine := r.engine()
	if err := engine.preflight(cfg.Name, cfg.Sorted()); err != nil {
		return SyncResult{}, err
	}
	if err := engine.prepareVolumes(cfg.Name, cfg.Sorted(), false); err != nil {
		return SyncResult{}, err
	}
	if err := r.checkDomainsFree(cfg); err != nil {
		return SyncResult{}, err
	}
	if err := r.requireSetup(cfg); err != nil {
		return SyncResult{}, err
	}
	// A service the file no longer declares keeps its container and its route
	// until this removes them.
	if err := engine.removeUndeclared(cfg); err != nil {
		return SyncResult{}, err
	}
	// A service that the proxy routes to may read the address of the proxy when
	// it starts, so the proxy runs before the services start.
	if routed(cfg) {
		if _, err := syncProxy(r.Machine, nil, ServiceEngine(), r.Progress); err != nil {
			return SyncResult{}, err
		}
	}
	if err := engine.start(cfg, cfg.Sorted(), false); err != nil {
		return SyncResult{}, err
	}
	res, err = r.sync(containerNames(cfg.Sorted()))
	if err != nil {
		return res, err
	}
	r.reportReady(containerNames(cfg.Sorted()))
	return res, nil
}

// Pull fetches the image of each selected service, or of every service with no
// name, from its registry, whether or not the local store holds it. It starts
// and changes no container.
func (r *Runtime) Pull(cfg *Config, names []string) error {
	targets, err := SelectServices(cfg, names)
	if err != nil {
		return err
	}
	e := r.engine()
	seen := map[string]bool{}
	for _, s := range targets {
		if seen[s.Image] {
			continue
		}
		seen[s.Image] = true
		e.say("pulling %s", s.Image)
		if _, err := e.command(context.Background(), "image", "pull", s.Image); err != nil {
			return fmt.Errorf("pulling %s: %w", s.Image, err)
		}
	}
	return nil
}

// ImageState says whether the local store holds a service's image.
type ImageState struct {
	Service string `json:"service"`
	Image   string `json:"image"`
	Present bool   `json:"present"`
}

// Images reports, for each selected service, or every service with no name,
// whether the local store holds its image. It fetches nothing.
func (r *Runtime) Images(cfg *Config, names []string) ([]ImageState, error) {
	targets, err := SelectServices(cfg, names)
	if err != nil {
		return nil, err
	}
	e := r.engine()
	var out []ImageState
	for _, s := range targets {
		_, err := e.command(context.Background(), "image", "inspect", s.Image)
		if err != nil && !imageMissing(err) {
			return nil, err
		}
		out = append(out, ImageState{Service: s.Name, Image: s.Image, Present: err == nil})
	}
	return out, nil
}

// ReadyTimeout bounds how long a command waits for the processes inside the
// containers to accept connections. A service that takes longer is reported as
// starting rather than failing the command.
const ReadyTimeout = 20 * time.Second

// reportReady waits for the named containers to accept connections and reports
// the ones that did not. The command does not fail: a service may take longer
// than the wait, and the containers are already running.
func (r *Runtime) reportReady(containers []string) {
	if len(containers) == 0 {
		return
	}
	pending, err := WaitReady(containers, ReadyTimeout)
	if err != nil {
		r.say("could not check readiness: %v", err)
		return
	}
	if len(pending) == 0 {
		r.say("all services accept connections")
		return
	}
	r.say("port not accepting connections after %s: %s", ReadyTimeout, strings.Join(pending, ", "))
}

// checkDomainsFree reports an error when another running project already serves
// a domain this project claims. Starting anyway would remove the running
// project's route.
func (r *Runtime) checkDomainsFree(cfg *Config) error {
	instances, err := Instances()
	if err != nil {
		return err
	}
	return domainsFree(cfg, instances)
}

// domainsFree reports an error naming both services when a running service of
// another project claims a domain of cfg.
func domainsFree(cfg *Config, instances []ServiceInstance) error {
	claimed := map[string]ServiceInstance{}
	for _, in := range instances {
		if !in.Running() || in.Group == cfg.Name {
			continue
		}
		for _, d := range in.Domains {
			claimed[d] = in
		}
	}
	for _, s := range cfg.Sorted() {
		for _, d := range s.Domains {
			if held, taken := claimed[d]; taken {
				return fmt.Errorf("%s is served by service %q of project %q and claimed by service %q of project %q; change the domain or stop %q",
					d, held.Service, held.Group, s.Name, cfg.Name, held.Group)
			}
		}
	}
	return nil
}

// Down removes the project's containers and withdraws its routes. Other
// projects are not changed. It takes the project's name rather than its
// configuration, because what to remove is read from the containers' labels and
// a project whose file is gone still has containers to remove.
func (r *Runtime) Down(group string) (SyncResult, error) {
	if err := r.ownsSetup(); err != nil {
		return SyncResult{}, err
	}
	served := proxyRuns()
	if err := r.engine().removeProject(group); err != nil {
		return SyncResult{}, err
	}
	if err := r.Machine.Unregister(group); err != nil {
		return SyncResult{}, err
	}
	res, err := r.sync(nil)
	if err != nil && !served {
		// The project's routes are withdrawn whether or not the proxy serves the
		// other projects again: no proxy served them before this command. What
		// down was asked to do is done, so the proxy is reported, not failed on.
		r.say("warning: the proxy was not running and did not start again: %v; run bay sync", err)
		return res, nil
	}
	return res, err
}

// proxyRuns reports whether a proxy runs on any engine.
func proxyRuns() bool {
	for _, engine := range Engines() {
		if in, found, err := lookupOn(engine, ProxyName); err == nil && found && in.State == "running" {
			return true
		}
	}
	return false
}

// StartServices starts the named services and creates any missing container. An
// empty list selects every service in the project.
func (r *Runtime) StartServices(cfg *Config, names []string) (SyncResult, error) {
	if err := r.ownsSetup(); err != nil {
		return SyncResult{}, err
	}
	targets, err := SelectServices(cfg, names)
	if err != nil {
		return SyncResult{}, err
	}
	if err := r.checkDomainsFree(cfg); err != nil {
		return SyncResult{}, err
	}
	if err := r.engine().start(cfg, targets, false); err != nil {
		return SyncResult{}, err
	}
	res, err := r.sync(containerNames(targets))
	if err != nil {
		return res, err
	}
	r.reportReady(containerNames(targets))
	return res, nil
}

// StopServices stops the named services, or every service of the project when
// no name is given, and leaves their containers in place. Like Down it reads the
// containers' labels, so it needs no Compose file.
func (r *Runtime) StopServices(group string, names []string) (SyncResult, error) {
	if err := r.ownsSetup(); err != nil {
		return SyncResult{}, err
	}
	if err := r.engine().stopProject(group, names); err != nil {
		return SyncResult{}, err
	}
	return r.sync(nil)
}

func (r *Runtime) RestartServices(cfg *Config, names []string) (SyncResult, error) {
	if err := r.ownsSetup(); err != nil {
		return SyncResult{}, err
	}
	targets, err := SelectServices(cfg, names)
	if err != nil {
		return SyncResult{}, err
	}
	if err := r.checkDomainsFree(cfg); err != nil {
		return SyncResult{}, err
	}
	if err := r.engine().start(cfg, targets, true); err != nil {
		return SyncResult{}, err
	}
	res, err := r.sync(containerNames(targets))
	if err != nil {
		return res, err
	}
	r.reportReady(containerNames(targets))
	return res, nil
}

// containerNames returns the services whose readiness is a connection: the
// long-running ones that declare a port. A service with no port is not waited
// for, since nothing would ever connect; one that runs once and exits is done.
func containerNames(services []*Service) []string {
	out := make([]string, 0, len(services))
	for _, s := range services {
		if !s.OneShot && s.Port > 0 {
			out = append(out, s.ContainerName)
		}
	}
	return out
}

// sync republishes the routes. A route to a container in started is published
// only after the proxy opens a connection to it.
func (r *Runtime) sync(started []string) (SyncResult, error) {
	res, err := syncProxy(r.Machine, started, "", r.Progress)
	if err != nil {
		return res, err
	}
	for _, c := range res.Conflicts {
		r.say("warning: %s is claimed by both %s and %s; keeping %s",
			c.Domain, c.Kept, c.Dropped, c.Kept)
	}
	switch res.Action {
	case "stopped":
		r.say("no routes left; proxy stopped")
	default:
		r.say("proxy %s with %d route(s)", res.Action, len(res.Routes))
	}
	return res, nil
}

// requireSetup stops up when a part of the machine setup a project needs is
// missing. Machine setup is applied only by install, so up names the parts and
// the command and changes nothing. The store of certificates containers trust
// is state of this tool, derived from the system's roots and the authority, and
// is refreshed here so a container is created with the current one.
func (r *Runtime) requireSetup(cfg *Config) error {
	if _, err := os.Stat(filepath.Join(r.Machine.Dir, "ca.crt")); err == nil {
		if err := EnsureTrust(r.Machine.Dir); err != nil {
			return err
		}
	}
	items, err := r.setupItems(cfg.Domains()...)
	if err != nil {
		return err
	}
	var missing []string
	for _, it := range items {
		if !it.OK {
			missing = append(missing, it.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("the machine setup is missing %s; run bay install, which applies only what is missing (bay install -dry-run lists it)",
		strings.Join(missing, ", "))
}

// EnsureInstalled applies the machine setup that is missing and changes
// nothing that is in place, so running it again does nothing. It requests
// administrator rights only for a step that needs them.
//
// A machine that is not macOS has its own, smaller setup; see LinuxSetup.
func (r *Runtime) EnsureInstalled() error {
	if err := r.ensureSetup(); err != nil {
		return err
	}
	if _, err := EnsureSkills(); err != nil {
		return err
	}
	_, err := EnsureMCP()
	return err
}

// ensureSetup applies the setup parts other than the agent skill.
func (r *Runtime) ensureSetup() error {
	// Who owns the setup is recorded by the DNS agent, which only macOS has.
	if !hostSetsUpNames {
		l, err := NewLinuxSetup(r.Machine, r.DNSBin, r.Addr)
		if err != nil {
			return err
		}
		return l.Ensure(r.HelperBin, r.Elevate)
	}
	if err := r.ownsSetup(); err != nil {
		return err
	}
	ca, err := LoadOrCreateCA(r.Machine.Dir)
	if err != nil {
		return err
	}
	if err := EnsureTrust(r.Machine.Dir); err != nil {
		return err
	}
	in, err := r.install(ca.CertPath())
	if err != nil {
		return err
	}
	if err := in.Ensure(); err != nil {
		return err
	}
	if pending := in.PendingText(); len(pending) > 0 {
		return fmt.Errorf("setup did not complete: %s", strings.Join(pending, "; "))
	}
	// The relay listens where the record says, and the record is written here
	// and by the DNS agent, which both read the network as the user.
	if err := RecordRelayAddress(r.Machine.Dir, PeerRelayAddress()); err != nil {
		return err
	}
	if r.dnsAgentCurrent(in.Domains) {
		return nil
	}
	if r.DNSBin == "" {
		return fmt.Errorf("no bayd binary configured for the launchd job")
	}
	_, err = InstallDNSAgent(r.DNSBin, strings.Join(in.Domains, ","), r.Addr, r.Machine.Dir, r.Machine.LogDir())
	return err
}

// install describes the macOS setup for the registered domains.
func (r *Runtime) install(caPath string) (Install, error) {
	domains, err := r.Machine.Domains()
	if err != nil {
		return Install{}, err
	}
	return Install{
		Domains:     domains,
		Retire:      r.Retire,
		Addr:        r.Addr,
		CAPath:      caPath,
		UntrustCA:   r.RetiredCA,
		Helper:      r.HelperBin,
		Elevate:     r.Elevate,
		RelayBin:    r.DNSBin,
		State:       r.Machine.Dir,
		AppleDomain: BackendDomain,
	}, nil
}

func (r *Runtime) dnsAgentCurrent(domains []string) bool {
	return DNSAgentLoaded() && DNSAgentServes(domains, r.Addr, r.DNSBin, r.Machine.Dir) &&
		!residentStale(launchdPID(dnsAgentTarget()), r.DNSBin)
}

// SetupItems lists every part of this machine's setup and whether it is in
// place. "bay check", "doctor" and "install -dry-run" all read it, and
// "install" applies the items that are not, so the four never disagree.
// Extra domains are included as if registered, for a report that covers the
// project about to be brought up.
func (r *Runtime) SetupItems(extra ...string) ([]SetupItem, error) {
	items, err := r.setupItems(extra...)
	if err != nil {
		return nil, err
	}
	items = append(items, SkillItems()...)
	return append(items, MCPItems()...), nil
}

func (r *Runtime) setupItems(extra ...string) ([]SetupItem, error) {
	if !hostSetsUpNames {
		l, err := NewLinuxSetup(r.Machine, r.DNSBin, r.Addr)
		if err != nil {
			return nil, err
		}
		return l.Items(), nil
	}
	authority := AuthorityInfo(r.Machine.Dir)
	_, err := os.Stat(authority.Path)
	items := []SetupItem{{
		Name: "certificate authority " + authority.Path, OK: err == nil,
		SetupStep: SetupStep{Text: "create local certificate authority"},
	}}
	in, err := r.install(authority.Path)
	if err != nil {
		return nil, err
	}
	in.Domains = unionDomains(in.Domains, extra)
	items = append(items, in.Items()...)
	items = append(items, TrustItem(r.Machine.Dir))
	agent := SetupItem{
		Name: "dns agent " + DNSAgentLabel, OK: r.dnsAgentCurrent(in.Domains),
		SetupStep: SetupStep{Text: "register " + DNSAgentLabel},
	}
	if residentStale(launchdPID(dnsAgentTarget()), r.DNSBin) {
		agent.Name += " (runs an older " + r.DNSBin + ")"
	}
	return append(items, agent), nil
}

func unionDomains(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var out []string
	for _, d := range append(append([]string{}, a...), b...) {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// LoadGroup reads a registered project's Compose file by project name.
func (r *Runtime) LoadGroup(name string) (*Config, error) {
	groups, err := r.Machine.Groups()
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.Name == name {
			return LoadNamed(r.Machine, g.StackPath, g.Name)
		}
	}
	return nil, fmt.Errorf("no registered group named %q", name)
}

// SelectServices returns the named services of a project, or every service when
// no name is given.
func SelectServices(cfg *Config, names []string) ([]*Service, error) {
	if len(names) == 0 {
		return cfg.Sorted(), nil
	}
	out := make([]*Service, 0, len(names))
	for _, n := range names {
		s, ok := cfg.Services[strings.ToLower(n)]
		if !ok {
			known := make([]string, 0, len(cfg.Services))
			for k := range cfg.Services {
				known = append(known, k)
			}
			sort.Strings(known)
			return nil, fmt.Errorf("group %q has no service %q (has %s)",
				cfg.Name, n, strings.Join(known, ", "))
		}
		out = append(out, s)
	}
	return out, nil
}

// WaitRunning returns when the named container is running and has an address,
// or when the timeout expires.
func WaitRunning(name string, timeout time.Duration) (Instance, error) {
	deadline := time.Now().Add(timeout)
	for {
		in, ok, err := Lookup(name)
		if err != nil {
			return Instance{}, err
		}
		if ok && in.State == "running" && in.IPv4 != "" {
			return in, nil
		}
		if time.Now().After(deadline) {
			return Instance{}, fmt.Errorf("%s did not start within %s", name, timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ProjectNameAt returns the name of the project at path, which is a Compose file
// or the directory holding one. The file names its project when it can be read.
// When it cannot, the registry entry recorded for that path does, so a project
// whose file is gone can still be stopped and removed.
func ProjectNameAt(m *Machine, path string) (string, error) {
	cfg, loadErr := LoadNamed(m, path, "")
	if loadErr == nil {
		return cfg.Name, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", loadErr
	}
	groups, err := m.Groups()
	if err != nil {
		return "", err
	}
	for _, g := range groups {
		if g.StackPath == abs || filepath.Dir(g.StackPath) == abs {
			return g.Name, nil
		}
	}
	// A file that fails validation still names its project, and down of a
	// project whose up was refused has nothing to remove and succeeds.
	if name, ok := declaredName(abs); ok {
		return name, nil
	}
	return "", fmt.Errorf("%w, and no project is registered for %s", loadErr, abs)
}

// declaredName reads only the project name of a Compose file: its top-level
// name, or the name of its directory. A name built from variables is not read.
func declaredName(path string) (string, bool) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, "compose.yaml")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var head struct {
		Name string `yaml:"name"`
	}
	if yaml.Unmarshal(b, &head) != nil || strings.Contains(head.Name, "$") {
		return "", false
	}
	name := head.Name
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	name = strings.ToLower(name)
	return name, validName(name)
}

// routed reports whether the proxy routes a domain to a service of cfg.
func routed(cfg *Config) bool {
	for _, s := range cfg.Sorted() {
		if len(s.Domains) > 0 {
			return true
		}
	}
	return false
}
