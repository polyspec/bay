package stack

import "context"

// removeUndeclared removes the project's service containers whose service the
// Compose file no longer declares. A service removed from the file keeps its
// container, and the container keeps its route, until this removes it. Each
// removal is reported before it starts. Containers of other projects and the
// proxy carry other labels and are not listed.
func (e *serviceEngine) removeUndeclared(cfg *Config) error {
	return e.locked(cfg.Name, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		defer cancel()
		containers, err := e.projectContainers(ctx, cfg.Name)
		if err != nil {
			return err
		}
		var undeclared []Instance
		for _, in := range containers {
			service := in.Labels[LabelService]
			if _, declared := cfg.Services[service]; declared {
				continue
			}
			e.say("%s: the Compose file does not declare it; removing its container", service)
			undeclared = append(undeclared, in)
		}
		return e.removeContainers(ctx, undeclared)
	})
}
