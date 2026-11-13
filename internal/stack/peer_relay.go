package stack

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// The Apple proxy reaches a peer's link through a relay on the host.
//
// Apple `container` translates a container's address to the host's only for
// traffic that leaves through the interface the host's default route uses. A
// peer reached through another interface, such as the bridge of Internet
// Sharing, a second network adapter or a VPN, receives the container's own
// address, and its answer comes back to the host, which does not pass it to the
// container. The connection never completes.
//
// The relay listens on the gateway of the proxy's network, which only the host
// and its containers reach. It reads the name the proxy asks for in the TLS
// handshake, opens a connection from the host to the link of the approved peer
// that serves that name, and copies bytes in both directions. The host opens
// the connection, so it leaves with the host's address on any interface. The
// TLS session is the proxy's and the peer's: the relay reads the name and
// nothing else, and the peer checks the proxy's client certificate as it
// does without the relay.

// PeerRelayPort is where the relay listens on the gateway of the Apple proxy's
// network.
const PeerRelayPort = 8444

// relayHandshakeWait bounds how long a connection may take to send the start
// of its handshake, and relayDialWait how long the peer's link may take to
// accept one.
const (
	relayHandshakeWait = 10 * time.Second
	relayDialWait      = 5 * time.Second
)

// AppleNetworkGateway returns the gateway of one Apple `container` network,
// read from the network itself, so it is known before any container runs.
func AppleNetworkGateway(network string) (string, error) {
	out, err := runEngineWithin(engineReadWait, AppleEngine, "network", "inspect", network)
	if err != nil {
		return "", err
	}
	return decodeAppleGateway(out)
}

func decodeAppleGateway(out []byte) (string, error) {
	var raw []struct {
		Status struct {
			Gateway string `json:"ipv4Gateway"`
		} `json:"status"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", err
	}
	if len(raw) == 0 || net.ParseIP(raw[0].Status.Gateway) == nil {
		return "", errors.New("the network has no IPv4 gateway")
	}
	return raw[0].Status.Gateway, nil
}

// PeerRelayAddress returns where the Apple proxy reaches the relay, or an empty
// string when the gateway of its network cannot be read, which leaves the proxy
// to reach each peer directly.
func PeerRelayAddress() string {
	gateway, err := AppleNetworkGateway(ProxyNetwork)
	if err != nil {
		return ""
	}
	return net.JoinHostPort(gateway, strconv.Itoa(PeerRelayPort))
}

// RelayTarget returns the link of the approved peer that serves a name, or an
// empty string when no approved peer serves it.
func RelayTarget(m *Machine, name string) string {
	peers, err := Peers(m.Dir)
	if err != nil {
		return ""
	}
	if p, ok := PeerDomains(peers, nil)[strings.ToLower(strings.TrimSuffix(name, "."))]; ok {
		return p.Address
	}
	return ""
}

// ServeRelay accepts connections on ln until ctx ends. target returns the
// address a name is relayed to; a name it returns nothing for is refused by
// closing the connection.
func ServeRelay(ctx context.Context, ln net.Listener, target func(string) string, logf func(string, ...any)) {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			logf("relay: accepting a connection: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go relayOne(conn, target, logf)
	}
}

// relayOne reads the name a connection asks for, connects to the peer serving
// it and copies bytes until either side closes.
func relayOne(client net.Conn, target func(string) string, logf func(string, ...any)) {
	defer client.Close()
	client.SetReadDeadline(time.Now().Add(relayHandshakeWait))
	name, hello, err := readServerName(client)
	if err != nil {
		logf("relay: reading the requested name: %v", err)
		return
	}
	client.SetReadDeadline(time.Time{})
	address := target(name)
	if address == "" {
		logf("relay: no approved machine serves %s", name)
		return
	}
	upstream, err := net.DialTimeout("tcp", address, relayDialWait)
	if err != nil {
		logf("relay: connecting to %s for %s: %v", address, name, err)
		return
	}
	defer upstream.Close()
	if _, err := upstream.Write(hello); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, client); closeWrite(upstream); done <- struct{}{} }()
	go func() { io.Copy(client, upstream); closeWrite(client); done <- struct{}{} }()
	<-done
	<-done
}

// closeWrite ends the sending half of a connection, so the other side reads the
// end of the stream while its answer can still arrive.
func closeWrite(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		tc.CloseWrite()
		return
	}
	c.Close()
}

// readServerName reads the start of a TLS handshake and returns the name it
// asks for, with every byte read so far, which the caller sends on unchanged.
// The handshake is not answered: the hook that receives the name ends it.
func readServerName(conn net.Conn) (string, []byte, error) {
	var seen bytes.Buffer
	var name string
	stop := errors.New("name read")
	err := tls.Server(readOnlyConn{conn: conn, r: io.TeeReader(conn, &seen)}, &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			name = hello.ServerName
			return nil, stop
		},
	}).Handshake()
	if name == "" {
		if err == nil || errors.Is(err, stop) {
			err = errors.New("the handshake names no server")
		}
		return "", nil, err
	}
	return name, seen.Bytes(), nil
}

// readOnlyConn lets the TLS library read a connection without writing to it.
type readOnlyConn struct {
	conn net.Conn
	r    io.Reader
}

func (c readOnlyConn) Read(p []byte) (int, error)         { return c.r.Read(p) }
func (c readOnlyConn) Write([]byte) (int, error)          { return 0, io.ErrClosedPipe }
func (c readOnlyConn) Close() error                       { return nil }
func (c readOnlyConn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c readOnlyConn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c readOnlyConn) SetDeadline(t time.Time) error      { return nil }
func (c readOnlyConn) SetReadDeadline(t time.Time) error  { return nil }
func (c readOnlyConn) SetWriteDeadline(t time.Time) error { return nil }
