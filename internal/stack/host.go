package stack

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strings"
)

// A machine that is not macOS serves its names to peers only. The machine
// setup is the resolver entries under /etc/resolver, the certificate authority
// in the login keychain and the DNS agent under launchd, and all three are
// macOS's. Without them a browser on that machine does not resolve the names it
// serves, and a peer that approves it reaches them over its link: the peer
// answers the name with its own proxy and forwards the request there.

// hostSetsUpNames reports whether this machine delegates its domains to
// bayd and trusts its own authority. Tests replace it.
var hostSetsUpNames = runtime.GOOS == "darwin"

// HostSetsUpNames reports whether this machine has a machine setup. A machine
// without one serves its names to peers only.
func HostSetsUpNames() bool { return hostSetsUpNames }

// routeTable is the kernel's IPv4 routing table on Linux. Tests replace it.
var routeTable = "/proc/net/route"

// defaultRouteInterface returns the interface holding the default route, read
// from the kernel's routing table, or an empty string when there is none or the
// table cannot be read.
func defaultRouteInterface() string {
	f, err := os.Open(routeTable)
	if err != nil {
		return ""
	}
	defer f.Close()
	return defaultRouteIn(bufio.NewScanner(f))
}

// defaultRouteIn reads a routing table in the format of /proc/net/route: a
// header line, then one route per line whose first field is the interface and
// whose second is the destination in hexadecimal. The default route has the
// destination 00000000 and the mask 00000000.
func defaultRouteIn(s *bufio.Scanner) string {
	first := true
	for s.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(s.Text())
		if len(fields) < 8 {
			continue
		}
		if fields[1] == "00000000" && fields[7] == "00000000" {
			return fields[0]
		}
	}
	return ""
}

// interfaceIPv4 returns the first IPv4 address of a named interface.
func interfaceIPv4(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip := n.IP.To4(); ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				return ip.String()
			}
		}
	}
	return ""
}
