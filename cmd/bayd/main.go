// bayd resolves <name>.<domain> to the live IP of an Apple `container`
// container, so you can reach container port 80 directly without /etc/hosts and
// without publishing host ports.
//
// One-time setup (needs admin once):
//
//	sudo bayd -install -domain test
//
// Then just run it:
//
//	bayd -domain test
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// defaultState is where the machine keeps what it was set up with.
func defaultState() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".bay"
	}
	return filepath.Join(home, ".bay")
}

const (
	typeA    = 1
	typeAAAA = 28
	classIN  = 1
)

var (
	domain    = flag.String("domain", "test", "comma-separated local domains to serve, e.g. `test,lab.internal`")
	addr      = flag.String("addr", stack.DefaultDNSAddr, "UDP/TCP address to listen on")
	ttl       = flag.Uint("ttl", 5, "TTL in seconds advertised for answers")
	install   = flag.Bool("install", false, "write /etc/resolver/<domain> and exit; acquires root itself")
	privApply = flag.Bool("privileged-apply", false, "internal: apply the privileged setup steps")
	hosts     = flag.Bool("hosts", false, "keep the names the proxy serves in /etc/hosts (a machine that is not macOS)")
	relay     = flag.Bool("relay", false, "run the peer relay daemon for the state directory instead of the DNS server")
	state     = flag.String("state", defaultState(), "bay state directory, read to announce this machine and follow its peers")
)

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime)

	go exitWhenReplaced(context.Background())
	if *relay {
		serveRelay(context.Background(), *state)
		return
	}

	var doms []string
	for _, d := range strings.Split(strings.ToLower(*domain), ",") {
		if d = strings.Trim(strings.TrimSpace(d), "."); d != "" {
			doms = append(doms, d)
		}
	}
	if len(doms) == 0 {
		log.Fatal("-domain must name at least one domain")
	}

	if *privApply {
		if err := (stack.Install{Domains: doms, Addr: *addr}).Apply(); err != nil {
			log.Fatalf("install: %v", err)
		}
		return
	}
	if *install {
		in := stack.Install{Domains: doms, Addr: *addr}
		if err := in.Ensure(); err != nil {
			log.Fatalf("install: %v", err)
		}
		if pending := in.PendingText(); len(pending) > 0 {
			log.Fatalf("install did not complete: %v", pending)
		}
		for _, d := range doms {
			fmt.Printf("%s delegates *.%s to %s\n", stack.ResolverPath(d), d, *addr)
		}
		return
	}

	r := &resolver{domains: doms, list: stack.List}

	pc, err := net.ListenPacket("udp", *addr)
	if err != nil {
		log.Fatalf("listen udp: %v", err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen tcp: %v", err)
	}

	served := "*." + strings.Join(doms, ", *.")
	log.Printf("serving %s on %s -> the proxy of each name's engine", served, *addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(peerPollDelay, func() { servePeers(ctx, *state) })
	go serveJobs(ctx, *state)
	if *hosts {
		go keepHostsFile(ctx, stack.NewMachine(*state))
	}

	go serveTCP(ln, r)
	serveUDP(pc, r)
}

// --- resolution ---------------------------------------------------------

type resolver struct {
	domains []string
	// list returns the containers of every engine; stack.List outside tests.
	list func() ([]stack.Instance, error)

	mu     sync.Mutex
	cache  *index
	cached time.Time
	// refreshing is true while a new list runs in the background.
	refreshing bool
}

// index maps a served domain to the address of the proxy for the engine its
// container runs on. A container is reachable on its own engine's network and
// on no other, so only that engine's proxy can serve its domain.
type index struct {
	byDomain map[string]string
	// unclaimed answers a name no container holds. Every proxy serves such a
	// name, so any of them is a correct answer and the first engine is used.
	unclaimed string
}

// lookup returns the IPv4 address for a fully qualified name. The second result
// reports a temporary failure, which is answered with SERVFAIL: a resolver
// caches NXDOMAIN and the name would stay unresolvable after the proxy
// returns.
func (r *resolver) lookup(name string) (string, bool) {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	host, ok := r.strip(name)
	if !ok || host == "" {
		return "", false
	}
	idx, err := r.index()
	if err != nil {
		log.Printf("listing containers: %v", err)
		return "", true
	}
	if addr, ok := idx.byDomain[name]; ok {
		return addr, false
	}
	// A name no container holds is answered by a proxy all the same, so the
	// caller receives 404 from it rather than a DNS failure it would cache.
	return idx.unclaimed, idx.unclaimed == ""
}

// strip removes the local domain suffix and reports whether the name is served
// by this server.
func (r *resolver) strip(name string) (string, bool) {
	for _, d := range r.domains {
		if host, ok := strings.CutSuffix(name, "."+d); ok {
			return host, true
		}
	}
	return "", false
}

// index returns the last list of running containers (architecture rule 11).
// Only a query before the first list waits for a list; a list older than the
// TTL starts one new list in the background and the last list answers until
// the new one replaces it.
func (r *resolver) index() (*index, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		list, err := r.list()
		if err != nil {
			return nil, err
		}
		r.cache, r.cached = build(list), time.Now()
		return r.cache, nil
	}
	if time.Since(r.cached) >= time.Duration(*ttl)*time.Second && !r.refreshing {
		r.refreshing = true
		go r.refresh(r.list)
	}
	return r.cache, nil
}

// refresh runs one new list and replaces the last list with it. A failed list
// keeps the last list and is logged.
func (r *resolver) refresh(list func() ([]stack.Instance, error)) {
	started := time.Now()
	instances, err := list()
	var idx *index
	if err == nil {
		idx = build(instances)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshing = false
	if err != nil {
		log.Printf("listing containers: %v after %s; answering from the list of %s", err, time.Since(started).Round(time.Millisecond), r.cached.Format(time.TimeOnly))
		return
	}
	r.cache, r.cached = idx, time.Now()
}

// build maps the domains of the running containers to the proxy of their
// engine.
func build(list []stack.Instance) *index {
	// The address the host reaches a proxy at differs by engine, so it is read
	// from the proxy container of each one.
	proxies := map[string]string{}
	for _, in := range list {
		if in.Name == stack.ProxyName && in.State == "running" {
			proxies[in.Engine] = stack.ProxyHostAddr(in.Engine, in)
		}
	}
	idx := &index{byDomain: make(map[string]string, len(list))}
	for _, in := range list {
		if in.State != "running" {
			continue
		}
		addr := proxies[in.Engine]
		if addr == "" {
			continue
		}
		for _, d := range stack.LabelledDomains(in.Labels) {
			idx.byDomain[d] = addr
		}
	}
	for _, engine := range stack.Engines() {
		if addr := proxies[engine]; addr != "" {
			idx.unclaimed = addr
			break
		}
	}
	return idx
}

func serveUDP(pc net.PacketConn, r *resolver) {
	buf := make([]byte, 512)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			log.Printf("udp read: %v", err)
			continue
		}
		req := make([]byte, n)
		copy(req, buf[:n])
		go func() {
			if resp := r.respond(req); resp != nil {
				pc.WriteTo(resp, from)
			}
		}()
	}
}

func serveTCP(ln net.Listener, r *resolver) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("tcp accept: %v", err)
			continue
		}
		go func() {
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			var hdr [2]byte
			if _, err := readFull(conn, hdr[:]); err != nil {
				return
			}
			req := make([]byte, binary.BigEndian.Uint16(hdr[:]))
			if _, err := readFull(conn, req); err != nil {
				return
			}
			resp := r.respond(req)
			if resp == nil {
				return
			}
			var out [2]byte
			binary.BigEndian.PutUint16(out[:], uint16(len(resp)))
			conn.Write(append(out[:], resp...))
		}()
	}
}

func readFull(c net.Conn, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := c.Read(b[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// --- DNS wire format ----------------------------------------------------

// respond builds a reply for a single-question query. It returns A records for
// known names and an empty NOERROR for other served names, so the resolver does
// not query upstream.
func (r *resolver) respond(req []byte) []byte {
	if len(req) < 12 {
		return nil
	}
	if req[2]&0x80 != 0 { // already a response
		return nil
	}
	if binary.BigEndian.Uint16(req[4:6]) != 1 { // exactly one question
		return errorReply(req, 1) // FORMERR
	}
	name, off, err := parseName(req, 12)
	if err != nil || off+4 > len(req) {
		return errorReply(req, 1)
	}
	qtype := binary.BigEndian.Uint16(req[off : off+2])
	question := req[12 : off+4]

	resp := make([]byte, 0, 512)
	resp = append(resp, req[0], req[1]) // ID
	// QR=1, opcode copied, AA=1, RD copied; RA=0, RCODE=0.
	resp = append(resp, 0x80|(req[2]&0x78)|0x04|(req[2]&0x01), 0x00)
	resp = append(resp, 0, 1) // QDCOUNT

	ip, temporary := r.lookup(name)
	_, inDomain := r.strip(strings.TrimSuffix(name, "."))
	answer := ip != "" && qtype == typeA

	if answer {
		resp = append(resp, 0, 1) // ANCOUNT
	} else {
		resp = append(resp, 0, 0)
		// A served name that cannot be resolved returns NXDOMAIN. A served name
		// queried for AAAA returns an empty NOERROR, so the client falls back
		// to A.
		switch {
		case temporary:
			// SERVFAIL is not cached, so the name resolves again once the
			// proxy returns.
			resp[3] |= 2
		case !inDomain || ip == "":
			resp[3] |= 3 // NXDOMAIN
		}
	}
	resp = append(resp, 0, 0, 0, 0) // NSCOUNT, ARCOUNT
	resp = append(resp, question...)

	if answer {
		resp = append(resp, 0xC0, 0x0C) // pointer to the question name
		resp = binary.BigEndian.AppendUint16(resp, typeA)
		resp = binary.BigEndian.AppendUint16(resp, classIN)
		resp = binary.BigEndian.AppendUint32(resp, uint32(*ttl))
		resp = binary.BigEndian.AppendUint16(resp, 4)
		resp = append(resp, net.ParseIP(ip).To4()...)
		log.Printf("%s -> %s", name, ip)
	}
	return resp
}

func errorReply(req []byte, rcode byte) []byte {
	resp := make([]byte, 12)
	copy(resp, req[:12])
	resp[2] |= 0x80
	resp[3] = (resp[3] & 0xF0) | rcode
	binary.BigEndian.PutUint16(resp[4:6], 0)
	binary.BigEndian.PutUint16(resp[6:8], 0)
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0)
	return resp
}

// parseName decodes an uncompressed QNAME and returns it with the offset after
// it. Queries do not compress the question section.
func parseName(msg []byte, off int) (string, int, error) {
	var sb strings.Builder
	for {
		if off >= len(msg) {
			return "", 0, errors.New("truncated name")
		}
		n := int(msg[off])
		off++
		if n == 0 {
			return sb.String(), off, nil
		}
		if n&0xC0 != 0 {
			return "", 0, errors.New("compressed name in question")
		}
		if off+n > len(msg) {
			return "", 0, errors.New("truncated label")
		}
		sb.Write(msg[off : off+n])
		sb.WriteByte('.')
		off += n
	}
}
