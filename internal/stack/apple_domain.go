package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Apple `container` names every container <name>.<domain> in its own DNS, and
// the domain is a setting of the machine: dns.domain. The proxy and the other
// services reach a service by that name, and the host resolves it through
// /etc/resolver/containerization.<domain>, which `container system dns create`
// writes. bay sets the domain to BackendDomain, so a service is
// <project>-<service>.bay.test on Apple `container` as on Docker, where the
// name is given as a network alias.

// appleCLI is the Apple `container` command. Tests replace it.
var appleCLI = func() (string, bool) {
	p, err := exec.LookPath("container")
	return p, err == nil
}

// AppleDomainResolver is the resolver entry `container system dns create`
// writes for a domain.
func AppleDomainResolver(domain string) string {
	return filepath.Join(resolverDir, "containerization."+domain)
}

// appleDomainSetting returns the dns.domain setting of Apple `container`.
func appleDomainSetting(cli string) string {
	out, err := exec.Command(cli, "system", "property", "list", "--format", "json").Output()
	if err != nil {
		return ""
	}
	var props struct {
		DNS struct {
			Domain string `json:"domain"`
		} `json:"dns"`
	}
	if json.Unmarshal(out, &props) != nil {
		return ""
	}
	return props.DNS.Domain
}

// appleConfigPath is the user's Apple `container` settings file. Version 1.3
// has no command that changes a setting, so the one key is written there.
// Tests replace it.
var appleConfigPath = func() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "container", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "container", "config.toml"), nil
}

// withAppleDomain returns the settings with domain under [dns], every other
// line kept as it is.
func withAppleDomain(current, domain string) string {
	line := fmt.Sprintf("domain = %q", domain)
	lines := strings.Split(strings.TrimRight(current, "\n"), "\n")
	if current == "" {
		lines = nil
	}
	section, inDNS, dnsAt := "", false, -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			section = t
			inDNS = section == "[dns]"
			if inDNS {
				dnsAt = i
			}
			continue
		}
		if inDNS {
			if k, _, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == "domain" {
				lines[i] = line
				return strings.Join(lines, "\n") + "\n"
			}
		}
	}
	if dnsAt >= 0 {
		lines = append(lines[:dnsAt+1], append([]string{line}, lines[dnsAt+1:]...)...)
		return strings.Join(lines, "\n") + "\n"
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, "[dns]", line)
	return strings.Join(lines, "\n") + "\n"
}

// appleDomainItems are the domain's parts: the resolver entry, which needs
// root; the setting, which belongs to the user; and the running system, which
// reads the setting when it starts.
func appleDomainItems(domain string) []SetupItem {
	cli, ok := appleCLI()
	if domain == "" || !ok {
		return nil
	}
	_, err := os.Stat(AppleDomainResolver(domain))
	items := []SetupItem{
		{
			Name: "Apple container domain " + AppleDomainResolver(domain), OK: err == nil,
			SetupStep: SetupStep{Text: "create the Apple container DNS domain " + domain, Asks: AsksAdministrator},
		},
		{
			Name: "Apple container setting dns.domain = " + domain, OK: appleDomainWritten(domain),
			SetupStep: SetupStep{Text: "set dns.domain to " + domain + " in the Apple container settings"},
		},
	}
	running := appleDomainSetting(cli) == domain
	item := SetupItem{Name: "Apple container names containers <name>." + domain, OK: running}
	if !running {
		if names := runningContainers(cli); len(names) > 0 {
			item.Name += ": the system reads the setting when it starts, and " + strconv.Itoa(len(names)) +
				" container(s) run; stop them or run \"container system stop; container system start\", then \"bay up\" each project"
		} else {
			item.SetupStep = SetupStep{Text: "restart the Apple container system, which runs no container, so it reads the setting"}
		}
	}
	return append(items, item)
}

// appleDomainWritten reports whether the settings file names the domain.
func appleDomainWritten(domain string) bool {
	path, err := appleConfigPath()
	if err != nil {
		return false
	}
	b, err := os.ReadFile(path)
	return err == nil && withAppleDomain(string(b), domain) == string(b)
}

// runningContainers lists the running Apple containers.
func runningContainers(cli string) []string {
	out, err := exec.Command(cli, "ls", "--format", "json").Output()
	if err != nil {
		return nil
	}
	var list []struct {
		Configuration struct {
			ID string `json:"id"`
		} `json:"configuration"`
	}
	if json.Unmarshal(out, &list) != nil {
		return nil
	}
	var names []string
	for _, c := range list {
		names = append(names, c.Configuration.ID)
	}
	return names
}

// restartAppleSystem restarts the system so it reads the setting, only when
// no container runs: a restart stops every container.
func restartAppleSystem(domain string) error {
	cli, ok := appleCLI()
	if domain == "" || !ok || appleDomainSetting(cli) == domain || len(runningContainers(cli)) > 0 {
		return nil
	}
	for _, args := range [][]string{{"system", "stop"}, {"system", "start"}} {
		if out, err := exec.Command(cli, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("container %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	fmt.Printf("restarted the Apple container system; it names containers <name>.%s\n", domain)
	return nil
}

// appleDomainRootPending reports whether the resolver entry is missing.
func appleDomainRootPending(domain string) bool {
	if _, ok := appleCLI(); domain == "" || !ok {
		return false
	}
	_, err := os.Stat(AppleDomainResolver(domain))
	return err != nil
}

// createAppleDomain writes the resolver entry. It runs as root.
func createAppleDomain(domain string) error {
	cli, ok := appleCLI()
	if !ok {
		return fmt.Errorf("the container command is not installed")
	}
	if out, err := exec.Command(cli, "system", "dns", "create", domain).CombinedOutput(); err != nil {
		return fmt.Errorf("container system dns create %s: %v: %s", domain, err, out)
	}
	fmt.Printf("created the Apple container DNS domain %s\n", domain)
	return nil
}

// setAppleDomain sets the user's dns.domain when it differs. A container
// created after it is named in the domain; one created before keeps its name
// until it is created again.
func setAppleDomain(domain string) error {
	if _, ok := appleCLI(); domain == "" || !ok || appleDomainWritten(domain) {
		return nil
	}
	path, err := appleConfigPath()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(withAppleDomain(string(current), domain)), 0o644); err != nil {
		return err
	}
	fmt.Printf("set the Apple container dns.domain to %s in %s\n", domain, path)
	return nil
}
