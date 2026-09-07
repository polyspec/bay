package stack

import (
	"net"
	"strconv"
	"time"
)

// A container reports as running as soon as the runtime has started it, which
// is before the process inside listens on its port. Requests during that window
// fail: the proxy returns 502 for a routed service, and a connection from
// another service times out. Readiness is therefore a separate question,
// answered by connecting to the port the service declares.

// readyTimeout bounds one connection attempt.
const readyTimeout = 700 * time.Millisecond

// Ready reports whether the service accepts a connection on its port. A service
// that is not running is never ready.
func (s ServiceInstance) Ready() bool {
	if !s.Running() || s.IPv4 == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort(s.IPv4, strconv.Itoa(s.Port)), readyTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// WaitReady returns when every named container accepts a connection, or when
// the timeout expires. It returns the containers that were still not accepting
// connections, so the caller can report them rather than fail.
func WaitReady(containers []string, timeout time.Duration) ([]string, error) {
	deadline := time.Now().Add(timeout)
	for {
		instances, err := Instances()
		if err != nil {
			return nil, err
		}
		byName := make(map[string]ServiceInstance, len(instances))
		for _, in := range instances {
			byName[in.Container] = in
		}

		var pending []string
		for _, name := range containers {
			if in, ok := byName[name]; !ok || !in.Ready() {
				pending = append(pending, name)
			}
		}
		if len(pending) == 0 {
			return nil, nil
		}
		if time.Now().After(deadline) {
			return pending, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}
