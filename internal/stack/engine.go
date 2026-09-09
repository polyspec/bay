package stack

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// The engines that run containers. Each engine is named by the command used to
// control it.
const (
	AppleEngine  = "container"
	DockerEngine = "docker"
)

// engineReader reads the container list of one engine.
type engineReader struct {
	name string
	read func() ([]Instance, error)
}

// engineReaders returns one reader per engine whose command is on PATH. A
// missing command is excluded here. A present command whose daemon does not
// respond fails in listFrom.
func engineReaders() []engineReader {
	var readers []engineReader
	if bin := engineBin(AppleEngine); bin != "" {
		readers = append(readers, engineReader{name: AppleEngine, read: appleContainers})
	}
	if bin := engineBin(DockerEngine); bin != "" {
		readers = append(readers, engineReader{name: DockerEngine, read: dockerContainers})
	}
	return readers
}

// listFrom merges the container lists of every reader. A reader that fails
// returns no containers and does not cause an error. With no reader, listFrom
// returns an error.
func listFrom(readers []engineReader) ([]Instance, error) {
	if len(readers) == 0 {
		return nil, fmt.Errorf("no container engine found: install Apple %s or %s",
			AppleEngine, DockerEngine)
	}
	list := make([]Instance, 0, len(readers))
	for _, r := range readers {
		found, err := r.read()
		if err != nil {
			continue
		}
		list = append(list, found...)
	}
	return list, nil
}

// engineBin returns the command used to control an engine, or an empty string
// when the command is not on PATH. An environment variable overrides the name so
// that a test can substitute its own program.
func engineBin(engine string) string {
	env := "CONTAINER_BIN"
	if engine == DockerEngine {
		env = "DOCKER_BIN"
	}
	if bin := os.Getenv(env); bin != "" {
		return bin
	}
	if path, err := exec.LookPath(engine); err == nil {
		return path
	}
	return ""
}

// runEngine runs one command against an engine and returns its output. On
// failure the error contains the command's stderr.
func runEngine(engine string, args ...string) ([]byte, error) {
	bin := engineBin(engine)
	if bin == "" {
		return nil, fmt.Errorf("%s is not installed", engine)
	}
	out, err := exec.Command(bin, args...).Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		msg := strings.TrimSpace(string(ee.Stderr))
		if msg == "" {
			msg = ee.String()
		}
		return nil, fmt.Errorf("%s %s: %s", engine, strings.Join(args, " "), msg)
	}
	return out, err
}

func appleContainers() ([]Instance, error) {
	out, err := runEngine(AppleEngine, "ls", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return decodeInstances(out)
}

// dockerContainers reads the container identifiers and then inspects them.
// `docker container ls` returns labels as one joined string and returns no
// address, so inspect is required.
func dockerContainers() ([]Instance, error) {
	out, err := runEngine(DockerEngine, "container", "ls", "--all", "--quiet", "--no-trunc")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}
	out, err = runEngine(DockerEngine, append([]string{"inspect", "--type", "container"}, ids...)...)
	if err != nil {
		return nil, err
	}
	return decodeDockerInstances(out)
}

// decodeDockerInstances parses `docker inspect` output. Docker returns the
// container name with a leading slash, which is trimmed, and returns networks as
// a map, which is read in name order so that a container on several networks
// returns the same address on every call.
func decodeDockerInstances(out []byte) ([]Instance, error) {
	var raw []struct {
		ID      string `json:"Id"`
		Name    string `json:"Name"`
		Created string `json:"Created"`
		Image   string `json:"Image"`
		State   struct {
			Status    string `json:"Status"`
			StartedAt string `json:"StartedAt"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
				Gateway   string `json:"Gateway"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing docker container list: %w", err)
	}
	list := make([]Instance, 0, len(raw))
	for _, c := range raw {
		in := Instance{
			Name:        strings.TrimPrefix(c.Name, "/"),
			State:       c.State.Status,
			Labels:      c.Config.Labels,
			Created:     c.Created,
			Started:     c.State.StartedAt,
			ImageDigest: c.Image,
			Engine:      DockerEngine,
		}
		names := make([]string, 0, len(c.NetworkSettings.Networks))
		for name := range c.NetworkSettings.Networks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if net := c.NetworkSettings.Networks[name]; net.IPAddress != "" {
				in.IPv4, in.Gateway = net.IPAddress, net.Gateway
				break
			}
		}
		list = append(list, in)
	}
	return list, nil
}

// Engines returns the name of every engine present, in a fixed order so that a
// domain belonging to no engine resolves to the same proxy on every call.
func Engines() []string {
	var names []string
	for _, e := range []string{AppleEngine, DockerEngine} {
		if engineBin(e) != "" {
			names = append(names, e)
		}
	}
	return names
}

// ProxyHostAddr returns the address the host uses to reach one engine's proxy.
// The host has a route to an Apple container, so its address is used. The host
// has no route to a Docker container, so the published loopback address is used.
func ProxyHostAddr(engine string, in Instance) string {
	if engine == DockerEngine {
		return DockerProxyAddr
	}
	return in.IPv4
}

// ServiceEngine returns the engine used to create a project's containers. A
// container is reachable only on its own engine's network, so all services of
// one project are created on one engine, the first engine present.
func ServiceEngine() string {
	if names := Engines(); len(names) > 0 {
		return names[0]
	}
	return AppleEngine
}

// ensureNetwork creates the network shared by the proxy and the services. Apple
// `container` provides it. Docker has no network named "default" and resolves a
// container name only on a user-defined network.
func ensureNetwork(engine string) error {
	if engine != DockerEngine {
		return nil
	}
	if _, err := runEngine(engine, "network", "inspect", DockerNetwork); err == nil {
		return nil
	}
	_, err := runEngine(engine, "network", "create", DockerNetwork)
	return err
}

// engineResolver returns the DNS address the proxy uses to resolve a container
// name. Apple `container` answers on the network gateway. Docker answers on its
// own resolver address and does not answer on the gateway.
func engineResolver(engine, gateway string) string {
	if engine == DockerEngine {
		return DockerResolver
	}
	return gateway
}
