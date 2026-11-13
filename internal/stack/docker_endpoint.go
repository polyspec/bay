package stack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The docker command can be pointed at a daemon on another machine, through
// DOCKER_HOST or through a context such as one created with
// `docker context create --docker host=ssh://...`. Such a daemon is not this
// machine's engine: it publishes the proxy's ports on its own loopback address,
// it mounts the paths of its own file system, and the host here has no route to
// its containers, so every rule of the architecture fails for it. The docker
// command is counted as this machine's engine only when it speaks to a daemon on
// this machine.

// dockerEndpoint returns the address the docker command sends its requests to,
// read the way the command reads it: DOCKER_HOST, then the context named by
// DOCKER_CONTEXT, then the current context in the configuration file. An empty
// result is the default context, which is the daemon's socket on this machine.
func dockerEndpoint() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return host
	}
	dir := dockerConfigDir()
	if dir == "" {
		return ""
	}
	name := os.Getenv("DOCKER_CONTEXT")
	if name == "" {
		b, err := os.ReadFile(filepath.Join(dir, "config.json"))
		if err != nil {
			return ""
		}
		var config struct {
			CurrentContext string `json:"currentContext"`
		}
		if json.Unmarshal(b, &config) != nil {
			return ""
		}
		name = config.CurrentContext
	}
	if name == "" || name == "default" {
		return ""
	}
	return contextEndpoint(dir, name)
}

// dockerConfigDir returns the directory the docker command reads its
// configuration and contexts from.
func dockerConfigDir() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker")
}

// contextEndpoint returns the daemon address a named context holds. The docker
// command stores a context under the SHA-256 of its name. A context that cannot
// be read returns no address, because the docker command fails on it as well
// and no container is listed from it.
func contextEndpoint(dir, name string) string {
	sum := sha256.Sum256([]byte(name))
	b, err := os.ReadFile(filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]), "meta.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	if json.Unmarshal(b, &meta) != nil {
		return ""
	}
	return meta.Endpoints["docker"].Host
}

// endpointOnThisMachine reports whether a daemon address is on this machine: the
// default, a Unix socket, a Windows named pipe, or a network address whose host
// is a loopback address, "localhost" or an address of one of this machine's
// interfaces. Every other address reaches a daemon on another machine. The
// scheme does not decide it: tcp://127.0.0.1:2375 is this machine's daemon and
// ssh://root@192.168.2.3 is another machine's.
func endpointOnThisMachine(host string) bool {
	if host == "" || strings.HasPrefix(host, "unix://") || strings.HasPrefix(host, "npipe://") {
		return true
	}
	u, err := url.Parse(host)
	if err != nil || u.Hostname() == "" {
		return false
	}
	return hostOnThisMachine(u.Hostname(), localAddresses())
}

// hostOnThisMachine reports whether a host name or address names this machine,
// given the addresses of its interfaces. A name other than "localhost" is not
// resolved: resolving it would make the answer depend on the network.
func hostOnThisMachine(name string, local []net.IP) bool {
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, l := range local {
		if l.Equal(ip) {
			return true
		}
	}
	return false
}

// localAddresses returns the addresses of this machine's interfaces.
func localAddresses() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			out = append(out, n.IP)
		}
	}
	return out
}
