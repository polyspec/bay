package stack

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// A project's services run on Docker with the commands they run with on Apple
// `container` wherever the two commands agree: create, start, stop, rm, exec
// and logs take the same arguments. They differ in four places, and each is
// translated here so the lifecycle reads one shape from either engine:
//
//   - Docker has no `ls --format json`. The containers are listed and then
//     inspected, as the machine's list reads them.
//   - Docker names a stopped container "created" or "exited" and Apple names it
//     "stopped". The lifecycle reads Apple's word.
//   - Docker identifies an image by its ID, and a container records the same
//     ID, so the ID is the digest the configuration identity compares.
//   - Docker's volume inspect has another shape and records no size, and its
//     local driver takes none.

// dockerStopped are the states Docker gives a container that is not running
// and has not been removed.
var dockerStopped = map[string]bool{"created": true, "exited": true, "dead": true}

// dockerNeverStarted is the start time Docker records for a container that has
// not been started. Apple records none.
const dockerNeverStarted = "0001-01-01T00:00:00Z"

// listDocker reads every container on Docker in the shape the lifecycle reads.
func (e *serviceEngine) listDocker(ctx context.Context) ([]Instance, error) {
	out, err := e.command(ctx, "container", "ls", "--all", "--quiet", "--no-trunc")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}
	out, err = e.command(ctx, append([]string{"inspect", "--type", "container"}, ids...)...)
	if err != nil {
		return nil, err
	}
	list, err := decodeDockerInstances(out)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i] = lifecycleState(list[i])
	}
	return list, nil
}

// lifecycleState gives a Docker container Apple's word for its state and no
// start time when it was never started.
func lifecycleState(in Instance) Instance {
	if dockerStopped[in.State] {
		in.State = "stopped"
	}
	if strings.HasPrefix(in.Started, "0001-01-01") {
		in.Started = ""
	}
	return in
}

// dockerImageID reads the ID out of `docker image inspect`.
func dockerImageID(out []byte, reference string) (string, error) {
	var images []struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(out, &images); err != nil {
		return "", err
	}
	if len(images) != 1 || !strings.HasPrefix(images[0].ID, "sha256:") {
		return "", fmt.Errorf("image %s has no resolved digest", reference)
	}
	return images[0].ID, nil
}

// imageMissing reports whether an inspect failed because the image is not
// held. Apple says "not found" and Docker says "No such image".
func imageMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "no such image")
}

// volumeMissing reports whether an inspect failed because the volume does not
// exist. Apple says "not found" and Docker says "no such volume".
func volumeMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "no such volume")
}

// decodeDockerVolumes reads `docker volume inspect` into the shape Apple's
// inspect has. Docker records no size.
func decodeDockerVolumes(out []byte) ([]runtimeVolume, error) {
	var raw []struct {
		Name   string            `json:"Name"`
		Driver string            `json:"Driver"`
		Labels map[string]string `json:"Labels"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	all := make([]runtimeVolume, len(raw))
	for i, v := range raw {
		all[i].Configuration.Name = v.Name
		all[i].Configuration.Driver = v.Driver
		all[i].Configuration.Labels = v.Labels
	}
	return all, nil
}

// dockerServiceArguments are the arguments a service's container is created
// with on Docker in addition to the ones both engines take.
//
// The proxy reaches a backend by the name <container>.container.test when its
// address stops answering. Apple's DNS answers that name; Docker's answers a
// container's name and its aliases on a user-defined network, so the name is
// given as an alias.
//
// A Docker machine runs without anyone at it, and Docker starts a container
// again after the machine restarts only when the container carries a restart
// policy. A service is given "unless-stopped", so one that `stop` stopped stays
// stopped; an initializer runs once and is given none.
func dockerServiceArguments(s *Service) []string {
	args := []string{"--network-alias", s.ContainerName + "." + BackendDomain}
	if !s.OneShot {
		args = append(args, "--restart", "unless-stopped")
	}
	return args
}

// createArguments returns the arguments a service's container is created with
// on this project's engine: the store of certificates this machine trusts, and
// on Docker its own arguments. They are placed before the image, because
// everything after the image is the service's command.
func (e *serviceEngine) createArguments(group string, s *Service, fingerprint string) []string {
	args := serviceArguments(group, s, fingerprint)
	extra := trustArguments(e.dir, s)
	if e.engine == DockerEngine {
		extra = append(extra, dockerServiceArguments(s)...)
	}
	if len(extra) == 0 {
		return args
	}
	image := len(args) - len(s.Command) - 1
	out := append([]string{}, args[:image]...)
	out = append(out, extra...)
	return append(out, args[image:]...)
}
