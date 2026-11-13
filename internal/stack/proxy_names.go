package stack

import (
	"encoding/json"
	"sort"
	"strings"
)

// On Docker, a container reaches the names the proxy serves through Docker's
// own DNS. Docker answers a container's network aliases to every container on
// the same user-defined network, so the proxy carries every name it serves, its
// own routes' domains and its peers', as an alias on the proxy network, and a
// service on that network resolves them to the proxy. The system's DNS is not
// changed, and a container that reads the host's /etc/hosts is not needed.
//
// An alias cannot be changed on a connected container, so a changed list is
// applied by connecting the proxy to the network again. The container is not
// created again and its published ports stay. A list that is already the
// proxy's is left alone.

// servedNames returns every name one proxy configuration serves.
func servedNames(c NginxConfig) []string {
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		d = strings.ToLower(d)
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, r := range c.Routes {
		add(r.Domain)
	}
	for _, p := range c.PeerRoutes {
		add(p.Domain)
	}
	sort.Strings(out)
	return out
}

// ProxyNames returns the domain names the running proxy of an engine answers
// to on its network. Docker adds the container's own name and ID, which hold no
// dot and are left out.
func ProxyNames(engine string) ([]string, error) {
	out, err := runEngine(engine, "inspect", ProxyName)
	if err != nil {
		return nil, err
	}
	return decodeProxyAliases(out, proxyNetwork(engine)), nil
}

func decodeProxyAliases(out []byte, network string) []string {
	var raw []struct {
		NetworkSettings struct {
			Networks map[string]struct {
				Aliases []string `json:"Aliases"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if json.Unmarshal(out, &raw) != nil || len(raw) == 0 {
		return nil
	}
	var names []string
	for _, a := range raw[0].NetworkSettings.Networks[network].Aliases {
		if strings.Contains(a, ".") {
			names = append(names, strings.ToLower(a))
		}
	}
	sort.Strings(names)
	return names
}

// ensureProxyNames gives the Docker proxy exactly these names on its network.
func ensureProxyNames(engine string, names []string) error {
	if engine != DockerEngine {
		return nil
	}
	current, err := ProxyNames(engine)
	if err != nil {
		return err
	}
	if strings.Join(current, " ") == strings.Join(names, " ") {
		return nil
	}
	network := proxyNetwork(engine)
	if _, err := runEngine(engine, "network", "disconnect", network, ProxyName); err != nil {
		return err
	}
	_, err = runEngine(engine, connectArguments(network, names)...)
	return err
}

func connectArguments(network string, names []string) []string {
	args := []string{"network", "connect"}
	for _, n := range names {
		args = append(args, "--alias", n)
	}
	return append(args, network, ProxyName)
}
