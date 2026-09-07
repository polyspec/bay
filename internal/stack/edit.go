package stack

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Edits are applied to the YAML node tree rather than by re-encoding the
// Config, so comments, key order and formatting are preserved.

// AddDomain adds a domain to the group's extra_domains.
func AddDomain(cfg *Config, domain string) error {
	domain = normalizeDomain(domain)
	if err := checkDomain(domain); err != nil {
		return err
	}
	for _, d := range cfg.Domains() {
		if d == domain {
			return fmt.Errorf("%q already serves %s", cfg.Name, domain)
		}
	}
	return editStack(cfg, func(root *yaml.Node) error {
		settings := ensureMapping(root, projectKey)
		list := ensureKey(settings, "extra_domains", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
		list.Kind = yaml.SequenceNode
		list.Tag = "!!seq"
		list.Style = 0
		list.Content = append(list.Content, scalar(domain))
		return nil
	})
}

// RemoveDomain removes a domain from extra_domains. It refuses the primary
// domain and a domain a service still claims.
func RemoveDomain(cfg *Config, domain string) error {
	domain = normalizeDomain(domain)
	if domain == cfg.Domain {
		return fmt.Errorf("%s is the group's primary domain; change it instead of removing it", domain)
	}
	if users := servicesUnder(cfg, domain); len(users) > 0 {
		return fmt.Errorf("%s is still used by %s", domain, strings.Join(users, ", "))
	}
	return editStack(cfg, func(root *yaml.Node) error {
		list := findKey(findKey(root, projectKey), "extra_domains")
		if list == nil {
			return fmt.Errorf("%s has no extra domains", cfg.Name)
		}
		kept := list.Content[:0]
		found := false
		for _, n := range list.Content {
			if normalizeDomain(n.Value) == domain {
				found = true
				continue
			}
			kept = append(kept, n)
		}
		if !found {
			return fmt.Errorf("%s does not serve %s", cfg.Name, domain)
		}
		list.Content = kept
		return nil
	})
}

// SetPrimaryDomain changes the project's domain and moves every service that
// used the previous domain.
func SetPrimaryDomain(cfg *Config, domain string) error {
	domain = normalizeDomain(domain)
	if err := checkDomain(domain); err != nil {
		return err
	}
	if domain == cfg.Domain {
		return nil
	}
	for _, d := range cfg.ExtraDomains {
		if d == domain {
			return fmt.Errorf("%s is already an extra domain of this group", domain)
		}
	}
	old := cfg.Domain
	return editStack(cfg, func(root *yaml.Node) error {
		settings := ensureMapping(root, projectKey)
		ensureKey(settings, "domain", scalar(domain)).Value = domain

		services := findKey(root, "services")
		if services == nil {
			return nil
		}
		for i := 0; i+1 < len(services.Content); i += 2 {
			name, body := services.Content[i].Value, services.Content[i+1]
			s := cfg.Services[strings.ToLower(name)]
			if s == nil || s.Internal {
				continue
			}
			// Only rewrite domains that actually sat under the old name, and
			// only where the file spelled them out; the rest re-derive.
			if !strings.HasSuffix(s.Domain, "."+old) {
				continue
			}
			if node := findLabel(body, LabelKeyDomain); node != nil {
				node.Value = strings.TrimSuffix(s.Domain, "."+old) + "." + domain
			}
		}
		return nil
	})
}

// SetServiceDomain sets one service's domain. The name must be under one of the
// project's domains.
func SetServiceDomain(cfg *Config, service, domain string) error {
	domain = normalizeDomain(domain)
	if err := checkDomain(domain); err != nil {
		return err
	}
	s, ok := cfg.Services[strings.ToLower(service)]
	if !ok {
		return fmt.Errorf("group %q has no service %q", cfg.Name, service)
	}
	if !cfg.covers(domain) {
		return fmt.Errorf("%s is outside .%s - add the domain to this group first",
			domain, strings.Join(cfg.Domains(), ", ."))
	}
	for _, other := range cfg.Sorted() {
		if other.Name != s.Name && other.Domain == domain {
			return fmt.Errorf("%s already claims %s", other.Name, domain)
		}
	}
	return editStack(cfg, func(root *yaml.Node) error {
		services := findKey(root, "services")
		if services == nil {
			return fmt.Errorf("no services in %s", cfg.Path())
		}
		for i := 0; i+1 < len(services.Content); i += 2 {
			if strings.ToLower(services.Content[i].Value) != s.Name {
				continue
			}
			setLabel(services.Content[i+1], LabelKeyDomain, domain)
			return nil
		}
		return fmt.Errorf("service %q not found in %s", s.Name, cfg.Path())
	})
}

// editStack applies fn to the file's node tree and writes the result only after
// it loads without error.
func editStack(cfg *Config, fn func(*yaml.Node) error) error {
	b, err := os.ReadFile(cfg.Path())
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s is not a mapping", cfg.Path())
	}
	if err := fn(doc.Content[0]); err != nil {
		return err
	}

	out, err := marshalNode(&doc)
	if err != nil {
		return err
	}
	tmp := cfg.Path() + ".bay-edit"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err := Load(tmp); err != nil {
		return fmt.Errorf("the edit would make %s unloadable: %w", cfg.Path(), err)
	}
	return os.WriteFile(cfg.Path(), out, 0o644)
}

func marshalNode(doc *yaml.Node) ([]byte, error) {
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

// projectKey is the Compose extension field holding this tool's settings.
const projectKey = "x-bay"

// ensureMapping returns the mapping under key, creating it when absent.
func ensureMapping(root *yaml.Node, key string) *yaml.Node {
	node := ensureKey(root, key, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
	if node.Kind != yaml.MappingNode {
		node.Kind, node.Tag, node.Value, node.Content = yaml.MappingNode, "!!map", "", nil
	}
	return node
}

// findLabel returns a service's label value. Compose accepts a mapping or a
// list.
func findLabel(service *yaml.Node, key string) *yaml.Node {
	labels := findKey(service, "labels")
	if labels == nil {
		return nil
	}
	switch labels.Kind {
	case yaml.MappingNode:
		return findKey(labels, key)
	case yaml.SequenceNode:
		for _, item := range labels.Content {
			if k, _, ok := strings.Cut(item.Value, "="); ok && strings.TrimSpace(k) == key {
				return item
			}
		}
	}
	return nil
}

// setLabel writes a service label and keeps the form the file uses.
func setLabel(service *yaml.Node, key, value string) {
	labels := findKey(service, "labels")
	if labels != nil && labels.Kind == yaml.SequenceNode {
		for _, item := range labels.Content {
			if k, _, ok := strings.Cut(item.Value, "="); ok && strings.TrimSpace(k) == key {
				item.Value = key + "=" + value
				return
			}
		}
		labels.Content = append(labels.Content, scalar(key+"="+value))
		return
	}
	mapping := ensureMapping(service, "labels")
	ensureKey(mapping, key, scalar(value)).Value = value
}

func findKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// ensureKey returns the value node for key, appending placeholder when the key
// is absent.
func ensureKey(mapping *yaml.Node, key string, placeholder *yaml.Node) *yaml.Node {
	if v := findKey(mapping, key); v != nil {
		return v
	}
	mapping.Content = append(mapping.Content, scalar(key), placeholder)
	return placeholder
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func normalizeDomain(d string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(d)), ".")
}

// checkDomain reports an error for a name that cannot be used as a local
// domain.
func checkDomain(d string) error {
	if d == "" {
		return fmt.Errorf("the domain is empty")
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" {
			return fmt.Errorf("%q has an empty label", d)
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			default:
				return fmt.Errorf("%q may only contain letters, digits, - and .", d)
			}
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("%q has a label starting or ending with -", d)
		}
	}
	return nil
}

func servicesUnder(cfg *Config, domain string) []string {
	var out []string
	for _, s := range cfg.Sorted() {
		if s.Domain == domain || strings.HasSuffix(s.Domain, "."+domain) {
			out = append(out, s.Name)
		}
	}
	return out
}
