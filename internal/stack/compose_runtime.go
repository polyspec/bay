package stack

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Healthcheck struct {
	Test        []string
	Interval    time.Duration
	Timeout     time.Duration
	StartPeriod time.Duration
	Retries     int
}

func mappingKeys(node yaml.Node, allowed ...string) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		known := false
		for _, key := range allowed {
			if node.Content[i].Value == key {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unsupported field %q", node.Content[i].Value)
		}
	}
	return nil
}

func decodeDependencies(node yaml.Node) (map[string]string, error) {
	out := map[string]string{}
	if node.Kind == 0 {
		return out, nil
	}
	if node.Kind == yaml.SequenceNode {
		var names []string
		if err := node.Decode(&names); err != nil {
			return nil, err
		}
		for _, name := range names {
			if _, ok := out[name]; ok {
				return nil, fmt.Errorf("duplicate dependency %s", name)
			}
			out[name] = "service_started"
		}
		return out, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("depends_on must be a list or mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		name, value := node.Content[i].Value, *node.Content[i+1]
		if err := mappingKeys(value, "condition"); err != nil {
			return nil, fmt.Errorf("depends_on %s: %w", name, err)
		}
		var condition struct {
			Condition string `yaml:"condition"`
		}
		if err := value.Decode(&condition); err != nil {
			return nil, err
		}
		switch condition.Condition {
		case "service_started", "service_healthy", "service_completed_successfully":
		default:
			return nil, fmt.Errorf("unsupported depends_on condition for %s", name)
		}
		out[name] = condition.Condition
	}
	return out, nil
}

func decodeHealthcheck(node yaml.Node) (*Healthcheck, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if err := mappingKeys(node, "test", "interval", "timeout", "retries", "start_period", "disable"); err != nil {
		return nil, fmt.Errorf("healthcheck: %w", err)
	}
	var raw struct {
		Test        yaml.Node `yaml:"test"`
		Interval    string    `yaml:"interval"`
		Timeout     string    `yaml:"timeout"`
		StartPeriod string    `yaml:"start_period"`
		Retries     *int      `yaml:"retries"`
		Disable     bool      `yaml:"disable"`
	}
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}
	if raw.Disable {
		return nil, nil
	}
	h := &Healthcheck{Interval: 30 * time.Second, Timeout: 30 * time.Second, Retries: 3}
	switch raw.Test.Kind {
	case yaml.ScalarNode:
		h.Test = []string{"CMD-SHELL", raw.Test.Value}
	case yaml.SequenceNode:
		if err := raw.Test.Decode(&h.Test); err != nil {
			return nil, err
		}
	}
	if len(h.Test) == 1 && h.Test[0] == "NONE" {
		return nil, nil
	}
	if len(h.Test) < 2 || h.Test[0] != "CMD" && h.Test[0] != "CMD-SHELL" || h.Test[0] == "CMD-SHELL" && len(h.Test) != 2 {
		return nil, fmt.Errorf("healthcheck test must be CMD arguments or one CMD-SHELL string")
	}
	if raw.Retries != nil {
		h.Retries = *raw.Retries
	}
	if h.Retries < 1 || h.Retries > 100 {
		return nil, fmt.Errorf("healthcheck retries must be 1..100")
	}
	for _, item := range []struct {
		value  string
		target *time.Duration
		zero   bool
	}{{raw.Interval, &h.Interval, false}, {raw.Timeout, &h.Timeout, false}, {raw.StartPeriod, &h.StartPeriod, true}} {
		if item.value == "" {
			continue
		}
		d, err := time.ParseDuration(item.value)
		if err != nil || d < 0 || !item.zero && d == 0 || d > 10*time.Minute {
			return nil, fmt.Errorf("healthcheck duration must be positive and at most 10m (start_period may be zero)")
		}
		*item.target = d
	}
	if h.StartPeriod+time.Duration(h.Retries)*(h.Interval+h.Timeout) > 30*time.Minute {
		return nil, fmt.Errorf("healthcheck startup budget exceeds 30m")
	}
	return h, nil
}

func validateDependencies(c *Config) error {
	for _, s := range c.Sorted() {
		for name, condition := range s.DependsOn {
			dep := c.Services[name]
			if dep == nil {
				return fmt.Errorf("service %s depends on missing service %s", s.Name, name)
			}
			if condition == "service_healthy" && dep.Healthcheck == nil {
				return fmt.Errorf("service %s requires a healthcheck on %s", s.Name, name)
			}
			if condition == "service_completed_successfully" {
				if !dep.Internal {
					return fmt.Errorf("completed service %s must be internal", name)
				}
				dep.OneShot = true
			}
		}
	}
	_, err := dependencyOrder(c, c.Sorted())
	return err
}

func dependencyOrder(c *Config, targets []*Service) ([]*Service, error) {
	state := map[string]int{}
	out := []*Service{}
	var visit func(*Service) error
	visit = func(s *Service) error {
		if state[s.Name] == 1 {
			return fmt.Errorf("dependency cycle at %s", s.Name)
		}
		if state[s.Name] == 2 {
			return nil
		}
		state[s.Name] = 1
		names := make([]string, 0, len(s.DependsOn))
		for name := range s.DependsOn {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if err := visit(c.Services[name]); err != nil {
				return err
			}
		}
		state[s.Name] = 2
		out = append(out, s)
		return nil
	}
	for _, s := range targets {
		if err := visit(s); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func normalizeVolumes(c *Config, s *Service) error {
	for i, volume := range s.Volumes {
		parts := strings.Split(volume, ":")
		if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || !filepath.IsAbs(parts[1]) {
			return fmt.Errorf("service %s: volumes require source:absolute-target[:ro|rw]", s.Name)
		}
		if len(parts) == 3 && parts[2] != "ro" && parts[2] != "rw" {
			return fmt.Errorf("service %s: unsupported volume mode", s.Name)
		}
		if filepath.IsAbs(parts[0]) || strings.HasPrefix(parts[0], ".") {
			if !filepath.IsAbs(parts[0]) {
				parts[0] = filepath.Join(filepath.Dir(c.path), parts[0])
			}
			parts[0] = filepath.Clean(parts[0])
		} else {
			volume, ok := c.Volumes[parts[0]]
			if !ok {
				return fmt.Errorf("service %s: volume %s is not declared", s.Name, parts[0])
			}
			parts[0] = volume.Name
			s.NamedVolumes = append(s.NamedVolumes, volume)
		}
		s.Volumes[i] = strings.Join(parts, ":")
	}
	return nil
}
