package stack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Labels carry a container's routing settings. The proxy configuration and the
// DNS answers are generated from them, so removing a container removes its
// routes without editing a file. LabelDomain holds the service's domains
// separated by commas; it is empty for an internal service.
const (
	LabelRole    = "bay.role"
	LabelGroup   = "bay.group"
	LabelService = "bay.service"
	LabelDomain  = "bay.domain"
	LabelPort    = "bay.port"
	LabelScheme  = "bay.scheme"
	LabelConfig  = "bay.config"
	// LabelStopGrace holds the service's stop_grace_period in whole seconds. It
	// is set only when the Compose file sets the key.
	LabelStopGrace = "bay.grace"
)

const (
	roleService = "service"
	roleProxy   = "proxy"
)

type Instance struct {
	Name        string
	State       string
	IPv4        string
	IPv6        string
	Gateway     string
	Labels      map[string]string
	Created     string
	Started     string
	ImageDigest string
	// Engine is the engine that reported this container.
	Engine string
}

// List returns the containers of every engine present in one list. An engine
// that is absent or does not respond returns no containers.
func List() ([]Instance, error) {
	return listFrom(engineReaders())
}

func decodeInstances(out []byte) ([]Instance, error) {
	var raw []struct {
		Configuration struct {
			ID           string            `json:"id"`
			Labels       map[string]string `json:"labels"`
			CreationDate string            `json:"creationDate"`
			Image        struct {
				Descriptor struct {
					Digest string `json:"digest"`
				} `json:"descriptor"`
			} `json:"image"`
		} `json:"configuration"`
		Status struct {
			State       string `json:"state"`
			StartedDate string `json:"startedDate"`
			Networks    []struct {
				IPv4Address string `json:"ipv4Address"`
				IPv6Address string `json:"ipv6Address"`
				IPv4Gateway string `json:"ipv4Gateway"`
			} `json:"networks"`
		} `json:"status"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing container list: %w", err)
	}
	list := make([]Instance, 0, len(raw))
	for _, c := range raw {
		in := Instance{
			Name:        c.Configuration.ID,
			State:       c.Status.State,
			Labels:      c.Configuration.Labels,
			Created:     c.Configuration.CreationDate,
			Started:     c.Status.StartedDate,
			ImageDigest: c.Configuration.Image.Descriptor.Digest,
			Engine:      AppleEngine,
		}
		if len(c.Status.Networks) > 0 {
			// ipv4Address carries a prefix length, e.g. "192.168.64.61/24".
			in.IPv4, _, _ = strings.Cut(c.Status.Networks[0].IPv4Address, "/")
			in.IPv6, _, _ = strings.Cut(c.Status.Networks[0].IPv6Address, "/")
			in.Gateway = c.Status.Networks[0].IPv4Gateway
		}
		list = append(list, in)
	}
	return list, nil
}

// engineOf returns the engine that reported a container. A command sent to the
// wrong engine changes nothing and returns success.
func engineOf(name string) (string, error) {
	in, found, err := Lookup(name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s: no such container", name)
	}
	return in.Engine, nil
}

func Lookup(name string) (Instance, bool, error) {
	list, err := List()
	if err != nil {
		return Instance{}, false, err
	}
	for _, in := range list {
		if in.Name == name {
			return in, true, nil
		}
	}
	return Instance{}, false, nil
}

// stopArguments returns the arguments that stop one container. A grace period
// is passed as --time in whole seconds; nil leaves the runtime default.
func stopArguments(name string, grace *int) []string {
	if grace == nil {
		return []string{"stop", name}
	}
	return []string{"stop", "--time", strconv.Itoa(*grace), name}
}

// graceOf reads the grace period recorded on a container. A container without
// the label has none; a label that is not a whole number is an error.
func graceOf(in Instance) (*int, error) {
	value, ok := in.Labels[LabelStopGrace]
	if !ok {
		return nil, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return nil, fmt.Errorf("%s: invalid %s label %q", in.Name, LabelStopGrace, value)
	}
	return &seconds, nil
}

// Remove deletes a container, ignoring the case where it does not exist.
func Remove(name string) error {
	engine, err := engineOf(name)
	if err != nil {
		return nil
	}
	if _, err := runEngine(engine, "rm", "--force", name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}
	return nil
}

// StartService creates and starts one service container. The container is named
// <project>-<service>, so two projects can use the same service names.
func StartService(group string, s *Service) error {
	return newServiceEngine("").reconcile(group, s, false)
}

func serviceArguments(group string, s *Service, fingerprint string) []string {
	scheme := "http"
	if s.TLS {
		scheme = "https"
	}
	// An internal service is started with an empty domain label, so the proxy
	// and the DNS server do not route to it.
	args := []string{"create", "--name", s.ContainerName,
		"--network", serviceNetworkOn(ServiceEngine(), s.Network),
		"--label", LabelRole + "=" + roleService,
		"--label", LabelGroup + "=" + group,
		"--label", LabelService + "=" + s.Name,
		"--label", LabelDomain + "=" + strings.Join(s.Domains, ","),
		"--label", LabelPort + "=" + strconv.Itoa(s.Port),
		"--label", LabelScheme + "=" + scheme,
		"--label", LabelConfig + "=" + fingerprint,
	}
	if s.StopGrace != nil {
		args = append(args, "--label", LabelStopGrace+"="+strconv.Itoa(*s.StopGrace))
	}
	if s.User != "" {
		args = append(args, "--user", s.User)
	}
	if s.ReadOnly {
		args = append(args, "--read-only")
	}
	for _, capability := range s.CapDrop {
		args = append(args, "--cap-drop", capability)
	}
	for k, v := range s.Env {
		args = append(args, "--env", k+"="+v)
	}
	for _, v := range s.Volumes {
		args = append(args, "--volume", v)
	}
	if s.CPUs > 0 {
		args = append(args, "--cpus", strconv.Itoa(s.CPUs))
	}
	if s.Memory != "" {
		args = append(args, "--memory", s.Memory)
	}
	args = append(args, s.Image)
	args = append(args, s.Command...)
	return args
}

// EnsureProxy makes sure the machine's single TLS-terminating nginx is running
// with the rendered config and the issued certificates bind-mounted read-only.
// It reports whether it had to create the container, which is the caller's cue
// that a reload is unnecessary.
//
// A proxy running against a different state directory is replaced, because it
// serves that directory's configuration and ignores this one.
func EnsureProxy(engine, confDir, certDir, peerDir string) (created bool, err error) {
	in, found, err := lookupOn(engine, ProxyName)
	if err != nil {
		return false, err
	}
	if found && in.State == "running" {
		conf, certs, peers, err := ProxyMounts(engine)
		if err != nil {
			return false, err
		}
		// The peer link is a published port and a mount, neither of which can
		// be added to a container that is already running.
		if sameDir(conf, confDir) && sameDir(certs, certDir) && sameDir(peers, peerDir) {
			return false, nil
		}
	}
	if found {
		if _, err := runEngine(engine, "rm", "--force", ProxyName); err != nil {
			return false, err
		}
	}
	if err := ensureNetwork(engine); err != nil {
		return false, err
	}
	_, err = runEngine(engine, append(proxyRunArgs(engine, confDir, certDir, peerDir), ProxyImage)...)
	return err == nil, err
}

// proxyRunArgs returns the arguments used to create one engine's proxy.
//
// The host has a route to an Apple container, so the Apple proxy publishes no
// port. The host has no route to a Docker container, so the Docker proxy
// publishes 80 and 443. It publishes them on the loopback address, not on every
// interface, because the peer link is the only port published to the network.
// The address is 127.0.0.1 because macOS assigns no other address to lo0 and
// adding one requires root.
func proxyRunArgs(engine, confDir, certDir, peerDir string) []string {
	args := []string{"run", "--detach", "--name", ProxyName,
		"--network", proxyNetwork(engine),
		"--label", LabelRole + "=" + roleProxy,
		"--volume", confDir + ":/etc/nginx/conf.d:ro",
		"--volume", certDir + ":/etc/nginx/certs:ro",
	}
	if engine == DockerEngine {
		for _, port := range []int{80, 443} {
			p := strconv.Itoa(port)
			args = append(args, "--publish", DockerProxyAddr+":"+p+":"+p)
		}
	}
	if peerDir != "" {
		port := strconv.Itoa(PeerPort)
		args = append(args,
			"--volume", peerDir+":/etc/nginx/peers:ro",
			"--publish", port+":"+port)
	}
	return args
}

// lookupOn returns a container from one engine. Each engine's proxy uses the
// same name, so the engine is required to select one.
func lookupOn(engine, name string) (Instance, bool, error) {
	list, err := List()
	if err != nil {
		return Instance{}, false, err
	}
	for _, in := range list {
		if in.Name == name && in.Engine == engine {
			return in, true, nil
		}
	}
	return Instance{}, false, nil
}

// proxyNetwork returns the network the proxy joins. Docker has no network named
// "default" and resolves a container name only on a user-defined network.
func proxyNetwork(engine string) string {
	if engine == DockerEngine {
		return DockerNetwork
	}
	return ProxyNetwork
}

// StartContainer starts an existing stopped container.
func StartContainer(name string) error {
	engine, err := engineOf(name)
	if err != nil {
		return err
	}
	_, err = runEngine(engine, "start", name)
	return err
}

// StopContainer stops a running container and leaves it in place.
func StopContainer(name string) error {
	in, found, err := Lookup(name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s: no such container", name)
	}
	grace, err := graceOf(in)
	if err != nil {
		return err
	}
	_, err = runEngine(in.Engine, stopArguments(name, grace)...)
	return err
}

// Logs writes a container's output to w. With follow set it returns when the
// stream ends or the container stops.
func Logs(name string, follow bool, tail int, w io.Writer) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "--follow")
	}
	if tail > 0 {
		args = append(args, "-n", strconv.Itoa(tail))
	}
	args = append(args, name)

	engine, err := engineOf(name)
	if err != nil {
		return err
	}
	// Following an output has no end of its own, so only a fixed read is bound.
	ctx := context.Background()
	if !follow {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, engineReadWait)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, engineBin(engine), args...)
	cmd.WaitDelay = time.Second
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// ProxyMounts returns the configuration and certificate directories the running
// proxy was started with.
func ProxyMounts(engine string) (confDir, certDir, peerDir string, err error) {
	out, err := runEngine(engine, "inspect", ProxyName)
	if err != nil {
		return "", "", "", err
	}
	mounts, err := decodeMounts(engine, out)
	if err != nil {
		return "", "", "", err
	}
	for _, m := range mounts {
		switch m.Destination {
		case "/etc/nginx/conf.d":
			confDir = m.Source
		case "/etc/nginx/certs":
			certDir = m.Source
		case "/etc/nginx/peers":
			peerDir = m.Source
		}
	}
	return confDir, certDir, peerDir, nil
}

// mount is one bind mount, parsed from either engine's output format.
type mount struct{ Source, Destination string }

func decodeMounts(engine string, out []byte) ([]mount, error) {
	if engine == DockerEngine {
		var raw []struct {
			Mounts []mount `json:"Mounts"`
		}
		if err := json.Unmarshal(out, &raw); err != nil || len(raw) == 0 {
			return nil, fmt.Errorf("reading the proxy's mounts: %w", err)
		}
		return raw[0].Mounts, nil
	}
	var raw []struct {
		Configuration struct {
			Mounts []struct {
				Source      string `json:"source"`
				Destination string `json:"destination"`
			} `json:"mounts"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(out, &raw); err != nil || len(raw) == 0 {
		return nil, fmt.Errorf("reading the proxy's mounts: %w", err)
	}
	list := make([]mount, 0, len(raw[0].Configuration.Mounts))
	for _, m := range raw[0].Configuration.Mounts {
		list = append(list, mount{Source: m.Source, Destination: m.Destination})
	}
	return list, nil
}

// sameDir compares two paths after resolving symlinks. On macOS /tmp and
// /private/tmp name the same directory.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, erra := filepath.EvalSymlinks(a)
	rb, errb := filepath.EvalSymlinks(b)
	return erra == nil && errb == nil && ra == rb
}

// StopProxy removes one engine's proxy. Callers call it when that engine has no
// route.
func StopProxy(engine string) error {
	_, err := runEngine(engine, "rm", "--force", ProxyName)
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

// ReloadProxy signals the running nginx to re-read its configuration and
// returns a failed command's original error without restarting the proxy.
func ReloadProxy(engine string) error {
	_, err := runEngine(engine, "exec", ProxyName, "nginx", "-s", "reload")
	return err
}

// serviceNetworkOn returns the network a service joins. Docker has no network
// named "default", so a service that names no network joins the network this
// program creates.
func serviceNetworkOn(engine, named string) string {
	if engine == DockerEngine && named == ProxyNetwork {
		return DockerNetwork
	}
	return named
}
