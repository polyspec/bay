# Architecture

[Korean](architecture.ko.md).

Status: implemented, except the peer link on a machine running both engines.
Rule 8 puts that link on a host process, which is not built; a machine with two
engines is refused when it opens the link rather than serving it from one of
them.

`bay` runs Compose projects on Apple `container` and on Docker, and
serves each service over HTTPS at a local domain name. No service port is
published.

## Components

| Component | Form | Count |
| --- | --- | --- |
| `bay` | Command line binary | Run on demand |
| `bayd` | DNS server, started by a launchd user agent | One per machine |
| `bay-edge` | nginx container that terminates TLS | One per engine in use |
| `baybar` | Menu bar application and window | One per user session |

Projects are Compose files. A project owns its service containers. The DNS
server, the certificate authority and the resolver entries are machine-level
and shared by every project. A proxy is shared by every project whose
containers run on the engine that proxy serves.

## Engines

An engine runs containers. Two are supported and neither is preferred: Apple
`container`, and Docker. A machine may have both, and one project's services
may run on either. An engine is identified by the command that speaks to it, so
anything answering the Docker socket is the Docker engine here.

An engine's command is found the same way by every program that uses it: the
variable that names it (`CONTAINER_BIN`, `DOCKER_BIN`), then the search path,
then the directories the engines' installers write to, `/opt/homebrew/bin` and
`/usr/local/bin`. The installed directories are searched because a program
macOS starts outside a terminal, the window from Finder or the DNS agent from
launchd, receives the search path `/usr/bin:/bin:/usr/sbin:/sbin` and finds
neither engine on it. The window launched from Finder reported that no engine
was installed while `container` answered from a terminal. The DNS agent's job
carries the same directories in its environment, taken from the same list.

An engine that is absent, or present with nothing answering its socket,
contributes no containers and is not an error. A machine with one engine
behaves as it did before the other was supported.

Routes, domain conflicts, certificates and peer state are read from every
engine's containers merged into one list. A domain is therefore claimed once
per machine, not once per engine, and two containers on different engines
claiming one domain is the same conflict as two on one engine.

## Rules

Each rule depends on a condition, and each rule after the first follows from the
rule above it. When a condition no longer holds, the rule is revised here before
any code is written against it.

1. A container is reachable on its own engine's network. Whether the host can
   reach that network is a property of the engine, not of the container.
   Condition: on macOS, Apple `container` places containers on a subnet the
   host has a route to, and Docker places them on a bridge the host has no route
   to. An engine that gives the host a route changes this rule and every rule
   below it.

2. Nothing depends on reaching a container from another engine. Such a
   connection fails.

3. One proxy per engine. A proxy serves the containers of its own engine and,
   by rule 2, no others.

4. Every proxy is reachable from the host, which is where the answers point. An
   Apple proxy already is. A Docker proxy publishes on 127.0.0.1, which is the
   only address macOS assigns to lo0: any other loopback address needs an alias
   added as root, and rule 9 leaves root to `/etc/resolver`.

5. A domain is claimed once per machine. Claims are resolved over the
   containers of every engine before any proxy is configured.

6. A name resolves to the address of the proxy for the engine its container runs
   on.

7. The machine publishes one port to the network, the peer link. Every other
   port is on loopback or on an engine's own network.

8. The peer link serves every domain this machine serves. By rule 2 no engine
   can do this, so the peer link runs as a host process that forwards to each
   engine's proxy.

9. Only `/etc/resolver` writes require administrator rights. The peer link
   binds port 8443, which requires no privilege, so rule 8 does not change
   this.

10. A name claimed by no container, and a peer's domain, belong to no engine.
    Every proxy is configured to serve them, so any proxy is a correct answer,
    and the answer names the first engine present.

## Request path

```
browser
  → /etc/resolver/<domain>            delegates the domain to 127.0.0.1:5354
  → bayd                      returns the address of the proxy for the
                                      engine that name's container runs on
  → bay-edge:443             selects a server block by Host header
  → <container>.container.test        resolved by that engine's DNS
  → service container:<port>
```

Plain HTTP on port 80 returns 308 to the HTTPS address, except
`/__bay/health`, which returns the configuration generation.

## Routing state

A proxy's configuration is generated from labels on running containers, not
from the Compose files. `bay` lists the containers of every engine
present, selects those labelled `bay.role=service`, and writes one
nginx server block per domain of each running container, into the
configuration of the proxy for that container's engine. A container carries the
domains of its service's `x-bay.domains` in the `bay.domain`
label, separated by commas; the label is empty for an internal service. Each
domain gets its own certificate. All server blocks of a container proxy to the
same backend.

Consequences:

- Removing a project's containers removes its routes. No file is edited.
- Two projects cannot overwrite each other's configuration.
- A service that is stopped has no route until it starts again.

## Container addressing

Each engine addresses its containers its own way, and the difference the rest
of this document rests on is whether the host can reach them.

Apple `container` gives a container an address on the `192.168.64.0/24` vmnet
subnet, routable from the host. Docker gives a container an address on a
user-defined bridge, routable only from that bridge. The host therefore reaches
an Apple proxy at the proxy's own address and a Docker proxy at a published
port, and `bayd` answers a name with whichever applies to that name's
engine.

A Docker proxy publishes 80 and 443 on 127.0.0.1, which is the only way the
host reaches it. Binding it to the loopback keeps the property vmnet gives for
free: what a service listens on is published on neither engine, and the one port
either proxy offers the network is the peer link.

Addresses are assigned by DHCP and change on every start; a fixed MAC address
does not hold an address.

Apple `container` answers DNS with a container's previous address for about
fifteen seconds after it is recreated. A route therefore sends the request to the
address the configuration was built from, taken from the runtime at that moment,
and does not depend on DNS for a service that was just started.

A container recreated by other means leaves the configuration holding an address
nothing answers on. Each route therefore carries the container's name as well: a
connection is given two seconds, and a route whose address fails is retried
against `<container>.container.test`, which the runtime resolves. The route
recovers on its own once the runtime's answer catches up.

Every response says which of the two answered, in `X-Bay-Route`:
`address` or `name`. A route that keeps answering through the name is one whose
address is out of date.

## Configuration reload

The generated HTTP configuration sets `server_names_hash_bucket_size 512`
for names longer than nginx's default server-name bucket can hold.
The value accounts for the name and the hash element's alignment and pointers;
it follows nginx's [power-of-two guidance](https://nginx.org/en/docs/http/server_names.html#optimization).
`TestRenderNginxLongNamesActualRuntime` checks the actual generated configuration
for a 61-byte routed name with the existing `ProxyImage` and `nginx -t`.
Its explicit command is
`BAY_SERVICE_E2E=1 go test ./internal/stack -run '^TestRenderNginxLongNamesActualRuntime$' -count=1 -v -timeout=120s`.
It requires the image to be present, creates its own container and temporary
certificates, and removes only the container whose ownership it verifies. It
does not register routes, change the shared proxy or DNS, or pull or remove images.

Reloading a proxy runs exactly one `exec bay-edge nginx -s reload` on
the engine that proxy runs on. If that command fails,
the caller receives its original error, including nginx stderr. A reload
failure must not stop, start, delete or recreate the proxy. Other projects use
the same proxy and must not be restarted as error recovery.

`nginx -s reload` returns after sending the signal. The workers being replaced
hold the listening sockets until they finish shutting down, and one of them can
accept a connection it then does not serve.

`bay` therefore computes a generation value from the route list and
each routed container's current IPv4 address and start timestamp. It writes
that value into the configuration and polls
`http://<proxy>/__bay/health` until the value is returned. A worker
serving the generation from before a backend restart cannot complete this wait.
Unchanged route and instance metadata retain the same generation. `up`, `down`,
`start`, `stop` and `restart` return only after the proxy serves the new
configuration.

A healthcheck runs inside the service's container and the readiness report
connects from the host, so neither shows that the proxy reaches the service.
Before `up`, `start` and `restart` give a running proxy the route of a service
they started, they run `nc -z -w 2 <address> <port>` inside the proxy
container, as a healthcheck runs inside a service's container, every 250
milliseconds until it succeeds, for at most 60 seconds, and report each wait
before it begins. The first connection
the proxy opens to the new container is then this attempt, not a request. When
no attempt succeeds, the command fails with the last attempt's error and the
proxy keeps its previous configuration. A proxy the command creates starts with
the route, and the command returns only after the same attempt succeeds.

## Certificates

A certificate authority is created in the state directory on first use and
added to the user's trust settings. Adding it does not require administrator
rights.

One leaf certificate is issued per routed domain, valid for one year and
reissued when fewer than 30 days remain. A default certificate covers every
delegated domain plus a wildcard, so a name with no route completes a
handshake and receives 404.

Certificate filenames append `.crt` or `.key` to the domain. A 253-byte domain
therefore exceeds a filesystem's 255-byte filename limit and is not supported
by the current certificate storage. The 61-byte configuration regression does
not verify TLS support for such names.

Clients reject a wildcard whose parent is a single label, so `*.test` does not
apply to `nope.test`. A domain with two labels, such as `dev.test`, gives
`*.dev.test`, which clients accept.

## Domains

A domain is delegated once per machine by a file in `/etc/resolver`. Domains are
therefore machine state, stored in `~/.bay/machine.json`. A project
uses the machine's default domain unless its Compose file names one under
`x-bay.domain`. The domains of a service must fall under the project's
domains.

The set of delegated domains is the machine's domains plus any domain a project
pins.

## Window

`baybar` shows one window with a source list: MACHINE holds Dashboard,
Domains, Certificates and Settings; PROJECTS holds one row per registered
project. An open project lists its services under it, indented, so a service
can be opened without leaving the sidebar. Each project is open or closed on its
own. Pressing a project that is not selected selects it and opens it; pressing
the selected project opens or closes it. Selecting a service opens its project.
A project stays open when another is selected, so several can be open at once.
The row itself is the control, and there is no separate disclosure mark.

The sidebar scrolls when its rows are taller than the window. A project with
twelve services pushed the rows below it out of the window with no way to reach
them.

Screens:

| Screen | Subject |
| --- | --- |
| Dashboard | The whole machine: one verdict line, every project, every address the proxy serves |
| Project | One project: its services, its domain and its Compose file, which opens in a text window |
| Service | One service: its route, its container, what it uses and the tail of its output |
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
  disclosure triangle, and its status dot is drawn at a third of the size.
- A key and value line runs its text together, so its spacing is zero and the
  controls at its trailing edge are set apart one by one.

A running service's screen shows what it uses, read from `container stats`:
CPU as a share of the cores it was given, memory against its limit, bytes read
from and written to disk, bytes received and sent, and its process count. CPU
is the change in the container's CPU time between two readings divided by the
time between them, so the first reading shows that it is measuring.

A service is one row in its project's list. A service serving several domains
lists them one under another inside that row, beside its name, state, address
and use, which are written once. A row for each domain read as several
services: with the name and the address repeated, and with the use on the first
row alone, the rows looked different when they were one container, and lines
of their own with nothing but a link still read as separate items. The row's first
line is the line a row with one domain draws, so its dot, name, address and
controls share one centre line; each further domain keeps the first one's left
edge, a fixed distance apart, and the space below the last is the space above
the first. Measured in the window, the first line sat on three centre lines up
to 7.5 points apart and the last domain touched the divider while the row was
aligned to its top.

A project's screen lists each running service's CPU and memory in its row, and
a service's screen shows them in full. One `container stats` call reads every
running container of the project in about two seconds, whether it names one
container or ten, so a project's containers are read together in one call, and
apart from the machine's refresh. Only the project whose screen is open is
read.

How much disk a container occupies is not shown, and the screen has no row for
it: a row that says a figure is missing tells the reader nothing. No
`container` command reports it per container. The figure exists only as the
size of a file inside the engine's own storage directory, whose layout the
engine does not publish, and reading it would break without notice when that
layout changes.
Docker reports its statistics in another form, and reading them is not
implemented; a Docker service's screen says so.

A control names what it does. Stop stops a project's services and leaves their
containers and the project's registration in place, so Start brings the project
back. Remove removes the containers and takes the project off the list. It asks
first, and says that each container's own filesystem goes with it while volumes
and the project's files are kept. The project's Stop sent `down`: pressing it on
`crudui` removed the container and the registration, and the project left the
sidebar with no Start to return it.

While an action runs, its screen shows each step as the action reaches it, the
same lines the command line prints. When it ends, the screen states what was
done to what: `Stopped crudui`, `Removed crudui`. A step's own line is not an
outcome; the window used to end a Stop on `proxy reloaded with 11 route(s)`.
When an action removes the subject of its screen, the window shows the
dashboard and the outcome moves there with it.

An action's outcome is shown on the screen the action was started from, and on
no other. A Stop pressed on one project reported its failure on every screen the
reader opened afterwards, so another project's service appeared to have failed.
The outcome stays on its screen until the next action, so returning to that
screen shows it again.

## Language

The window is written in English and shown in Korean when the language setting
asks for it. Settings offers System, English and Korean; System follows
`NSLocale.preferredLanguages`, and the choice is stored in the defaults database
under `language`. Changing it redraws the window without a restart. English is
the wording written at the call site and Korean is looked up by it, so a line
with no translation is shown in English rather than left blank. `go test ./internal/i18n` reads the
window's sources and fails when a line has no Korean, when a translation takes
different values than the English it replaces, or when it uses a phrasing the
project does not use.

The command line stays in English: its output is the specification the generated
documents are produced from.
