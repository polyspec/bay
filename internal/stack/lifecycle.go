package stack

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	return e
}

func (r *Runtime) say(format string, args ...any) {
	if r.Progress != nil {
		r.Progress(fmt.Sprintf(format, args...))
	}
}

// Up registers the project, applies any missing machine setup, starts every
// service and republishes the routes.
func (r *Runtime) Up(cfg *Config) (SyncResult, error) {
	if err := r.ownsSetup(); err != nil {
		return SyncResult{}, err
	}
	// Registering is what makes the project this machine's, and the window
	// draws from the registry, so it comes before the work: the project is on
	// the screen while its images are pulled, and it is still there when the
	// run fails.
	if err := r.Machine.Register(cfg.Ref()); err != nil {
		return SyncResult{}, err
	}
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
	if err := r.EnsureInstalled(); err != nil {
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
	res, err := r.sync(containerNames(cfg.Sorted()))
	if err != nil {
		return res, err
	}
	r.reportReady(containerNames(cfg.Sorted()))
	return res, nil
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
	if err := r.engine().removeProject(group); err != nil {
		return SyncResult{}, err
	}
	if err := r.Machine.Unregister(group); err != nil {
		return SyncResult{}, err
	}
	return r.sync(nil)
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

// containerNames returns the long-running services that need TCP readiness and,
// when routed, a connection from the proxy.
func containerNames(services []*Service) []string {
	out := make([]string, 0, len(services))
	for _, s := range services {
		if !s.OneShot {
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

// EnsureInstalled applies the machine setup missing for the registered domains.
// It does nothing when the machine is set up, and requests administrator rights
// only when a resolver entry changes.
func (r *Runtime) EnsureInstalled() error {
	if err := r.ownsSetup(); err != nil {
		return err
	}
	ca, err := LoadOrCreateCA(r.Machine.Dir)
	if err != nil {
		return err
	}
	domains, err := r.Machine.Domains()
	if err != nil {
		return err
	}
	in := Install{
		Domains:   domains,
		Retire:    r.Retire,
		Addr:      r.Addr,
		CAPath:    ca.CertPath(),
		UntrustCA: r.RetiredCA,
		Helper:    r.HelperBin,
		Elevate:   r.Elevate,
	}
	if err := in.Ensure(); err != nil {
		return err
	}
	if pending := in.PendingText(); len(pending) > 0 {
		return fmt.Errorf("setup did not complete: %s", strings.Join(pending, "; "))
	}
	if DNSAgentLoaded() && DNSAgentServes(domains, r.Addr, r.DNSBin, r.Machine.Dir) {
		return nil
	}
	if r.DNSBin == "" {
		return fmt.Errorf("no bayd binary configured for the launchd job")
	}
	_, err = InstallDNSAgent(r.DNSBin, strings.Join(domains, ","), r.Addr, r.Machine.Dir, r.Machine.LogDir())
	return err
}

// LoadGroup reads a registered project's Compose file by project name.
func (r *Runtime) LoadGroup(name string) (*Config, error) {
	groups, err := r.Machine.Groups()
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.Name == name {
			return LoadIn(r.Machine, g.StackPath)
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
	cfg, loadErr := LoadIn(m, path)
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
	return "", fmt.Errorf("%w, and no project is registered for %s", loadErr, abs)
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
