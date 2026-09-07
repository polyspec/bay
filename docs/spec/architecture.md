# Architecture

Status: implemented.

`bay` runs Compose projects on Apple `container` and serves each
service over HTTPS at a local domain name. No host port is published.

## Components

| Component | Form | Count |
| --- | --- | --- |
| `bay` | Command line binary | Run on demand |
| `bayd` | DNS server, started by a launchd user agent | One per machine |
| `bay-edge` | nginx container that terminates TLS | One per machine |
| `baybar` | Menu bar application and window | One per user session |

Projects are Compose files. A project owns its service containers. The proxy,
the DNS server, the certificate authority and the resolver entries are
machine-level and shared by every project.

## Request path

```
browser
  → /etc/resolver/<domain>            delegates the domain to 127.0.0.1:5354
  → bayd                      returns the proxy container's address
  → bay-edge:443             selects a server block by Host header
  → <container>.container.test        resolved by the runtime DNS at the gateway
  → service container:<port>
```

Plain HTTP on port 80 returns 308 to the HTTPS address, except
`/__bay/health`, which returns the configuration generation.

## Routing state

The proxy configuration is generated from labels on running containers, not
from the Compose files. `bay` reads `container ls --format json`,
selects containers labelled `bay.role=service`, and writes one nginx
server block per container that carries a `bay.domain` label and is
running.

Consequences:

- Removing a project's containers removes its routes. No file is edited.
- Two projects cannot overwrite each other's configuration.
- A service that is stopped has no route until it starts again.

## Container addressing

Each container receives an address on the `192.168.64.0/24` vmnet subnet and is
routable from the host. Addresses are assigned by DHCP and change on every
start; a fixed MAC address does not hold an address.

The proxy therefore names backends instead of addressing them:
`proxy_pass` uses a variable and `resolver` points at the network gateway, so
nginx resolves `<container>.container.test` per request. A restarted container
is reached at its new address without a configuration change.

## Configuration reload

`nginx -s reload` returns after sending the signal. The workers being replaced
hold the listening sockets until they finish shutting down, and one of them can
accept a connection it then does not serve.

`bay` therefore computes a generation value from the route list, writes
it into the configuration, and polls `http://<proxy>/__bay/health`
until that value is returned. `up`, `down`, `start`, `stop` and `restart` return
only after the proxy serves the new configuration.

## Certificates

A certificate authority is created in the state directory on first use and
added to the user's trust settings. Adding it does not require administrator
rights.

One leaf certificate is issued per routed domain, valid for one year and
reissued when fewer than 30 days remain. A default certificate covers every
delegated domain plus a wildcard, so a name with no route completes a
handshake and receives 404.

Clients reject a wildcard whose parent is a single label, so `*.test` does not
apply to `nope.test`. A domain with two labels, such as `dev.test`, gives
`*.dev.test`, which clients accept.

## Domains

A domain is delegated once per machine by a file in `/etc/resolver`. Domains are
therefore machine state, stored in `~/.bay/machine.json`. A project
uses the machine's default domain unless its Compose file names one under
`x-bay.domain`.

The set of delegated domains is the machine's domains plus any domain a project
pins.
