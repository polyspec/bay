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

## Window

`baybar` shows one window with a source list: MACHINE holds Dashboard,
Domains, Certificates and Settings; PROJECTS holds one row per registered
project. Selecting a project lists its services under it, indented, so a service
can be opened without leaving the sidebar. There is no disclosure control: the
list follows the selection, so a mark would name an action that does not exist.

Screens:

| Screen | Subject |
| --- | --- |
| Dashboard | The whole machine: one verdict line, every project, every address the proxy serves |
| Project | One project: its services, its domain and its Compose file |
| Service | One service: its route, its container and the tail of its output |
| Domains | The domains delegated on this machine, and what delegating one writes |
| Certificates | The authority and what it issued |
| Settings | The application's own settings, the machine's, and the maintenance actions |

The dashboard reports three conditions the machine can be in. Setup is
incomplete, which is a warning and offers the setup action. The proxy is not
answering while containers run, which is a fault: the addresses stop being links
and each says why. Otherwise it states the number of domains served.

Sizes, colours and row heights come from `design/Mockups.dc.html`, which states
them in points. Two things differ from that file on purpose:

- A push button occupies less height in the layout than its bezel draws, so a
  row holding one is measured from its padding rather than from the design's row
  height.
- A service row is indented under its project instead of being marked with a
  disclosure triangle, and its status dot is drawn at half size.

## Language

The window is written in English and shown in Korean when the system prefers
Korean, read from `NSLocale.preferredLanguages`. English is the wording written
at the call site and Korean is looked up by it, so a line with no translation is
shown in English rather than left blank. `go test ./internal/i18n` reads the
window's sources and fails when a line has no Korean, when a translation takes
different values than the English it replaces, or when it uses a phrasing the
project does not use.

The command line stays in English: its output is the specification the generated
documents are produced from.
