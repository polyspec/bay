package stack

import (
	"context"
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"
)

// A machine with the link open announces itself to the network, so another
// machine finds it without being given an address.
//
// The announcement is a broadcast rather than a multicast: access points
// commonly prune multicast between clients while still flooding broadcast to
// the subnet, so this reaches machines where a multicast would not. It is also
// one path on every system, with nothing per-system behind it.
//
// Announcing is not the only way to find a machine. An address can be given
// instead, which is what works across subnets and where broadcast is filtered.
const (
	// BeaconPort is where announcements are sent and listened for.
	BeaconPort = 47270
	// BeaconEvery is how often a machine announces itself.
	BeaconEvery = 1500 * time.Millisecond
	// BeaconStaleAfter drops a machine that has missed about three
	// announcements, so a machine that was shut down leaves the list.
	BeaconStaleAfter = 5 * time.Second
	// maxBeacon caps what is read from the network.
	maxBeacon = 4 << 10
)

// Beacon is what a machine announces about itself. It carries what is needed to
// read that machine's document and nothing more: the authority itself is read
// from the machine, not taken from a packet anyone can send.
type Beacon struct {
	// ID is the fingerprint of the machine's authority, which is what the
	// machine is. A name and an address are attributes.
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Announcer broadcasts a machine's beacon.
type Announcer struct {
	// Port is where announcements are sent. Zero uses BeaconPort.
	Port int
	// To is the address announcements are sent to. Empty broadcasts to the
	// subnet.
	To string
	// Every is how often to announce. Zero uses BeaconEvery.
	Every time.Duration
	// OnError is called the first time an announcement cannot be sent. A
	// network that refuses them is not a reason to stop, because an address
	// given by hand still reaches this machine, but it should be said once.
	OnError func(error)
}

// Run announces until the context ends. now is asked for the beacon before each
// announcement and reports whether there is anything to announce, so a machine
// that closes its link stops announcing without being restarted.
func (a Announcer) Run(ctx context.Context, now func() (Beacon, bool)) error {
	port := a.Port
	if port == 0 {
		port = BeaconPort
	}
	to := a.To
	if to == "" {
		to = BroadcastAddress()
	}
	every := a.Every
	if every == 0 {
		every = BeaconEvery
	}
	// Sending to a broadcast address is refused unless the socket asks for it.
	lc := net.ListenConfig{Control: allowBroadcast}
	conn, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return err
	}
	defer conn.Close()
	target, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(to, strconv.Itoa(port)))
	if err != nil {
		return err
	}

	var reported bool
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if b, ok := now(); ok {
			payload, err := json.Marshal(b)
			if err == nil {
				_, err = conn.WriteTo(payload, target)
			}
			// A network that refuses the announcement is not a reason to stop:
			// an address given by hand still reaches this machine. It is said
			// once so it is not a silence.
			if err != nil && !reported {
				reported = true
				if a.OnError != nil {
					a.OnError(err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// BroadcastAddress returns the address an announcement is sent to: the one that
// reaches this machine's own subnet, or the address that reaches any subnet
// when this machine's own cannot be read.
func BroadcastAddress() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "255.255.255.255"
	}
	_, containers, _ := net.ParseCIDR("192.168.64.0/24")
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		if containers != nil && containers.Contains(ip) {
			continue
		}
		mask := n.Mask
		if len(mask) != net.IPv4len {
			continue
		}
		out := make(net.IP, net.IPv4len)
		for i := range out {
			out[i] = ip[i] | ^mask[i]
		}
		return out.String()
	}
	return "255.255.255.255"
}

// Listen reads announcements until the context ends, calling heard for each.
func Listen(ctx context.Context, port int, heard func(Beacon)) error {
	if port == 0 {
		port = BeaconPort
	}
	// The resident agent listens for these and so does a command asked to find
	// machines, so the port is shared. Without that the second one to start
	// fails and neither the agent nor the command can be relied on.
	lc := net.ListenConfig{Control: shareAddress}
	conn, err := lc.ListenPacket(ctx, "udp4", ":"+strconv.Itoa(port))
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, maxBeacon)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		var b Beacon
		if err := json.Unmarshal(buf[:n], &b); err != nil || b.ID == "" {
			continue
		}
		heard(b)
	}
}

// heardSet keeps the machines announcing right now.
type heardSet struct {
	mu    sync.Mutex
	stale time.Duration
	at    map[string]time.Time
	last  map[string]Beacon
}

func newHeard(stale time.Duration) *heardSet {
	if stale == 0 {
		stale = BeaconStaleAfter
	}
	return &heardSet{stale: stale, at: map[string]time.Time{}, last: map[string]Beacon{}}
}

// put records an announcement. A machine announcing again with another name or
// address updates the one entry: a machine is its identity.
func (h *heardSet) put(b Beacon, when time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.at[b.ID], h.last[b.ID] = when, b
}

// list returns the machines heard recently, ordered by name and then by
// identity so what is printed does not depend on arrival order.
func (h *heardSet) list(when time.Time) []Beacon {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Beacon
	for id, at := range h.at {
		if when.Sub(at) > h.stale {
			continue
		}
		out = append(out, h.last[id])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Discover listens for the given time and returns the machines heard.
func Discover(ctx context.Context, port int, d time.Duration) ([]Beacon, error) {
	h := newHeard(d + BeaconStaleAfter)
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	err := Listen(ctx, port, func(b Beacon) { h.put(b, time.Now()) })
	if err != nil {
		return nil, err
	}
	return h.list(time.Now()), nil
}

// Watcher keeps the machines announcing themselves right now. A window shows
// what it holds, so a machine that starts announcing appears without anything
// being asked for.
type Watcher struct{ heard *heardSet }

// Watch listens until the context ends and keeps what it hears.
func Watch(ctx context.Context, port int) *Watcher {
	w := &Watcher{heard: newHeard(BeaconStaleAfter)}
	go func() {
		// A machine that cannot listen is one that shows nothing, which is what
		// an address given by hand is for.
		_ = Listen(ctx, port, func(b Beacon) { w.heard.put(b, time.Now()) })
	}()
	return w
}

// List returns the machines heard recently.
func (w *Watcher) List() []Beacon {
	if w == nil {
		return nil
	}
	return w.heard.list(time.Now())
}
