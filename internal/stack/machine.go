package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Machine-level names. One proxy and one DNS server run per machine, and
// projects register routes with them.
const (
	ProxyName        = "bay-edge"
	ProxyImage       = "nginx:1.29-alpine"
	ProxyNetwork     = "default"
	BackendDomain    = "container.test"
	DefaultDNSAddr   = "127.0.0.1:5354"
	groupsFileName   = "groups.json"
	settingsFileName = "machine.json"
	groupsFileMode   = 0o644
	groupsDirIsMine  = 0o700
)

// GroupRef records a project between invocations: the path of its Compose file
// and the domains it pins. The domains are recorded while the project is down,
// because the resolver entries that delegate them are machine state.
type GroupRef struct {
	Name      string    `json:"name"`
	StackPath string    `json:"stackPath"`
	Domains   []string  `json:"domains"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Machine is the per-user state directory. It holds the certificate authority,
// the issued certificates, the generated proxy configuration and the project
// registry.
type Machine struct {
	Dir string
}

func NewMachine(dir string) *Machine { return &Machine{Dir: dir} }

func (m *Machine) CertDir() string      { return filepath.Join(m.Dir, "certs") }
func (m *Machine) ConfDir() string      { return filepath.Join(m.Dir, "conf.d") }
func (m *Machine) LogDir() string       { return filepath.Join(m.Dir, "logs") }
func (m *Machine) groupsPath() string   { return filepath.Join(m.Dir, groupsFileName) }
func (m *Machine) settingsPath() string { return filepath.Join(m.Dir, settingsFileName) }

// Settings holds the machine's domains. A domain is delegated once for the
// machine in /etc/resolver, so it is configured here rather than in each
// project. A project may pin a domain in its Compose file; the machine
// delegates the union.
type Settings struct {
	// Domain is the domain a project uses when its Compose file does not set
	// one.
	Domain string `json:"domain"`
	// Extra are further domains delegated on this machine.
	Extra []string `json:"extra,omitempty"`
}

// DefaultDomain is the domain a project uses when none is set. RFC 6761
// reserves "test", so it does not collide with a registered name.
const DefaultDomain = "test"

func (m *Machine) Settings() (Settings, error) {
	var s Settings
	b, err := os.ReadFile(m.settingsPath())
	if err == nil {
		if err := json.Unmarshal(b, &s); err != nil {
			return s, fmt.Errorf("%s: %w", m.settingsPath(), err)
		}
	} else if !os.IsNotExist(err) {
		return s, err
	}
	if s.Domain == "" {
		s.Domain = DefaultDomain
	}
	return s, nil
}

func (m *Machine) SaveSettings(s Settings) error {
	if s.Domain == "" {
		s.Domain = DefaultDomain
	}
	if err := os.MkdirAll(m.Dir, groupsDirIsMine); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), groupsFileMode); err != nil {
		return err
	}
	return os.Rename(tmp, m.settingsPath())
}

// AddDomain delegates another domain on this machine.
func (m *Machine) AddDomain(domain string) error {
	domain = normalizeDomain(domain)
	if err := checkDomain(domain); err != nil {
		return err
	}
	current, err := m.Domains()
	if err != nil {
		return err
	}
	for _, d := range current {
		if d == domain {
			return fmt.Errorf("%s is already delegated on this machine", domain)
		}
	}
	s, err := m.Settings()
	if err != nil {
		return err
	}
	s.Extra = append(s.Extra, domain)
	return m.SaveSettings(s)
}

// RemoveDomain stops delegating a domain. It refuses the default domain and a
// domain pinned by a project.
func (m *Machine) RemoveDomain(domain string) error {
	domain = normalizeDomain(domain)
	s, err := m.Settings()
	if err != nil {
		return err
	}
	if domain == s.Domain {
		return fmt.Errorf("%s is this machine's default domain; set another default first", domain)
	}
	if users, err := m.projectsUsing(domain); err != nil {
		return err
	} else if len(users) > 0 {
		return fmt.Errorf("%s is pinned by %s", domain, strings.Join(users, ", "))
	}
	kept := s.Extra[:0]
	found := false
	for _, d := range s.Extra {
		if d == domain {
			found = true
			continue
		}
		kept = append(kept, d)
	}
	if !found {
		return fmt.Errorf("%s is not one of this machine's domains", domain)
	}
	s.Extra = kept
	return m.SaveSettings(s)
}

// SetDefaultDomain sets the domain projects use when none is set, and delegates
// it when it is new.
func (m *Machine) SetDefaultDomain(domain string) error {
	domain = normalizeDomain(domain)
	if err := checkDomain(domain); err != nil {
		return err
	}
	s, err := m.Settings()
	if err != nil {
		return err
	}
	kept := s.Extra[:0]
	for _, d := range s.Extra {
		if d != domain {
			kept = append(kept, d)
		}
	}
	if s.Domain != "" && s.Domain != domain {
		kept = append(kept, s.Domain)
	}
	s.Domain, s.Extra = domain, kept
	return m.SaveSettings(s)
}

// projectsUsing returns the registered projects that pin the domain.
func (m *Machine) projectsUsing(domain string) ([]string, error) {
	groups, err := m.Groups()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, g := range groups {
		for _, d := range g.Domains {
			if normalizeDomain(d) == domain {
				out = append(out, g.Name)
				break
			}
		}
	}
	return out, nil
}

// Groups lists the registered groups, ordered by name.
func (m *Machine) Groups() ([]GroupRef, error) {
	b, err := os.ReadFile(m.groupsPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var byName map[string]GroupRef
	if err := json.Unmarshal(b, &byName); err != nil {
		return nil, fmt.Errorf("%s: %w", m.groupsPath(), err)
	}
	out := make([]GroupRef, 0, len(byName))
	for _, g := range byName {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Register records a project and the domains it pins, replacing an earlier
// entry with the same name.
func (m *Machine) Register(g GroupRef) error {
	g.UpdatedAt = time.Now()
	return m.mutateGroups(func(byName map[string]GroupRef) {
		byName[g.Name] = g
	})
}

// Unregister removes a project. Its domains no longer count towards the
// resolver entries the machine needs.
func (m *Machine) Unregister(name string) error {
	return m.mutateGroups(func(byName map[string]GroupRef) {
		delete(byName, name)
	})
}

func (m *Machine) mutateGroups(fn func(map[string]GroupRef)) error {
	byName := map[string]GroupRef{}
	if b, err := os.ReadFile(m.groupsPath()); err == nil {
		if err := json.Unmarshal(b, &byName); err != nil {
			return fmt.Errorf("%s: %w", m.groupsPath(), err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	fn(byName)

	if err := os.MkdirAll(m.Dir, groupsDirIsMine); err != nil {
		return err
	}
	b, err := json.MarshalIndent(byName, "", "  ")
	if err != nil {
		return err
	}
	// Write through a temporary file so an interrupted write cannot truncate
	// the registry.
	tmp := m.groupsPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), groupsFileMode); err != nil {
		return err
	}
	return os.Rename(tmp, m.groupsPath())
}

// Domains returns the domains the resolver entries must cover: the machine's
// domains and every domain a project pins.
func (m *Machine) Domains() ([]string, error) {
	settings, err := m.Settings()
	if err != nil {
		return nil, err
	}
	groups, err := m.Groups()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range append([]string{settings.Domain}, settings.Extra...) {
		d = strings.Trim(strings.ToLower(d), ".")
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, g := range groups {
		for _, d := range g.Domains {
			d = strings.Trim(strings.ToLower(d), ".")
			if d != "" && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}
