package stack

import (
	"fmt"
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
	// Elevate runs the helper with administrator rights. Nil uses sudo, which
	// only works from a terminal.
	Elevate Elevator
	// Progress, when set, is called with a line describing each step.
	Progress func(string)
}

func (r *Runtime) say(format string, args ...any) {
	if r.Progress != nil {
		r.Progress(fmt.Sprintf(format, args...))
	}
}

// Up registers the project, applies any missing machine setup, starts every
// service and republishes the routes.
func (r *Runtime) Up(cfg *Config) (SyncResult, error) {
	if err := newServiceEngine(r.Machine.Dir).preflight(cfg.Name, cfg.Sorted()); err != nil {
		return SyncResult{}, err
	}
	if err := newServiceEngine(r.Machine.Dir).prepareVolumes(cfg.Name, cfg.Sorted(), false); err != nil {
		return SyncResult{}, err
	}
	if err := r.checkDomainsFree(cfg); err != nil {
		return SyncResult{}, err
	}
	if err := r.Machine.Register(cfg.Ref()); err != nil {
		return SyncResult{}, err
	}
	if err := r.EnsureInstalled(); err != nil {
		return SyncResult{}, err
	}
	if err := newServiceEngine(r.Machine.Dir).start(cfg, cfg.Sorted(), false); err != nil {
		return SyncResult{}, err
	}
	res, err := r.sync()
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
	r.say("still starting after %s: %s", ReadyTimeout, strings.Join(pending, ", "))
}

// checkDomainsFree reports an error when another running project already serves
// a domain this project claims. Starting anyway would remove the running
// project's route.
func (r *Runtime) checkDomainsFree(cfg *Config) error {
	instances, err := Instances()
	if err != nil {
		return err
	}
	claimed := map[string]ServiceInstance{}
	for _, in := range instances {
		if in.Running() && in.Domain != "" && in.Group != cfg.Name {
			claimed[in.Domain] = in
		}
	}
	for _, s := range cfg.Sorted() {
		if s.Internal {
			continue
		}
		if held, taken := claimed[s.Domain]; taken {
			return fmt.Errorf("%s is served by project %q; change the domain of service %q or stop %q",
				s.Domain, held.Group, s.Name, held.Group)
		}
	}
	return nil
}

// Down removes the project's containers and withdraws its routes. Other
// projects are not changed.
func (r *Runtime) Down(cfg *Config) (SyncResult, error) {
	if err := newServiceEngine(r.Machine.Dir).stop(cfg, cfg.Sorted(), true); err != nil {
		return SyncResult{}, err
	}
	if err := r.Machine.Unregister(cfg.Name); err != nil {
		return SyncResult{}, err
	}
	return r.sync()
}

// StartServices starts the named services and creates any missing container. An
// empty list selects every service in the project.
func (r *Runtime) StartServices(cfg *Config, names []string) (SyncResult, error) {
	targets, err := SelectServices(cfg, names)
	if err != nil {
		return SyncResult{}, err
	}
	if err := r.checkDomainsFree(cfg); err != nil {
		return SyncResult{}, err
	}
	if err := newServiceEngine(r.Machine.Dir).start(cfg, targets, false); err != nil {
		return SyncResult{}, err
	}
	res, err := r.sync()
	if err != nil {
		return res, err
	}
	r.reportReady(containerNames(targets))
	return res, nil
}

// StopServices stops the named services and leaves their containers in place.
func (r *Runtime) StopServices(cfg *Config, names []string) (SyncResult, error) {
	targets, err := SelectServices(cfg, names)
	if err != nil {
		return SyncResult{}, err
	}
	if err := newServiceEngine(r.Machine.Dir).stop(cfg, targets, false); err != nil {
		return SyncResult{}, err
	}
	return r.sync()
}

func (r *Runtime) RestartServices(cfg *Config, names []string) (SyncResult, error) {
	targets, err := SelectServices(cfg, names)
	if err != nil {
		return SyncResult{}, err
	}
	if err := r.checkDomainsFree(cfg); err != nil {
		return SyncResult{}, err
	}
	if err := newServiceEngine(r.Machine.Dir).start(cfg, targets, true); err != nil {
		return SyncResult{}, err
	}
	res, err := r.sync()
	if err != nil {
		return res, err
	}
	r.reportReady(containerNames(targets))
	return res, nil
}

// containerNames returns the long-running services that need TCP readiness.
func containerNames(services []*Service) []string {
	out := make([]string, 0, len(services))
	for _, s := range services {
		if !s.OneShot {
			out = append(out, s.ContainerName)
		}
	}
	return out
}

func (r *Runtime) sync() (SyncResult, error) {
	res, err := SyncProxy(r.Machine)
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
		Addr:      r.Addr,
		CAPath:    ca.CertPath(),
		UntrustCA: r.RetiredCA,
		Helper:    r.HelperBin,
		Elevate:   r.Elevate,
	}
	if err := in.Ensure(); err != nil {
		return err
	}
	if pending := in.Pending(); len(pending) > 0 {
		return fmt.Errorf("setup did not complete: %s", strings.Join(pending, "; "))
	}
	if DNSAgentLoaded() && DNSAgentServes(domains, r.Addr, r.DNSBin) {
		return nil
	}
	if r.DNSBin == "" {
		return fmt.Errorf("no bayd binary configured for the launchd job")
	}
	_, err = InstallDNSAgent(r.DNSBin, strings.Join(domains, ","), r.Addr, r.Machine.LogDir())
	return err
}

// describe returns the service's domain, or "internal" when it has none.
func describe(s *Service) string {
	if s.Internal {
		return "internal"
	}
	return s.Domain
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
