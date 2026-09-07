package stack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Labels carry a container's routing settings. The proxy configuration and the
// DNS answers are generated from them, so removing a container removes its
// route without editing a file.
const (
	LabelRole    = "bay.role"
	LabelGroup   = "bay.group"
	LabelService = "bay.service"
	LabelDomain  = "bay.domain"
	LabelPort    = "bay.port"
	LabelScheme  = "bay.scheme"
	LabelConfig  = "bay.config"
)

const (
	roleService = "service"
	roleProxy   = "proxy"
)

type Instance struct {
	Name        string
	State       string
	IPv4        string
	Gateway     string
	Labels      map[string]string
	Created     string
	Started     string
	ImageDigest string
}

// List returns every container the runtime knows about.
func List() ([]Instance, error) {
	out, err := run("ls", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return decodeInstances(out)
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
		}
		if len(c.Status.Networks) > 0 {
			// ipv4Address carries a prefix length, e.g. "192.168.64.61/24".
			in.IPv4, _, _ = strings.Cut(c.Status.Networks[0].IPv4Address, "/")
			in.Gateway = c.Status.Networks[0].IPv4Gateway
		}
		list = append(list, in)
	}
	return list, nil
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

// Remove deletes a container, ignoring the case where it does not exist.
func Remove(name string) error {
	if _, err := run("rm", "--force", name); err != nil {
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
	args := []string{"run", "--name", s.ContainerName,
		"--network", s.Network,
		"--label", LabelRole + "=" + roleService,
		"--label", LabelGroup + "=" + group,
		"--label", LabelService + "=" + s.Name,
		"--label", LabelDomain + "=" + s.Domain,
		"--label", LabelPort + "=" + strconv.Itoa(s.Port),
		"--label", LabelScheme + "=" + scheme,
		"--label", LabelConfig + "=" + fingerprint,
	}
	if !s.OneShot {
		args = append(args, "--detach")
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
func EnsureProxy(confDir, certDir string) (created bool, err error) {
	in, found, err := Lookup(ProxyName)
	if err != nil {
		return false, err
	}
	if found && in.State == "running" {
		conf, certs, err := ProxyMounts()
		if err != nil {
			return false, err
		}
		if sameDir(conf, confDir) && sameDir(certs, certDir) {
			return false, nil
		}
	}
	if found {
		if err := Remove(ProxyName); err != nil {
			return false, err
		}
	}
	_, err = run("run", "--detach", "--name", ProxyName,
		"--network", ProxyNetwork,
		"--label", LabelRole+"="+roleProxy,
		"--volume", confDir+":/etc/nginx/conf.d:ro",
		"--volume", certDir+":/etc/nginx/certs:ro",
		ProxyImage)
	return err == nil, err
}

// StartContainer starts an existing stopped container.
func StartContainer(name string) error {
	_, err := run("start", name)
	return err
}

// StopContainer stops a running container and leaves it in place.
func StopContainer(name string) error {
	_, err := run("stop", name)
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

	cmd := exec.Command(containerBin(), args...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// ProxyMounts returns the configuration and certificate directories the running
// proxy was started with.
func ProxyMounts() (confDir, certDir string, err error) {
	out, err := run("inspect", ProxyName)
	if err != nil {
		return "", "", err
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
		return "", "", fmt.Errorf("reading the proxy's mounts: %w", err)
	}
	for _, m := range raw[0].Configuration.Mounts {
		switch m.Destination {
		case "/etc/nginx/conf.d":
			confDir = m.Source
		case "/etc/nginx/certs":
			certDir = m.Source
		}
	}
	return confDir, certDir, nil
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

// StopProxy removes the proxy. Callers remove it when no route remains.
func StopProxy() error { return Remove(ProxyName) }

// ReloadProxy signals the running nginx to re-read its configuration. It stops
// and starts the container when the signal fails.
func ReloadProxy() error {
	if _, err := run("exec", ProxyName, "nginx", "-s", "reload"); err != nil {
		// The runtime has no restart subcommand.
		if _, serr := run("stop", ProxyName); serr != nil {
			return err
		}
		if _, serr := run("start", ProxyName); serr != nil {
			return err
		}
	}
	return nil
}

func run(args ...string) ([]byte, error) {
	cmd := exec.Command(containerBin(), args...)
	out, err := cmd.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		msg := strings.TrimSpace(string(ee.Stderr))
		if msg == "" {
			msg = ee.String()
		}
		return nil, fmt.Errorf("container %s: %s", strings.Join(args, " "), msg)
	}
	return out, err
}

func containerBin() string {
	if bin := os.Getenv("CONTAINER_BIN"); bin != "" {
		return bin
	}
	return "container"
}
