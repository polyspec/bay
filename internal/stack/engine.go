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

// The engines that run containers. Neither is preferred. An engine is named by
// the command that speaks to it rather than by the product that installed it,
// so Docker Desktop, OrbStack and Colima are one engine here.
const (
	AppleEngine  = "container"
	DockerEngine = "docker"
)

// An engineReader names one engine and reads the containers it holds.
type engineReader struct {
	name string
	read func() ([]Instance, error)
}

// engineReaders returns a reader for every engine whose command is on PATH. A
// command that is absent is left out here; a command that is present but whose
// socket does not answer fails at read time, and listFrom treats both the same.
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

// listFrom merges the containers of every reader into one list. A reader that
// fails contributes nothing and is not an error: a machine with one engine
// behaves as it did before the other was supported. With no reader at all there
// is nothing to run containers with, which is reported.
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

// engineBin returns the command that speaks to an engine, or an empty string
// when it is not on PATH. The environment overrides the name so a test can put
// its own program in its place.
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

// runEngine runs one command against an engine and returns its output, with a
// failure carrying the command's own diagnostic.
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

// dockerContainers reads the identifiers first and inspects them, because
// `docker container ls` reports labels as one joined string and reports no
// address at all. Inspect answers both, keyed and per network.
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

// decodeDockerInstances reads `docker inspect`. Docker writes a container's name
// with a leading slash and keys its networks by name, so the name is trimmed and
// the networks are taken in name order: a container on several networks must
// report the same address every time it is read.
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
// name belonging to no engine is answered the same way every time.
func Engines() []string {
	var names []string
	for _, e := range []string{AppleEngine, DockerEngine} {
		if engineBin(e) != "" {
			names = append(names, e)
		}
	}
	return names
}

// ProxyHostAddr returns the address the host reaches one engine's proxy at. The
// host routes to an Apple container directly, so that proxy is reached at its
// own address. It routes to no Docker container, so that proxy is reached only
// at the loopback address it publishes on.
func ProxyHostAddr(engine string, in Instance) string {
	if engine == DockerEngine {
		return DockerProxyAddr
	}
	return in.IPv4
}

// ServiceEngine returns the engine a project's containers are created on. A
// container is reachable on its own engine's network only, so all of a
// project's services belong to one engine, and it is the first one present.
func ServiceEngine() string {
	if names := Engines(); len(names) > 0 {
		return names[0]
	}
	return AppleEngine
}

// ensureNetwork creates the network the proxy and the services share. Apple
// `container` has it already; Docker has no network called "default" and
// resolves a container name only on a user-defined network.
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
