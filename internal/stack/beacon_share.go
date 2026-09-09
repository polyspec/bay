package stack

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// allowBroadcast lets a socket send to a broadcast address, which the system
// refuses otherwise.
func allowBroadcast(network, address string, c syscall.RawConn) error {
	var setErr error
	err := c.Control(func(fd uintptr) {
		setErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
	})
	if err != nil {
		return err
	}
	return setErr
}

// shareAddress lets more than one program on this machine listen for the same
// announcements: the resident agent and whatever command was asked to find
// machines.
func shareAddress(network, address string, c syscall.RawConn) error {
	var setErr error
	err := c.Control(func(fd uintptr) {
		if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			setErr = err
			return
		}
		setErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return setErr
}
