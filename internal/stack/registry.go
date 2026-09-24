package stack

import (
	"errors"
	"net"
	"sort"
	"strconv"
	"strings"
)

// errNoGateway is returned when no container is running, so the network gateway
// cannot be read.
var errNoGateway = errors.New("no running container to read the network gateway from")

// ServiceInstance is a service container and its routing settings, read from
// the container's labels. Several projects share one proxy because the
// configuration is generated from the containers that are running.
type ServiceInstance struct {
	Container string
	Group     string
	Service   string
	// Domains are the domains the proxy routes to the container. An internal
	// service has none.
	Domains []string
	Port    int
	Scheme  string
	State   string
	IPv4    string
	// Started is when the runtime started the container, in RFC 3339. It is
	// empty for a container that has never run.
	Started string
	// Engine is the engine running this container.
	Engine string
}

func (s ServiceInstance) Running() bool { return s.State == "running" }

// LabelledDomains returns the domains of a container's LabelDomain label,
// which serviceArguments writes separated by commas.
func LabelledDomains(labels map[string]string) []string {
	value := strings.ToLower(labels[LabelDomain])
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

// Addr returns the address and port the container holds now. It is empty until
// the runtime has given the container an address.
func (s ServiceInstance) Addr() string {
	if s.IPv4 == "" {
		return ""
	}
	return net.JoinHostPort(s.IPv4, strconv.Itoa(s.Port))
}

// Backend returns the name and port the proxy connects to. nginx resolves this
// name through the runtime DNS and caches answers for the configured interval.
func (s ServiceInstance) Backend() string {
	return s.Container + "." + BackendDomain + ":" + strconv.Itoa(s.Port)
}

// Instances returns every container labelled as a bay service, ordered
// by project then container.
func Instances() ([]ServiceInstance, error) {
	list, err := List()
	if err != nil {
		return nil, err
	}
	return serviceInstances(list), nil
}

// serviceInstances reads the service containers of list from their labels,
// ordered by project then container.
func serviceInstances(list []Instance) []ServiceInstance {
	var out []ServiceInstance
	for _, in := range list {
		if in.Labels[LabelRole] != roleService {
			continue
		}
		port, err := strconv.Atoi(in.Labels[LabelPort])
		if err != nil || port <= 0 {
			port = 80
		}
		scheme := in.Labels[LabelScheme]
		if scheme != "https" {
			scheme = "http"
		}
		out = append(out, ServiceInstance{
			Container: in.Name,
			Group:     in.Labels[LabelGroup],
			Service:   in.Labels[LabelService],
			Domains:   LabelledDomains(in.Labels),
			Port:      port,
			Scheme:    scheme,
			State:     in.State,
			IPv4:      in.IPv4,
			Started:   in.Started,
			Engine:    in.Engine,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Container < out[j].Container
	})
	return out
}

// Routes returns one entry per running service that claims a domain, across
// every project. When two services claim the same domain, the first in sorted
// order is kept, so the result does not depend on start order.
func Routes() ([]Route, []DomainConflict, error) {
	instances, err := Instances()
	if err != nil {
		return nil, nil, err
	}
	routes, conflicts := routesFrom(instances)
	return routes, conflicts, nil
}

// routesFrom resolves domain claims over the containers of every engine. One
// domain is claimed once per machine. Each route records the engine of its
// container, because only that engine's proxy can serve it.
func routesFrom(instances []ServiceInstance) ([]Route, []DomainConflict) {
	var routes []Route
	var conflicts []DomainConflict
	claimed := map[string]ServiceInstance{}
	for _, in := range instances {
		if !in.Running() {
			continue
		}
		for _, d := range in.Domains {
			if prev, taken := claimed[d]; taken {
				conflicts = append(conflicts, DomainConflict{
					Domain: d, Kept: prev.Container, Dropped: in.Container,
				})
				continue
			}
			claimed[d] = in
			routes = append(routes, Route{
				Domain:  d,
				Address: in.Addr(),
				Backend: in.Backend(),
				Scheme:  in.Scheme,
				Engine:  in.Engine,
				ipv4:    in.IPv4,
				started: in.Started,
			})
		}
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Domain < routes[j].Domain })
	return routes, conflicts
}

// RoutesOn returns the routes served by one engine's proxy.
func RoutesOn(routes []Route, engine string) []Route {
	out := make([]Route, 0, len(routes))
	for _, r := range routes {
		if r.Engine == engine {
			out = append(out, r)
		}
	}
	return out
}

// DomainConflict reports two running containers claiming the same domain.
type DomainConflict struct {
	Domain  string
	Kept    string
	Dropped string
}

// NetworkGateway returns the container network's gateway, which is also the
// runtime's DNS server. The runtime reports it per attachment, so it is read
// from a running container.
func NetworkGateway(engine string) (string, error) {
	list, err := List()
	if err != nil {
		return "", err
	}
	// The gateway is written into one engine's proxy configuration, so it is
	// read from a container of that engine.
	for _, in := range list {
		if in.State == "running" && in.Gateway != "" && in.Engine == engine {
			return in.Gateway, nil
		}
	}
	return "", errNoGateway
}
