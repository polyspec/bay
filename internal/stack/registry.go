package stack

import (
	"errors"
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
	Domain    string
	Port      int
	Scheme    string
	State     string
	IPv4      string
	// Started is when the runtime started the container, in RFC 3339. It is
	// empty for a container that has never run.
	Started string
}

func (s ServiceInstance) Running() bool { return s.State == "running" }

// Backend returns the name and port the proxy connects to. nginx resolves this
// name through the runtime DNS and caches answers for the configured interval.
func (s ServiceInstance) Backend() string {
	return s.Container + "." + BackendDomain + ":" + strconv.Itoa(s.Port)
}

// Instances returns every container labelled as a bay service, ordered
// by project then domain.
func Instances() ([]ServiceInstance, error) {
	list, err := List()
	if err != nil {
		return nil, err
	}
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
			Domain:    strings.ToLower(in.Labels[LabelDomain]),
			Port:      port,
			Scheme:    scheme,
			State:     in.State,
			IPv4:      in.IPv4,
			Started:   in.Started,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Domain < out[j].Domain
	})
	return out, nil
}

// Routes returns one entry per running service that claims a domain, across
// every project. When two services claim the same domain, the first in sorted
// order is kept, so the result does not depend on start order.
func Routes() ([]Route, []DomainConflict, error) {
	instances, err := Instances()
	if err != nil {
		return nil, nil, err
	}
	var routes []Route
	var conflicts []DomainConflict
	claimed := map[string]ServiceInstance{}
	for _, in := range instances {
		if !in.Running() || in.Domain == "" {
			continue
		}
		if prev, taken := claimed[in.Domain]; taken {
			conflicts = append(conflicts, DomainConflict{
				Domain: in.Domain, Kept: prev.Container, Dropped: in.Container,
			})
			continue
		}
		claimed[in.Domain] = in
		routes = append(routes, Route{
			Domain:  in.Domain,
			Backend: in.Backend(),
			Scheme:  in.Scheme,
			ipv4:    in.IPv4,
			started: in.Started,
		})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Domain < routes[j].Domain })
	return routes, conflicts, nil
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
func NetworkGateway() (string, error) {
	list, err := List()
	if err != nil {
		return "", err
	}
	for _, in := range list {
		if in.State == "running" && in.Gateway != "" {
			return in.Gateway, nil
		}
	}
	return "", errNoGateway
}
