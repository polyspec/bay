package stack

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Snapshot holds the machine state: what is set up, what is running, and which
// project owns it. The command line and the application both read it.
type Snapshot struct {
	Machine      MachineStatus `json:"machine"`
	Groups       []GroupStatus `json:"groups"`
	Certificates CertStatus    `json:"certificates"`
	TakenAt      time.Time     `json:"takenAt"`
}

// CertStatus holds the certificates the local authority has issued.
type CertStatus struct {
	Authority CertInfo   `json:"authority"`
	Issued    []CertInfo `json:"issued"`
}

// DomainStatus describes one delegated domain and the projects that pin it.
type DomainStatus struct {
	Name string `json:"name"`
	// Default says projects fall back to this one.
	Default bool `json:"default"`
	// PinnedBy lists projects whose Compose file names it. The domain cannot be
	// removed from the machine while the list is not empty.
	PinnedBy []string `json:"pinnedBy,omitempty"`
}

type MachineStatus struct {
	StateDir   string         `json:"stateDir"`
	Domains    []string       `json:"domains"`
	DomainList []DomainStatus `json:"domainList"`
	// Proxies holds one entry per engine present, because each engine runs its
	// own proxy and serves only its own containers.
	Proxies []ProxyStatus `json:"proxies"`
	DNS     DNSStatus     `json:"dns"`
	CAPath  string        `json:"caPath"`
	// CATrusted says whether the CA verifies against the system trust store.
	CATrusted bool `json:"caTrusted"`
	// Pending lists the setup steps that still need root. It is empty on a
	// machine that is ready.
	Pending []string `json:"pending"`
}

type ProxyStatus struct {
	Name string `json:"name"`
	// Engine names the engine this proxy runs on.
	Engine string `json:"engine"`
	// State is the runtime's word for it, or "absent" when no such container
	// exists - which is the normal state when nothing is up.
	State  string `json:"state"`
	IPv4   string `json:"ipv4"`
	Routes int    `json:"routes"`
	// Generation is the configuration the proxy reports it is serving, read
	// from its health endpoint. Empty when it cannot be reached.
	Generation string `json:"generation"`
}

type DNSStatus struct {
	Label string `json:"label"`
	Addr  string `json:"addr"`
	// Loaded says launchd knows the job; Current says it was registered for
	// exactly the domains the machine now serves.
	Loaded  bool `json:"loaded"`
	Current bool `json:"current"`
}

type GroupStatus struct {
	Name      string `json:"name"`
	StackPath string `json:"stackPath"`
	// Domain is the domain this project's services use. It is inherited from
	// the machine unless the Compose file sets one.
	Domain string `json:"domain"`
	// DomainPinned says the Compose file named it rather than inheriting.
	DomainPinned bool            `json:"domainPinned"`
	Domains      []string        `json:"domains"`
	Services     []ServiceStatus `json:"services"`
	// Error is set when the group's stack file could not be read, in which
	// case Services only holds what is running.
	Error string `json:"error,omitempty"`
}

type ServiceStatus struct {
	Name      string `json:"name"`
	Container string `json:"container"`
	Domain    string `json:"domain"`
	Image     string `json:"image"`
	Port      int    `json:"port"`
	Scheme    string `json:"scheme"`
	// State is "running", "starting", "stopped", or "absent" when no container
	// exists. "starting" means the container runs but does not yet accept a
	// connection on its port.
	State string `json:"state"`
	IPv4  string `json:"ipv4"`
	// Routed says the proxy currently forwards this service's domain to it.
	Routed bool `json:"routed"`
	// Internal marks a service with no domain. Other services reach it by name;
	// the proxy does not route to it.
	Internal bool   `json:"internal"`
	URL      string `json:"url"`
	// Address is how other services reach it from inside the network.
	Address string `json:"address"`
	// Started is when the runtime started the container, in RFC 3339. It is
	// empty unless the container is running.
	Started string `json:"started,omitempty"`
}

// Uptime returns how long the container has been running, or zero when the
// start time is unknown.
func (s ServiceStatus) Uptime() time.Duration {
	if s.Started == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s.Started)
	if err != nil {
		return 0
	}
	if d := time.Since(t); d > 0 {
		return d
	}
	return 0
}

// Running reports that the service accepts connections. A starting service is
// not counted, because nothing can use it yet.
func (s ServiceStatus) Running() bool { return s.State == "running" }

// Live reports that the container is running, whether or not it is ready.
func (s ServiceStatus) Live() bool { return s.State == "running" || s.State == "starting" }

// Take reads the machine's current state. addr is the address bayd
// listens on.
func Take(m *Machine, addr string) (Snapshot, error) {
	snap := Snapshot{TakenAt: time.Now()}

	domains, err := m.Domains()
	if err != nil {
		return snap, err
	}
	authority := AuthorityInfo(m.Dir)
	install := Install{Domains: domains, Addr: addr, CAPath: authority.Path}
	domainList, err := domainStatuses(m, domains)
	if err != nil {
		return snap, err
	}
	snap.Machine = MachineStatus{
		StateDir:   m.Dir,
		Domains:    domains,
		DomainList: domainList,
		CAPath:     authority.Path,
		CATrusted:  authority.Trusted,
		Pending:    install.Pending(),
		DNS: DNSStatus{
			Label:   DNSAgentLabel,
			Addr:    addr,
			Loaded:  DNSAgentLoaded(),
			Current: DNSAgentServes(domains, addr, ""),
		},
	}

	instances, err := Instances()
	if err != nil {
		return snap, err
	}
	routed := map[string]bool{}
	routes, _, err := Routes()
	if err != nil {
		return snap, err
	}
	for _, r := range routes {
		routed[r.Domain] = true
	}

	snap.Machine.Proxies, err = proxyStatuses(routes)
	if err != nil {
		return snap, err
	}

	byContainer := map[string]ServiceInstance{}
	for _, in := range instances {
		byContainer[in.Container] = in
	}
	groups, err := m.Groups()
	if err != nil {
		return snap, err
	}
	// A stopped project still asks for its domains, so what the projects declare
	// is collected alongside what is routed right now. A project whose file is
	// still there but could not be read asks for something unknown.
	use := CertUse{}
	for _, g := range groups {
		status := groupStatus(m, g, byContainer, routed)
		snap.Groups = append(snap.Groups, status)
		if status.Error != "" && fileExists(g.StackPath) {
			use.Unread = true
		}
		for _, svc := range status.Services {
			if svc.Domain != "" {
				use.Declared = append(use.Declared, svc.Domain)
			}
		}
	}
	for _, r := range routes {
		use.Routed = append(use.Routed, r.Domain)
	}

	issued, err := Certificates(m.CertDir(), use)
	if err != nil {
		return snap, err
	}
	snap.Certificates = CertStatus{Authority: authority, Issued: issued}
	return snap, nil
}

// fileExists reports whether a path is there to be read. A project whose file
// is gone asks for nothing; one whose file is present but unreadable asks for
// something that is not known.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// domainStatuses returns, for each delegated domain, whether it is the default
// and which projects pin it.
func domainStatuses(m *Machine, domains []string) ([]DomainStatus, error) {
	settings, err := m.Settings()
	if err != nil {
		return nil, err
	}
	groups, err := m.Groups()
	if err != nil {
		return nil, err
	}
	pinned := map[string][]string{}
	for _, g := range groups {
		for _, d := range g.Domains {
			d = strings.ToLower(strings.Trim(d, "."))
			pinned[d] = append(pinned[d], g.Name)
		}
	}
	out := make([]DomainStatus, 0, len(domains))
	for _, d := range domains {
		out = append(out, DomainStatus{
			Name:     d,
			Default:  d == settings.Domain,
			PinnedBy: pinned[d],
		})
	}
	return out, nil
}

// proxyStatuses describes the proxy of every engine present. Routes are counted
// per engine because a proxy serves only its own engine's containers.
func proxyStatuses(routes []Route) ([]ProxyStatus, error) {
	var out []ProxyStatus
	for _, engine := range Engines() {
		st := ProxyStatus{
			Name:   ProxyName,
			Engine: engine,
			State:  "absent",
			Routes: len(RoutesOn(routes, engine)),
		}
		in, found, err := lookupOn(engine, ProxyName)
		if err != nil {
			return nil, err
		}
		if found {
			st.State = in.State
			st.IPv4 = ProxyHostAddr(engine, in)
			if in.State == "running" && st.IPv4 != "" {
				if gen, err := fetchGeneration(
					&http.Client{Timeout: 2 * time.Second}, st.IPv4); err == nil {
					st.Generation = gen
				}
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// ServingRoutes totals the routes every engine's proxy serves.
func (m MachineStatus) ServingRoutes() int {
	n := 0
	for _, p := range m.Proxies {
		n += p.Routes
	}
	return n
}

// ProxiesServing reports that every proxy with routes to serve is running and
// answering. One engine's proxy being down leaves its domains unreachable, so
// this is false unless all of them are up.
func (m MachineStatus) ProxiesServing() bool {
	serving := false
	for _, p := range m.Proxies {
		if p.Routes == 0 {
			continue
		}
		if p.State != "running" || p.Generation == "" {
			return false
		}
		serving = true
	}
	return serving
}

// ProxyAddrs lists the addresses the running proxies are reached at.
func (m MachineStatus) ProxyAddrs() []string {
	var out []string
	for _, p := range m.Proxies {
		if p.State == "running" && p.IPv4 != "" {
			out = append(out, p.IPv4)
		}
	}
	return out
}

// groupStatus describes one project from its Compose file, so services that are
// not running are included. When the file cannot be read, the project is
// described from its containers and the error is recorded.
func groupStatus(m *Machine, g GroupRef, byContainer map[string]ServiceInstance, routed map[string]bool) GroupStatus {
	out := GroupStatus{Name: g.Name, StackPath: g.StackPath, Domains: g.Domains}

	cfg, err := LoadIn(m, g.StackPath)
	if err != nil {
		out.Error = err.Error()
		for _, in := range byContainer {
			if in.Group == g.Name {
				out.Services = append(out.Services, serviceFromInstance(in, routed))
			}
		}
		return out
	}

	out.Domain, out.DomainPinned, out.Domains = cfg.Domain, cfg.DomainPinned, cfg.Domains()
	for _, s := range cfg.Sorted() {
		st := ServiceStatus{
			Name:      s.Name,
			Container: s.ContainerName,
			Domain:    s.Domain,
			Image:     s.Image,
			Port:      s.Port,
			Scheme:    "http",
			State:     "absent",
			Internal:  s.Internal,
			Address:   fmt.Sprintf("%s.%s:%d", s.ContainerName, BackendDomain, s.Port),
		}
		if !s.Internal {
			st.URL = "https://" + s.Domain + "/"
		}
		if s.TLS {
			st.Scheme = "https"
		}
		if in, ok := byContainer[s.ContainerName]; ok {
			st.State, st.IPv4, st.Started = in.State, in.IPv4, in.Started
			if in.Running() && !in.Ready() {
				st.State = "starting"
			}
		}
		st.Routed = routed[s.Domain] && st.Running()
		out.Services = append(out.Services, st)
	}
	return out
}

func serviceFromInstance(in ServiceInstance, routed map[string]bool) ServiceStatus {
	state := in.State
	if in.Running() && !in.Ready() {
		state = "starting"
	}
	st := ServiceStatus{
		Name:      in.Service,
		Container: in.Container,
		Domain:    in.Domain,
		Port:      in.Port,
		Scheme:    in.Scheme,
		State:     state,
		IPv4:      in.IPv4,
		Started:   in.Started,
		Routed:    routed[in.Domain] && in.Running(),
		Internal:  in.Domain == "",
		Address:   fmt.Sprintf("%s.%s:%d", in.Container, BackendDomain, in.Port),
	}
	if !st.Internal {
		st.URL = "https://" + in.Domain + "/"
	}
	return st
}
