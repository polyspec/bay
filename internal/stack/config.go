package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// This tool reads Compose files. Settings beyond the Compose schema are stored
// in the `x-bay` extension mapping and in `bay.*` service
// labels, so other Compose tools can read the same file.
const (
	LabelKeyDomain   = "bay.domain"
	LabelKeyPort     = "bay.port"
	LabelKeyInternal = "bay.internal"
	LabelKeyTLS      = "bay.tls"
)

// ComposeFileNames are looked for in a directory, in the order Compose itself
// uses.
var ComposeFileNames = []string{
	"compose.yaml", "compose.yml",
	"docker-compose.yaml", "docker-compose.yml",
}

// Config is one group: a Compose project, its domains and its services.
type Config struct {
	// Name is the Compose project name. It defaults to the file's directory,
	// and prefixes every container name so groups cannot collide.
	Name string
	// Domain is the local domain the project's services use. A Compose file
	// that does not set it inherits the machine's default domain.
	Domain string
	// DomainPinned reports that the Compose file named the domain. A pinned
	// domain is owned by the project; an inherited domain is owned by the
	// machine and is removed there.
	DomainPinned bool
	// ExtraDomains are further local domains this project's services may claim.
	ExtraDomains []string
	Services     map[string]*Service

	path string
}

type Service struct {
	Name string
	// ContainerName is <project>-<service> unless the file names it.
	ContainerName string
	Image         string
	Command       []string
	Env           map[string]string
	Volumes       []string
	Network       string
	CPUs          int
	Memory        string

	// Internal marks a service that receives no domain. The container runs and
	// other services reach it by name, but the proxy does not route to it and
	// issues no certificate for it.
	Internal bool
	// Domain the proxy routes to this service.
	Domain string
	// Port the service listens on inside the container.
	Port int
	// TLS marks a backend that already speaks HTTPS on Port.
	TLS bool
}

// composeFile holds the Compose keys this tool reads. Unknown keys are ignored
// so that a file written for other tools loads without change.
type composeFile struct {
	Name         string                     `yaml:"name"`
	Services     map[string]*composeService `yaml:"services"`
	Bay *projectSettings           `yaml:"x-bay"`
}

type projectSettings struct {
	Domain       string   `yaml:"domain"`
	ExtraDomains []string `yaml:"extra_domains"`
	Network      string   `yaml:"network"`
}

type composeService struct {
	Image         string       `yaml:"image"`
	Command       stringOrList `yaml:"command"`
	Entrypoint    stringOrList `yaml:"entrypoint"`
	Environment   mapOrList    `yaml:"environment"`
	Labels        mapOrList    `yaml:"labels"`
	Volumes       []string     `yaml:"volumes"`
	Ports         []string     `yaml:"ports"`
	Expose        []string     `yaml:"expose"`
	ContainerName string       `yaml:"container_name"`
	Networks      yaml.Node    `yaml:"networks"`
	CPUs          string       `yaml:"cpus"`
	MemLimit      string       `yaml:"mem_limit"`
}

// FindComposeFile returns the path when it names a file, or the first known
// Compose file name found in the directory.
func FindComposeFile(path string) (string, error) {
	if path == "" {
		path = "."
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() {
		return path, nil
	}
	for _, name := range ComposeFileNames {
		candidate := filepath.Join(path, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no Compose file in %s (looked for %s)",
		path, strings.Join(ComposeFileNames, ", "))
}

// Load reads a Compose file and applies the built-in default domain. Use
// LoadIn to apply a machine's default domain instead.
func Load(path string) (*Config, error) { return load(path, DefaultDomain) }

// LoadIn reads a Compose file, inheriting the machine's default domain when the
// file does not name one.
func LoadIn(m *Machine, path string) (*Config, error) {
	fallback := DefaultDomain
	if m != nil {
		if s, err := m.Settings(); err == nil {
			fallback = s.Domain
		}
	}
	return load(path, fallback)
}

func load(path, fallback string) (*Config, error) {
	path, err := FindComposeFile(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	var file composeFile
	if err := yaml.Unmarshal(b, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	cfg := &Config{Name: file.Name, path: abs}
	if file.Bay != nil {
		cfg.Domain = file.Bay.Domain
		cfg.DomainPinned = cfg.Domain != ""
		cfg.ExtraDomains = file.Bay.ExtraDomains
	}
	if cfg.Domain == "" {
		cfg.Domain = fallback
	}
	if err := cfg.build(&file); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return cfg, nil
}

func (c *Config) build(file *composeFile) error {
	if c.Name == "" {
		c.Name = filepath.Base(filepath.Dir(c.path))
	}
	c.Name = strings.ToLower(c.Name)
	if !validName(c.Name) {
		return fmt.Errorf("project name %q must be letters, digits, - or _", c.Name)
	}

	if c.Domain == "" {
		c.Domain = DefaultDomain
	}
	c.Domain = normalizeDomain(c.Domain)
	for i, d := range c.ExtraDomains {
		c.ExtraDomains[i] = normalizeDomain(d)
		if c.ExtraDomains[i] == "" {
			return fmt.Errorf("x-bay.extra_domains[%d] is empty", i)
		}
		if c.ExtraDomains[i] == c.Domain {
			return fmt.Errorf("x-bay.extra_domains[%d] repeats the domain %q", i, c.Domain)
		}
	}

	network := ProxyNetwork
	if file.Bay != nil && file.Bay.Network != "" {
		network = file.Bay.Network
	}
	if len(file.Services) == 0 {
		return fmt.Errorf("no services defined")
	}

	c.Services = make(map[string]*Service, len(file.Services))
	claimed := map[string]string{}
	for name, cs := range file.Services {
		s, err := c.service(name, cs, network)
		if err != nil {
			return err
		}
		if s.Domain != "" {
			if other, dup := claimed[s.Domain]; dup {
				return fmt.Errorf("services %q and %q both claim %s", other, name, s.Domain)
			}
			claimed[s.Domain] = name
		}
		c.Services[s.Name] = s
	}
	return nil
}

func (c *Config) service(name string, cs *composeService, network string) (*Service, error) {
	s := &Service{Name: strings.ToLower(name), Network: network}
	if !validName(s.Name) {
		return nil, fmt.Errorf("service name %q must be letters, digits, - or _", name)
	}
	if cs == nil || cs.Image == "" {
		return nil, fmt.Errorf("service %q: image is required", name)
	}
	s.Image = cs.Image
	s.ContainerName = cs.ContainerName
	if s.ContainerName == "" {
		s.ContainerName = c.Name + "-" + s.Name
	}
	s.Command = append(append([]string{}, cs.Entrypoint...), cs.Command...)
	s.Env = cs.Environment
	s.Volumes = cs.Volumes
	s.Memory = cs.MemLimit
	if cs.CPUs != "" {
		if n, err := strconv.ParseFloat(cs.CPUs, 64); err == nil && n >= 1 {
			s.CPUs = int(n)
		}
	}
	if n := serviceNetwork(cs.Networks); n != "" {
		s.Network = n
	}

	labels := cs.Labels
	s.Internal = isTrue(labels[LabelKeyInternal])
	s.TLS = isTrue(labels[LabelKeyTLS])
	s.Port = servicePort(labels[LabelKeyPort], cs)

	if s.Internal {
		if labels[LabelKeyDomain] != "" {
			return nil, fmt.Errorf("service %q is internal, so it cannot claim %s",
				name, labels[LabelKeyDomain])
		}
		return s, nil
	}
	s.Domain = normalizeDomain(labels[LabelKeyDomain])
	if s.Domain == "" {
		s.Domain = s.Name + "." + c.Domain
	}
	if !c.covers(s.Domain) {
		return nil, fmt.Errorf("service %q: domain %q is outside %s", name, s.Domain,
			"."+strings.Join(c.Domains(), ", ."))
	}
	return s, nil
}

// servicePort returns the port the container listens on. It reads the
// bay.port label, then `expose`, then the container side of `ports`,
// and returns 80 when none is set.
func servicePort(label string, cs *composeService) int {
	if n, err := strconv.Atoi(strings.TrimSpace(label)); err == nil && n > 0 {
		return n
	}
	for _, e := range cs.Expose {
		if n, err := strconv.Atoi(strings.TrimSpace(portNumber(e))); err == nil && n > 0 {
			return n
		}
	}
	for _, p := range cs.Ports {
		parts := strings.Split(p, ":")
		if n, err := strconv.Atoi(portNumber(parts[len(parts)-1])); err == nil && n > 0 {
			return n
		}
	}
	return 80
}

// portNumber strips a protocol suffix such as "5432/tcp".
func portNumber(s string) string {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i]
	}
	return s
}

// serviceNetwork returns the first network the service names. Compose accepts
// a sequence or a mapping.
func serviceNetwork(node yaml.Node) string {
	switch node.Kind {
	case yaml.SequenceNode:
		if len(node.Content) > 0 {
			return node.Content[0].Value
		}
	case yaml.MappingNode:
		if len(node.Content) > 0 {
			return node.Content[0].Value
		}
	}
	return ""
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Path is the absolute location of the Compose file this project was read from.
func (c *Config) Path() string { return c.path }

// Domains lists every local domain this project serves, primary first.
func (c *Config) Domains() []string {
	return append([]string{c.Domain}, c.ExtraDomains...)
}

// covers reports whether name falls under one of the project's domains.
func (c *Config) covers(name string) bool {
	for _, d := range c.Domains() {
		if strings.HasSuffix(name, "."+d) {
			return true
		}
	}
	return false
}

// Sorted returns services by name, so rendered output does not churn.
func (c *Config) Sorted() []*Service {
	out := make([]*Service, 0, len(c.Services))
	for _, s := range c.Services {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Ref returns the project's registry entry. It records a pinned domain only;
// an inherited domain is owned by the machine.
func (c *Config) Ref() GroupRef {
	ref := GroupRef{Name: c.Name, StackPath: c.path, Domains: c.ExtraDomains}
	if c.DomainPinned {
		ref.Domains = append([]string{c.Domain}, c.ExtraDomains...)
	}
	return ref
}

// validName reports whether the name can be used as a container id and as the
// leftmost label of a domain.
func validName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// --- Compose's two spellings ---------------------------------------------

// stringOrList decodes a Compose value written as a string or as a sequence.
type stringOrList []string

func (v *stringOrList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value == "" {
			return nil
		}
		*v = strings.Fields(node.Value)
		return nil
	case yaml.SequenceNode:
		var out []string
		if err := node.Decode(&out); err != nil {
			return err
		}
		*v = out
		return nil
	}
	return nil
}

// mapOrList decodes a Compose value written as a mapping or as a list of
// `KEY=value` entries.
type mapOrList map[string]string

func (v *mapOrList) UnmarshalYAML(node *yaml.Node) error {
	out := map[string]string{}
	switch node.Kind {
	case yaml.MappingNode:
		raw := map[string]any{}
		if err := node.Decode(&raw); err != nil {
			return err
		}
		for k, val := range raw {
			out[k] = fmt.Sprint(val)
		}
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return err
		}
		for _, item := range items {
			k, val, _ := strings.Cut(item, "=")
			out[strings.TrimSpace(k)] = val
		}
	}
	*v = out
	return nil
}
