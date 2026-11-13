package stack

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrInterrupted is the error of an engine command refused after Interrupt.
var ErrInterrupted = errors.New("interrupted")

// A command that changes containers can be interrupted. Interrupt ends every
// engine command that waits or reads (a pull, a log, an inspection) and refuses
// every command that would change state afterwards; a command that changes
// state and has started is let finish, so what it made exists when the cleanup
// lists it. Resume lets the next engine command run, which the cleanup after
// an interruption needs.

var (
	// inflight counts the engine commands changing state that run now.
	inflight     sync.WaitGroup
	interruptMu  sync.Mutex
	interruptCtx = context.Background()
	interruptEnd context.CancelFunc
)

// Interrupt ends the engine commands that run now and refuses the ones that
// start until Resume.
func Interrupt() {
	interruptMu.Lock()
	defer interruptMu.Unlock()
	if interruptEnd == nil {
		interruptCtx, interruptEnd = context.WithCancel(context.Background())
	}
	interruptEnd()
}

// Resume lets engine commands run again after Interrupt.
func Resume() {
	interruptMu.Lock()
	defer interruptMu.Unlock()
	interruptCtx, interruptEnd = context.Background(), nil
}

// cleanupKey marks the context of the cleanup after an interruption, whose
// commands run although the interrupted operation's are refused.
type cleanupKey struct{}

// forCleanup returns ctx for the commands of the cleanup.
func forCleanup(ctx context.Context) context.Context {
	return context.WithValue(ctx, cleanupKey{}, true)
}

// refused reports whether a command changing state may not start now.
func refused(ctx context.Context) bool {
	return Interrupted() && ctx.Value(cleanupKey{}) == nil
}

// Settle waits until the engine commands changing state that had started when
// Interrupt was called have finished, at most for limit, so what they made is
// listed by the cleanup. It reports whether they finished.
func Settle(limit time.Duration) bool {
	done := make(chan struct{})
	go func() { inflight.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}

// interruptible returns ctx ended also by Interrupt.
func interruptible(ctx context.Context) (context.Context, context.CancelFunc) {
	interruptMu.Lock()
	current := interruptCtx
	interruptMu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(current, cancel)
	return ctx, func() { stop(); cancel() }
}

// ProjectContainers returns the service containers of a project, by name, with
// when each was created. A container created again has the same name and
// another creation time.
func (r *Runtime) ProjectContainers(group string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.engine().timeout)
	defer cancel()
	list, err := r.engine().projectContainers(ctx, group)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, in := range list {
		out[in.Name] = in.Created
	}
	return out, nil
}

// RemoveCreatedSince removes the service containers of a project that are not
// in before, or were created again since, and withdraws their routes. With no
// container before, the project's registration is withdrawn too, so an
// interrupted first up leaves nothing behind. Volumes are kept: they hold data.
func (r *Runtime) RemoveCreatedSince(group string, before map[string]string) error {
	e := r.engine()
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	ctx = forCleanup(ctx)
	list, err := e.projectContainers(ctx, group)
	if err != nil {
		return err
	}
	var created []Instance
	for _, in := range list {
		if at, ok := before[in.Name]; !ok || at != in.Created {
			created = append(created, in)
		}
	}
	if err := e.removeContainers(ctx, created); err != nil {
		return err
	}
	if len(before) == 0 {
		if err := r.Machine.Unregister(group); err != nil {
			return err
		}
	}
	_, err = r.sync(nil)
	return err
}

// Interrupted reports whether Interrupt was called and Resume has not been.
func Interrupted() bool {
	interruptMu.Lock()
	defer interruptMu.Unlock()
	return interruptEnd != nil
}
