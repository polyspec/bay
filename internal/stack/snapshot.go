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
	// Proxies holds one entry per engine present. Each engine runs its own
	// proxy and serves only its own containers.
	Proxies []ProxyStatus `json:"proxies"`
	DNS     DNSStatus     `json:"dns"`
	CAPath  string        `json:"caPath"`
	// CATrusted says whether the CA verifies against the system trust store.
	CATrusted bool `json:"caTrusted"`
	// Pending lists the setup steps that still need root. It is empty on a
	// machine that is ready.
	Pending []string `json:"pending"`
	// Peering says the link is open, and Link is where it answers.
	Peering bool `json:"peering"`
	// LinkLog says the proxy records what crosses a peer's link.
	LinkLog bool   `json:"linkLog"`
	Link    string `json:"link,omitempty"`
	// Peers are the machines whose domains this one reaches.
	Peers []Peer `json:"peers,omitempty"`
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
	// Domains are the domains the proxy routes to the service, in the order of
	// x-bay.domains.
	Domains []string `json:"domains"`
	Image   string   `json:"image"`
	// Port is the port the service declares, absent when it declares none.
	Port   int    `json:"port,omitempty"`
	Scheme string `json:"scheme"`
	// State is the container's state as the engine reports it: "running",
	// "stopped", or "absent" when no container exists.
	State string `json:"state"`
	// Accepting says whether the declared port accepted a connection when the
	// machine was read. It is absent when nothing was measured: the container
	// is not running or the service declares no port.
	Accepting *bool  `json:"accepting,omitempty"`
	IPv4      string `json:"ipv4"`
	// Routed says the proxy currently forwards every domain of this service to
	// it.
	Routed bool `json:"routed"`
	// Internal marks a service with no domain. Other services reach it by name;
	// the proxy does not route to it.
	Internal bool `json:"internal"`
	// URLs are the addresses of the domains, in the order of Domains.
	URLs []string `json:"urls"`
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

// Live reports that the service's container is running.
func (s ServiceStatus) Live() bool { return s.State == "running" }

// Closed reports that the service declares a port, its container runs, and the
// port did not accept a connection when it was measured.
func (s ServiceStatus) Closed() bool { return s.Accepting != nil && !*s.Accepting }

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
	settings, err := m.Settings()
	if err != nil {
		return snap, err
	}
	peers, err := Peers(m.Dir)
	if err != nil {
		return snap, err
	}
	link := ""
	if settings.Peering {
		link = PeerLinkAddress(LANAddress())
	}
	snap.Machine = MachineStatus{
		StateDir:   m.Dir,
		Peering:    settings.Peering,
		LinkLog:    settings.LinkLog,
		Link:       link,
		Peers:      peers,
		Domains:    domains,
		DomainList: domainList,
		CAPath:     authority.Path,
		CATrusted:  authority.Trusted,
		Pending:    install.PendingText(),
		DNS: DNSStatus{
			Label:   DNSAgentLabel,
			Addr:    addr,
			Loaded:  DNSAgentLoaded(),
			Current: DNSAgentServes(domains, addr, "", m.Dir),
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
			use.Declared = append(use.Declared, svc.Domains...)
		}
	}
	for _, r := range routes {
		use.Routed = append(use.Routed, r.Domain)
	}
	// This machine serves a peer's domain with a certificate it issued, so that
	// certificate is in use while the peer is approved.
	if peers, err := Peers(m.Dir); err == nil {
		own := make([]string, 0, len(routes))
		for _, r := range routes {
			own = append(own, r.Domain)
		}
		for d := range PeerDomains(peers, own) {
			use.Peered = append(use.Peered, d)
		}
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

// proxyStatuses returns the status of every present engine's proxy. Routes are
// counted per engine, because a proxy serves only its own engine's containers.
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

// ServingRoutes returns the total number of routes served by all proxies.
func (m MachineStatus) ServingRoutes() int {
	n := 0
	for _, p := range m.Proxies {
		n += p.Routes
	}
	return n
}

// ProxiesServing reports whether every engine's proxy is running and answering.
// A proxy with no route of its own still answers the domains approved peers
// hold, so having no route is not a reason to leave it out.
func (m MachineStatus) ProxiesServing() bool {
	for _, p := range m.Proxies {
		if p.State != "running" || p.Generation == "" {
			return false
		}
	}
	return len(m.Proxies) > 0
}

// Serves is the number of names this machine answers: the domains its own
// containers hold and the domains approved peers hold. A machine that answers
// none of them needs no proxy.
func (m MachineStatus) Serves() int {
	n := m.ServingRoutes()
	for _, p := range m.Peers {
		n += len(p.Domains)
	}
	return n
}

// ProxyAddrs returns the addresses of the running proxies.
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
			Domains:   s.Domains,
			Image:     s.Image,
			Port:      s.Port,
			Scheme:    "http",
			State:     "absent",
			Internal:  s.Internal(),
			URLs:      domainURLs(s.Domains),
			Address:   serviceAddress(s.ContainerName, s.Port),
		}
		if s.TLS {
			st.Scheme = "https"
		}
		if in, ok := byContainer[s.ContainerName]; ok {
			st.State, st.IPv4, st.Started = in.State, in.IPv4, in.Started
			st.Accepting = accepting(in, s.Port)
		}
		st.Routed = allRouted(s.Domains, routed) && st.Live()
		out.Services = append(out.Services, st)
	}
	return out
}

// domainURLs returns the HTTPS address of each domain.
func domainURLs(domains []string) []string {
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		out = append(out, "https://"+d+"/")
	}
	return out
}

// allRouted reports whether domains is not empty and the proxy routes each of
// them.
func allRouted(domains []string, routed map[string]bool) bool {
	for _, d := range domains {
		if !routed[d] {
			return false
		}
	}
	return len(domains) > 0
}

// serviceFromInstance describes a container whose project file could not be
// read. Its declared port is then unknown: the port recorded on the container
// can be one that was assumed when none was declared, so no port is given and
// no connection is measured.
func serviceFromInstance(in ServiceInstance, routed map[string]bool) ServiceStatus {
	return ServiceStatus{
		Name:      in.Service,
		Container: in.Container,
		Domains:   in.Domains,
		Scheme:    in.Scheme,
		State:     in.State,
		IPv4:      in.IPv4,
		Started:   in.Started,
		Routed:    allRouted(in.Domains, routed) && in.Running(),
		Internal:  len(in.Domains) == 0,
		URLs:      domainURLs(in.Domains),
		Address:   serviceAddress(in.Container, 0),
	}
}

// serviceAddress is how the other services reach a container: its name, and
// its port when it declares one. A service without a port is reached by name
// alone; writing ":0" would name a port it does not have.
func serviceAddress(container string, port int) string {
	if port == 0 {
		return container + "." + BackendDomain
	}
	return fmt.Sprintf("%s.%s:%d", container, BackendDomain, port)
}
