package stack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Several commands run at once on one machine: a test suite brings up a
// project per test in parallel. What they share is serialised here, by an
// advisory lock on a file in the state directory, so it holds across
// processes:
//
//   - the state lock covers reading and rewriting a shared file (the project
//     registry, the settings, the approved peers, the store containers
//     trust). Each rewrite reads the file under the lock, so no command writes
//     back a registry that misses another's project.
//   - the proxy lock covers publishing the proxy: its configuration, the
//     certificates it serves and the container itself. Two commands that each
//     found no proxy would otherwise both create one.
//
// A file is replaced through a temporary file of its own name and this
// process, so two writers never take each other's temporary file.

const (
	stateLockName = "state"
	proxyLockName = "proxy"
	// stateLockWait bounds the wait for a rewrite, which takes milliseconds.
	stateLockWait = 30 * time.Second
	// proxyLockWait bounds the wait for a publish, which waits for the proxy
	// to connect to each started service.
	proxyLockWait = 5 * time.Minute
)

// withLock runs fn holding the named lock of the state directory.
func withLock(dir, name string, wait time.Duration, fn func() error) error {
	if dir == "" {
		return fn()
	}
	locks := filepath.Join(dir, "locks")
	if err := os.MkdirAll(locks, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(locks, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if err := sleepContext(ctx, 20*time.Millisecond); err != nil {
			return fmt.Errorf("waiting for the %s lock in %s: another bay command holds it: %w", name, locks, err)
		}
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return fn()
}

// replaceFile writes content to path through a temporary file in the same
// directory, named for this process, and renames it into place, so a reader
// sees the old file or the new one and two writers do not share a temporary
// file.
func replaceFile(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
