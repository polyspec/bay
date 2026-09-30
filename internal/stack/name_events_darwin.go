//go:build darwin && cgo

package stack

/*
#include "name_events_darwin.h"
#include <stdlib.h>
#include <sys/socket.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"runtime/cgo"
	"strings"
	"sync"
	"unsafe"
)

type nativeNameState struct {
	ctx    context.Context
	events chan nameEvent
}

//export goNameEvent
func goNameEvent(handle C.uintptr_t, flags C.uint32_t, family C.int, data unsafe.Pointer, size C.int, code C.int) {
	state := cgo.Handle(handle).Value().(*nativeNameState)
	event := nameEvent{added: flags&C.kDNSServiceFlagsAdd != 0, more: flags&C.kDNSServiceFlagsMoreComing != 0}
	if code != 0 {
		event.err = fmt.Errorf("native DNS error %d", code)
	} else if (family == C.AF_INET && size == 4) || (family == C.AF_INET6 && size == 16) {
		address, ok := netip.AddrFromSlice(C.GoBytes(data, size))
		if !ok {
			event.err = errors.New("native DNS address bytes are invalid")
		} else {
			event.address = address
		}
	} else {
		event.err = fmt.Errorf("native DNS address family %d and length %d are invalid", family, size)
	}
	select {
	case state.events <- event:
	case <-state.ctx.Done():
	}
}

func nativeNameSubscription(ctx context.Context, host string, expected []netip.Addr) (*nameSubscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if host == "" || strings.ContainsRune(host, 0) {
		return nil, errors.New("invalid native DNS hostname")
	}
	var protocols C.uint32_t
	for _, address := range expected {
		if address.Is4() {
			protocols |= C.kDNSServiceProtocol_IPv4
		} else if address.Is6() {
			protocols |= C.kDNSServiceProtocol_IPv6
		} else {
			return nil, errors.New("invalid native DNS address protocol")
		}
	}
	if protocols == 0 {
		return nil, errors.New("native DNS requires an address protocol")
	}
	read, write, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	active, cancel := context.WithCancel(ctx)
	state := &nativeNameState{ctx: active, events: make(chan nameEvent, 16)}
	handle := cgo.NewHandle(state)
	cname := C.CString(host)
	var ref C.DNSServiceRef
	code := C.name_start(&ref, cname, protocols, C.uintptr_t(handle))
	C.free(unsafe.Pointer(cname))
	if code != 0 {
		handle.Delete()
		cancel()
		cleanup := errors.Join(read.Close(), write.Close())
		return nil, errors.Join(fmt.Errorf("native DNS subscribe: error %d", code), cleanup)
	}
	notified := make(chan error, 1)
	go func() {
		<-active.Done()
		_, err := write.Write([]byte{1})
		notified <- err
	}()
	done := make(chan struct{})
	var cleanup error
	go func() {
		defer close(done)
		defer close(state.events)
		for {
			code := C.name_next(ref, C.int(read.Fd()))
			if code == 1 {
				break
			}
			if code != 0 {
				event := nameEvent{err: fmt.Errorf("native DNS events: error %d", code)}
				select {
				case state.events <- event:
				case <-active.Done():
				}
				break
			}
		}
		C.DNSServiceRefDeallocate(ref)
		handle.Delete()
		cancel()
		cleanup = errors.Join(<-notified, read.Close(), write.Close())
	}()
	var once sync.Once
	return &nameSubscription{events: state.events, close: func() error {
		once.Do(cancel)
		<-done
		return cleanup
	}}, nil
}
