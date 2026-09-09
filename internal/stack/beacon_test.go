package stack

import (
	"context"
	"net"
	"testing"
	"time"
)

// A machine that stops announcing leaves the list, so a machine that was shut
// down is not offered as something to approve.
func TestAMachineUnheardLeavesTheList(t *testing.T) {
	h := newHeard(200 * time.Millisecond)
	h.put(Beacon{ID: "aa", Name: "alpha", Address: "10.0.0.1:8443"}, time.Now())
	if len(h.list(time.Now())) != 1 {
		t.Fatal("a machine that just announced is not in the list")
	}
	if got := h.list(time.Now().Add(time.Second)); len(got) != 0 {
		t.Errorf("%d machines after the announcements stopped, want 0", len(got))
	}
}

// A machine is its identity, not its name or address. Announcing again with
// both changed updates the entry rather than adding one.
func TestAnnouncingAgainUpdatesTheSameMachine(t *testing.T) {
	h := newHeard(time.Minute)
	now := time.Now()
	h.put(Beacon{ID: "aa", Name: "alpha", Address: "10.0.0.1:8443"}, now)
	h.put(Beacon{ID: "aa", Name: "renamed", Address: "10.0.0.9:8443"}, now)
	got := h.list(now)
	if len(got) != 1 {
		t.Fatalf("%d machines after one announced twice, want 1", len(got))
	}
	if got[0].Name != "renamed" || got[0].Address != "10.0.0.9:8443" {
		t.Errorf("the announcement was not taken: %+v", got[0])
	}
}

// The list is ordered, so what is printed does not depend on the order the
// announcements arrived in.
func TestTheListIsOrdered(t *testing.T) {
	h := newHeard(time.Minute)
	now := time.Now()
	h.put(Beacon{ID: "bb", Name: "beta"}, now)
	h.put(Beacon{ID: "aa", Name: "alpha"}, now)
	got := h.list(now)
	if len(got) != 2 || got[0].Name != "alpha" {
		t.Errorf("out of order: %+v", got)
	}
}

// What is announced reaches a machine listening for it.
func TestAnAnnouncementIsHeard(t *testing.T) {
	port := freeUDPPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	heard := make(chan Beacon, 1)
	go func() {
		_ = Listen(ctx, port, func(b Beacon) {
			select {
			case heard <- b:
			default:
			}
		})
	}()
	time.Sleep(200 * time.Millisecond)

	announce := Announcer{Port: port, To: "127.0.0.1", Every: 100 * time.Millisecond}
	go announce.Run(ctx, func() (Beacon, bool) {
		return Beacon{ID: "aa", Name: "alpha", Address: "10.0.0.1:8443"}, true
	})

	select {
	case b := <-heard:
		if b.ID != "aa" || b.Name != "alpha" {
			t.Errorf("heard %+v", b)
		}
	case <-ctx.Done():
		t.Fatal("nothing was heard")
	}
}

// A machine that is not announcing sends nothing.
func TestNothingIsAnnouncedWhileTheLinkIsClosed(t *testing.T) {
	port := freeUDPPort(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	heard := make(chan Beacon, 1)
	go func() {
		_ = Listen(ctx, port, func(b Beacon) {
			select {
			case heard <- b:
			default:
			}
		})
	}()
	time.Sleep(200 * time.Millisecond)

	announce := Announcer{Port: port, To: "127.0.0.1", Every: 100 * time.Millisecond}
	go announce.Run(ctx, func() (Beacon, bool) { return Beacon{}, false })

	select {
	case b := <-heard:
		t.Errorf("a closed link announced %+v", b)
	case <-ctx.Done():
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}
