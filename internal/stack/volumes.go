package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const LabelVolume = "bay.volume"

type Volume struct {
	Key, Name string
	External  bool
	Size      int64
}

func decodeVolume(project, key string, node yaml.Node) (*Volume, error) {
	if !validName(key) {
		return nil, fmt.Errorf("invalid volume key %q", key)
	}
	if node.Kind == 0 || node.Tag == "!!null" {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	if err := mappingKeys(node, "name", "external", "driver", "driver_opts"); err != nil {
		return nil, fmt.Errorf("volume %s: %w", key, err)
	}
	var raw struct {
		Name     string    `yaml:"name"`
		External bool      `yaml:"external"`
		Driver   string    `yaml:"driver"`
		Options  yaml.Node `yaml:"driver_opts"`
	}
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}
	if raw.Driver != "" && raw.Driver != "local" {
		return nil, fmt.Errorf("volume %s: only local driver is supported", key)
	}
	if raw.External && (raw.Driver != "" || raw.Options.Kind != 0) {
		return nil, fmt.Errorf("external volume %s cannot declare a driver or driver_opts", key)
	}
	v := &Volume{Key: key, Name: raw.Name, External: raw.External}
	if v.Name == "" {
		if v.External {
			v.Name = key
		} else {
			v.Name = project + "-" + key
		}
	}
	if !validName(v.Name) {
		return nil, fmt.Errorf("invalid volume name %q", v.Name)
	}
	if raw.Options.Kind != 0 {
		if err := mappingKeys(raw.Options, "size"); err != nil {
			return nil, fmt.Errorf("volume %s driver_opts: %w", key, err)
		}
		var options struct {
			Size *string `yaml:"size"`
		}
		if err := raw.Options.Decode(&options); err != nil {
			return nil, err
		}
		if options.Size != nil {
			size, err := volumeSize(*options.Size)
			if err != nil {
				return nil, fmt.Errorf("volume %s: %w", key, err)
			}
			v.Size = size
		}
	}
	return v, nil
}

func volumeSize(value string) (int64, error) {
	if value == "" {
		return 0, fmt.Errorf("volume size must be positive")
	}
	suffix := strings.ToUpper(value[len(value)-1:])
	power := 0
	if index := strings.Index("KMGTP", suffix); index >= 0 {
		power = index + 1
		value = value[:len(value)-1]
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("volume size must be positive bytes with an optional K/M/G/T/P suffix")
	}
	for range power {
		if n > math.MaxInt64/1024 {
			return 0, fmt.Errorf("volume size overflows int64")
		}
		n *= 1024
	}
	return n, nil
}

type runtimeVolume struct {
	Configuration struct {
		Name, Driver string
		CreationDate string `json:"creationDate"`
		Source       string `json:"source"`
		SizeInBytes  int64  `json:"sizeInBytes"`
		Labels       map[string]string
	} `json:"configuration"`
}

func volumeFingerprint(group string, v *Volume) string {
	data, _ := json.Marshal(struct {
		Group  string
		Volume *Volume
	}{group, v})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// volumeCommand runs one volume command under its own bound. A batch of
// volume commands shares no deadline, because a shared deadline ends a healthy
// command once the batch has taken longer than the deadline.
func (e *serviceEngine) volumeCommand(wait time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	return e.command(ctx, append([]string{"volume"}, args...)...)
}

// inspectVolume reads a volume the runtime holds, under the read bound.
func (e *serviceEngine) inspectVolume(name string) (runtimeVolume, bool, error) {
	out, err := e.volumeCommand(engineReadWait, "inspect", name)
	if err != nil {
		if volumeMissing(err) {
			return runtimeVolume{}, false, nil
		}
		return runtimeVolume{}, false, err
	}
	var all []runtimeVolume
	if e.engine == DockerEngine {
		all, err = decodeDockerVolumes(out)
	} else {
		err = json.Unmarshal(out, &all)
	}
	if err != nil {
		return runtimeVolume{}, false, err
	}
	if len(all) != 1 || all[0].Configuration.Name != name {
		return runtimeVolume{}, false, fmt.Errorf("volume %s has no stable runtime identity", name)
	}
	return all[0], true, nil
}

// checkVolume compares a volume the runtime holds with its declaration. Docker
// records no size, so on Docker the size is not compared.
func checkVolume(group string, v *Volume, actual runtimeVolume) error {
	return checkVolumeOn(AppleEngine, group, v, actual)
}

func checkVolumeOn(engine, group string, v *Volume, actual runtimeVolume) error {
	c := actual.Configuration
	if v.External {
		return nil
	}
	if c.Labels[LabelRole] != "volume" || c.Labels[LabelGroup] != group || c.Labels[LabelVolume] != v.Key {
		return fmt.Errorf("volume %s is not owned by project %s declaration %s", v.Name, group, v.Key)
	}
	if c.Labels[LabelConfig] != volumeFingerprint(group, v) || c.Driver != "local" || engine != DockerEngine && (c.SizeInBytes <= 0 || v.Size > 0 && c.SizeInBytes != v.Size) {
		return fmt.Errorf("volume %s configuration differs; automatic resize or replacement is forbidden", v.Name)
	}
	return nil
}

func (e *serviceEngine) prepareVolumes(group string, services []*Service, create bool) error {
	selected := map[string]*Volume{}
	order := []*Volume{}
	for _, s := range services {
		for _, v := range s.NamedVolumes {
			if selected[v.Name] == nil {
				selected[v.Name] = v
				order = append(order, v)
			}
		}
	}
	missing := []*Volume{}
	for _, v := range order {
		actual, found, err := e.inspectVolume(v.Name)
		if err != nil {
			return err
		}
		if found {
			if err := checkVolumeOn(e.engine, group, v, actual); err != nil {
				return err
			}
		} else if v.External {
			return fmt.Errorf("external volume %s is unavailable", v.Name)
		} else {
			missing = append(missing, v)
		}
	}
	if !create {
		return nil
	}
	for _, v := range missing {
		// Docker's local driver takes no size, so a declared size cannot be
		// created there and is refused before anything is created.
		if e.engine == DockerEngine && v.Size > 0 {
			return fmt.Errorf("volume %s declares a size; Docker's local driver creates a volume without one", v.Name)
		}
	}
	for _, v := range missing {
		args := []string{"create", "--label", LabelRole + "=volume", "--label", LabelGroup + "=" + group, "--label", LabelVolume + "=" + v.Key, "--label", LabelConfig + "=" + volumeFingerprint(group, v)}
		if v.Size > 0 {
			args = append(args, "-s", strconv.FormatInt(v.Size, 10))
		}
		args = append(args, v.Name)
		// Creating is an action of the engine and has the action bound.
		if _, err := e.volumeCommand(engineActWait, args...); err != nil {
			return fmt.Errorf("create volume %s: %w", v.Name, err)
		}
		actual, found, err := e.inspectVolume(v.Name)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("created volume %s is unavailable", v.Name)
		}
		if err := checkVolumeOn(e.engine, group, v, actual); err != nil {
			return err
		}
	}
	return nil
}
