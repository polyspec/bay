package main

import (
	"fmt"
	"os"

	"github.com/polyspec/bay/internal/stack"
)

func main() {
	c := stack.NginxConfig{
		Routes: []stack.Route{{Domain: "web.test", Address: "192.168.64.10:80",
			Backend: "g-web.container.test:80", Scheme: "http"}},
		Resolver:        "192.168.64.1",
		DefaultCert:     stack.DefaultCertName,
		Generation:      "gen1",
		PeerPort:        8443,
		PeerAuthorities: "/etc/nginx/peers/authorities.pem",
		ClientCert:      "/etc/nginx/peers/client.crt",
		ClientKey:       "/etc/nginx/peers/client.key",
		PeerRoutes: []stack.PeerRoute{{Domain: "api.test", Address: "192.168.0.99:8443",
			Authority: "/etc/nginx/peers/aa.crt"}},
	}
	if err := stack.RenderNginxConfig(os.Args[1], c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
