package stack

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// TunnelImage is the tunnel client a publication runs. The version is pinned:
// a publication must not change behaviour between runs.
const TunnelImage = "cloudflare/cloudflared:2026.11.1"

// TunnelService is the publication's service name inside the project. The
// container is <project>-tunnel, labelled as an internal service, so the
// project's own commands take it down with the rest and the next up removes it
// as a container of a service the Compose file does not declare.
const TunnelService = "tunnel"

// tunnelAddressShape matches the public address the tunnel client names in its
// output. The client prints it once, inside a box of its own drawing.
var tunnelAddressShape = regexp.MustCompile(`https://[-0-9a-z]+\.trycloudflare\.com`)

// tunnelAddressWait bounds how long the command waits for the client to name
// its public address. After the address is named the command follows the
// publication with no bound, as logs -f does.
const tunnelAddressWait = time.Minute

// tunnelOutputKeep bounds how much of the client's output is kept for the
// report of a client that ended without an address.
const tunnelOutputKeep = 8192

// Tunnel publishes one served domain of the project at a public address and
// prints the address to out. The publication runs until the command ends or
// the container is removed by anything else.
func (r *Runtime) Tunnel(cfg *Config, domain string, out io.Writer) error {
	routes, conflicts, err := Routes()
	if err != nil {
		return err
	}
	route, service, err := tunnelRoute(cfg, domain, routes, conflicts)
	if err != nil {
		return err
	}
	engine := route.Engine
	backendHost, _, _ := strings.Cut(route.Backend, ":") // the port is in the URL below
	name := cfg.Name + "-" + TunnelService

	if err := r.clearOldPublication(name); err != nil {
		return err
	}
	if err := r.ensureTunnelImage(engine); err != nil {
		return err
	}

	r.say("publishing %s at a public address", domain)
	caPath := filepath.Join(r.Machine.Dir, "ca.crt")
	if _, err := os.Stat(caPath); err != nil {
		return fmt.Errorf("the machine's authority is missing: %s", caPath)
	}
	args := []string{"create", "--name", name,
		"--network", serviceNetworkOn(engine, service.Network),
		"--label", LabelRole + "=" + roleService,
		"--label", LabelGroup + "=" + cfg.Name,
		"--label", LabelService + "=" + TunnelService,
		// An empty domain label keeps the publication off the routes, as an
		// internal service's is.
		"--label", LabelDomain + "=",
		"--volume", caPath + ":/etc/bay/tunnel-ca.crt:ro",
		TunnelImage,
		"tunnel", "--no-autoupdate",
		"--url", route.Scheme + "://" + route.Backend,
	}
	if route.Scheme == "https" {
		// The service serves its own TLS: the client verifies it against this
		// machine's authority, at the backend's own name.
		args = append(args, "--origin-ca-pool", "/etc/bay/tunnel-ca.crt",
			"--origin-server-name", backendHost)
	}
	if _, err := runEngine(engine, args...); err != nil {
		return err
	}
	r.say("%s: starting the tunnel client", name)
	if _, err := runEngine(engine, "start", name); err != nil {
		r.endPublication(engine, name)
		return err
	}
	return r.followPublication(engine, name, out)
}

// tunnelRoute returns the route a publication forwards to and the service of
// the project it belongs to. The domain must be one the proxy serves for a
// running container of this project: no route, a route another project holds
// and a domain lost to a conflict are all refused with the domain named.
func tunnelRoute(cfg *Config, domain string, routes []Route, conflicts []DomainConflict) (Route, *Service, error) {
	for _, route := range routes {
		if route.Domain != domain {
			continue
		}
		backend, _, _ := strings.Cut(route.Backend, ":")
		for _, s := range cfg.Services {
			if s.ContainerName+"."+BackendDomain == backend {
				return route, s, nil
			}
		}
		return Route{}, nil, fmt.Errorf("%s is served by %s, which is not a container of this project", domain, backend)
	}
	for _, c := range conflicts {
		if c.Domain == domain {
			return Route{}, nil, fmt.Errorf("%s is claimed by more than one container and is served by neither", domain)
		}
	}
	return Route{}, nil, fmt.Errorf("%s: the proxy serves no route for it; is its service running?", domain)
}

// clearOldPublication removes what an earlier publication left. A running one
// is refused: two clients would publish one domain twice. A stopped one is the
// remainder of a run that was killed, and is removed.
func (r *Runtime) clearOldPublication(name string) error {
	in, found, err := Lookup(name)
	if err != nil || !found {
		return err
	}
	if in.Labels[LabelRole] != roleService {
		return fmt.Errorf("container %s exists and is not a service container; a publication cannot use its name", name)
	}
	if in.State == "running" {
		return fmt.Errorf("container %s is running; end the publication it serves first", name)
	}
	_, err = runEngine(in.Engine, "rm", "--force", name)
	return err
}

// ensureTunnelImage fetches the tunnel client's image when the store lacks it,
// for the platform this machine runs. -no-pull refuses the fetch.
func (r *Runtime) ensureTunnelImage(engine string) error {
	if _, err := runEngineWithin(engineReadWait, engine, "image", "inspect", TunnelImage); err == nil {
		return nil
	} else if !imageMissing(err) {
		return err
	}
	if r.NoPull {
		return fmt.Errorf("%s is not in the local store and -no-pull fetches nothing; run \"bay pull\" first", TunnelImage)
	}
	pull := []string{"image", "pull", TunnelImage}
	if engine != DockerEngine {
		// The engine fetches every platform of an image index without one.
		pull = []string{"image", "pull", "--platform", "linux/" + runtime.GOARCH, TunnelImage}
	}
	r.say("pulling %s", TunnelImage)
	_, err := runEngine(engine, pull...)
	return err
}

// followPublication reads the client's output until the publication ends. It
// prints the public address once the client names it, keeps the rest of the
// output for a report, and ends with the publication: a signal ends the
// command, and the client's own end ends it too. The container is stopped and
// removed on every path, so an ended publication leaves nothing behind.
func (r *Runtime) followPublication(engine, name string, out io.Writer) error {
	cmd := exec.Command(engineBin(engine), "logs", "--follow", name)
	cmd.WaitDelay = time.Second

	// The reader turns the stream into lines, keeps a bounded tail and hands
	// the public address to the loop below.
	reader, writer := io.Pipe()
	cmd.Stdout, cmd.Stderr = writer, writer
	var (
		kept      strings.Builder
		addressCh = make(chan string, 1)
	)
	if err := cmd.Start(); err != nil {
		return err
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if kept.Len() < tunnelOutputKeep {
				kept.WriteString(line + "\n")
			}
			if !strings.Contains(line, "https://") {
				continue
			}
			if address := tunnelAddressShape.FindString(line); address != "" {
				select {
				case addressCh <- address:
				default:
				}
			}
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	var address string
	bound := time.NewTimer(tunnelAddressWait)
	defer bound.Stop()
	for address == "" {
		select {
		case sig := <-signals:
			_ = cmd.Process.Signal(sig)
			<-waitCh
			r.endPublication(engine, name)
			return nil
		case address = <-addressCh:
			fmt.Fprintln(out, address)
		case <-bound.C:
			_ = cmd.Process.Signal(syscall.SIGTERM)
			<-waitCh
			r.endPublication(engine, name)
			return fmt.Errorf("%s named no public address within %s: %s",
				name, tunnelAddressWait, strings.TrimSpace(kept.String()))
		case <-waitCh:
			r.endPublication(engine, name)
			return fmt.Errorf("%s ended without an address: %s",
				name, strings.TrimSpace(kept.String()))
		}
	}
	bound.Stop()

	// The address is named: the publication runs until it is ended. A signal
	// ends the command; the client's own end ends it with a report.
	for {
		select {
		case sig := <-signals:
			_ = cmd.Process.Signal(sig)
			<-waitCh
			r.endPublication(engine, name)
			return nil
		case err := <-waitCh:
			r.endPublication(engine, name)
			if err != nil {
				return fmt.Errorf("%s ended: %v: %s", name, err, strings.TrimSpace(kept.String()))
			}
			fmt.Fprintf(out, "the tunnel client ended; the address is no longer served\n")
			return nil
		}
	}
}

// endPublication stops and removes the publication's container. Both steps are
// best effort: ending a publication that something else removed first must not
// fail the command.
func (r *Runtime) endPublication(engine, name string) {
	if in, found, err := Lookup(name); err == nil && found && in.State == "running" {
		if _, err := runEngine(engine, "stop", name); err != nil {
			r.say("warning: stopping %s: %v", name, err)
		}
	}
	if _, err := runEngine(engine, "rm", "--force", name); err != nil {
		r.say("warning: removing %s: %v", name, err)
	}
}
