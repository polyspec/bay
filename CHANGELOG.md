# Changelog

Entries describe behavior changes and the verification run for each. Dates are
the day the change was made.

## 2026-09-10

### Say when the proxy did not take the link record

The switch wrote the setting, drew itself on, and applied the configuration
behind the screen without reading the result. When that failed the switch still
read as on while the proxy ran the configuration it had before, so the record
was asked for and nothing was written, with nothing said about it.

The result is read. When the proxy does not take the record the setting goes
back to what the proxy is running, the switch follows it, and the failure is
reported on the screen.

Verification: `make check` passes. With the record on, a request over the link
recorded `link 200 host=polyspec.test uri=/comparison.css declared=3905
received=4130 sent=3893`.


### Answer the link record switch without redrawing the screen

Turning the record on ran as an action: the screen was drawn once with every
button disabled, the proxy configuration was written and re-read, and the screen
was drawn again from a fresh reading of the machine. Nothing on it had changed
but the switch.

The switch now answers on the screen at once and the configuration is written
and re-read behind it, so nothing else moves. The proxy reads the record from
its configuration and there is no way to change that without re-reading it, so
that part remains.

Verification: `make check` passes.


### Record what crosses a peer's link, when the machine asks

A response cut short by the other machine is answered 200, so nothing in the
proxy's output says how much of it reached the client. The cause of one such
response could not be established: the same request arrived whole 60 times in a
row after a change and short 1 time in 20 before it, and the rate moved between
measurements, so the change is not proven to be the cause.

Settings holds a switch that records one line per request over the link with the
length the other machine sent, the number of bytes received from it, the length
that reached the client, and the time to connect, to the first header and to the
end of the response. A short send is then a number in the record rather than a
report that something felt wrong. It is off until it is turned on, because it
writes a line for every request over the link.

`internal/stack/peer_body_runtime_e2e_test.go` stands up both ends of a link,
each running this program's own configuration, and asks for a body larger than
the proxy's buffers five times. It does not reproduce the fault, so it guards
the behaviour and does not explain it.

Verification: with the switch on, a request over the link recorded
`link 200 host=polyspec.test declared=327568 received=327800 sent=327197`, where
the difference between what was declared and what was sent is the chunk framing
of a whole response. `make check` passes and the runtime test passes with
`BAY_E2E=1`.


### Do not reuse a TLS session on a route to a peer

A route to a peer presents a client certificate. Reusing a TLS session on such a
connection made the peer end the response before the body was complete, and the
proxy answered 200 with what it had received. A page loaded its markup and none
of its scripts.

An earlier change removed `Connection: close` from the same request and the
truncation stopped for as long as it was measured. It returned. That header was
sent in error and its removal stands, but it was not the cause.

Verification: measured against the peer's link. With the session reused the same
327197-byte file arrived as 163431, 179799 and 163447 bytes on successive tries,
answered 200 each time, while a request made directly to the peer arrived whole
on every try. With the session not reused it arrived whole five times in a row
and the proxy logged no upstream error. `make check` passes.

## 2026-09-09

### Keep the tool that measures the window

The window's spacing was measured with a program written for one session and
left in a temporary directory, which is removed when the session ends. The next
change to the layout would have been judged by eye again.

`tools/window.swift` captures the window by its identifier, so the window does
not have to be in front, and reports each run of pixels holding type with the
distance from one run's centre to the next. `make window` runs it and `AGENTS.md`
states how.

Verification: `make window OUT=/tmp/w.png REGION="262 895 205 390"` captured the
running window and reported the peer card's rows 23.0, 24.5, 24.0 and 24.0
apart, and 35.0 across the boundary that carries a rule. `make check` passes.
### Space a card's rows by one padding, measured on screen

The peer card set a different space at every boundary. The rows measured 23, 32,
27 and 24 points from one row's centre to the next, and the space between the
address and the fingerprint was more than twice the space at the card's edge.

A card that names its parts now states its padding once and derives the rest.
Two rows with no rule between them are one padding apart, which is also the
space at the card's edge. A rule marks where a part begins, so the boundary that
carries one takes a padding on each side of it. Every row in such a card is held
to one line of type plus the padding, because the stacks inside a key and value
row reserve more height than the type needs and left that boundary wider than
the others whatever the padding was set to.

A card that is a plain list is unchanged. Every boundary there carries a rule,
and the rows keep the padding they had.

Verification: measured on screen. The window was opened, captured by its window
identifier and the rows of pixels holding type were read off. Before: centres 23,
32, 27, 24 apart. After: 24 apart at every boundary with no rule, and 35 across
the one that carries a rule, which is one padding and the rule wider. `make
check` passes.
### Draw a rule only where a card's part begins

The window drew a rule between every pair of rows in a card. A card made of a
title, two values and a list read as one undifferentiated list.

A card whose rows name its parts draws a rule only where a part begins, which is
a label row. A card that is a plain list still draws one between every row,
because there the rows are the items.

The line under a peer's fingerprint explaining what a fingerprint is has been
removed. It described the design rather than the machine.

Verification: `make check` passes. The window was not seen: it is a menu bar
application and does not come forward from a script.
### A proxy that went away is started, not reported as a reload failure

`bay peer add` stored an approval and then failed with `container exec
bay-edge nginx -s reload: Error: container with ID bay-edge
not found`. The proxy is read from the container list before the configuration
is written and the reload is sent after, so a proxy removed in between is
reported as missing by an engine that had just listed it running. The machine
was left with an approval stored, a configuration written, and a command
reporting failure for a change it had already made.

The rule that a reload failure must not restart the proxy stands: it protects a
running proxy other projects share. A proxy that is not there is neither
running nor shared, so that case starts it and the sync reports it as started.
A configuration the proxy refuses still fails, because replacing the proxy
there would hide the fault behind a container that starts and fails the same
way.

Verification: `make check`, including tests over each engine's wording for a
container that is not there, a proxy removed between the container list and the
reload, and a configuration the proxy refuses. Without the change the second
fails with the message quoted above.

### Show each approved machine and its domains as one section

The screen named each approved machine and did not state which domains that
machine provides, although the command line prints them. With several approved
machines the reader could not tell which machine provides which domain.

One machine is one card. The card's first row states the machine and carries the
remove action, the next two state the address and the fingerprint, and a label
row states how many domains follow. The rows are the domains that
machine provides, and each row opens its address. A machine that provides no
domain states that.

A list holding both machines and domains states the machine of a domain in a
column, which is read as one more attribute of the row. The section states it
instead. The remove action belongs to the machine and the open action belongs to
the domain, and each is now on the thing it acts on.

A domain row states the domain and opens it when clicked. The address was
written beside the name as well, which states the same thing twice.

The address and the fingerprint were first placed in the section's `detail`
field, which the window does not read, so both were dropped and the screen named
the machine and stated nothing else about it. The field was read nowhere and is
removed.

The window draws two more kinds of row: `title` names what a card stands for and
carries the actions on it, and `label` names the rows under it.

Verification: `go test ./cmd/baybar -run 'TestThePeersScreen|TestAPeerSection'`
covers two approved machines with three domains between them, checks that each
domain is inside the section of the machine that provides it, and checks that the
address and the fingerprint are in rows the window draws. `make check` passes.
### Include peer domains when deciding whether a certificate is used

This machine serves an approved peer's domain with a certificate it issued. The
certificates screen decided whether a name was used from the routes and the
projects only, so it reported those certificates as unused and offered to remove
them. Removing one removes the certificate required to serve the name.

The domains of approved peers are now included. A name required by no route, no
project and no peer is still reported as unused.

Verification: `bay status --json` reported polyspec.test and
registry.soksak.test as orphaned while both were served. After the change
neither is reported as orphaned, and a name nothing requires still is.
`make check` passes.

### Report an untrusted certificate authority

Setup reported no remaining steps while no keychain contained the authority, so
a browser rejected every name this machine served and no message stated why.
Two defects caused this.

The check verified the authority against itself. A self-signed certificate is
its own root, so the check succeeded without any keychain and returned true for
an authority created moments earlier. The check now tests whether a keychain
contains the certificate, comparing the certificate bytes rather than the
subject name, because a replaced authority has the same subject name as its
replacement.

The command that adds the authority named no keychain. Without a named keychain
it exits zero and adds nothing. It now names the login keychain, and both adding
and removing read the result back instead of relying on the exit status.

Verification: no keychain contained the authority and a certificate it issued
failed `security verify-cert -p ssl`, while setup reported no pending steps.
After the change setup reported the trust step, ran it, and the certificate
verified. Two calls return the same result, and a second run of setup reports no
remaining steps. `make check` passes.

### Remove Connection: close from a proxied request

A request that is not a protocol upgrade sent `Connection: close` to the
upstream, and the upstream ended the response before the body was complete. The
proxy returned status 200 with a body shorter than its content length, so a
browser reported a protocol error for a request it was told had succeeded, and a
page loaded its markup and none of its scripts.

An upgrade request still sends `Connection: upgrade`. Every other request sends
no Connection header, which nginx omits when the mapped value is empty.

Verification: measured against a peer link. A 327197-byte file was received in
full four times after the change. Before the change the same request returned
163440, 212536, 310744 and 114351 bytes on successive tries, with status 200 each
time. Each of the four headers this proxy adds was sent alone; only
`Connection: close` reproduced the truncation. `make check` passes.

### Publish the Docker proxy on 127.0.0.1

The Docker proxy published on 127.0.0.2. macOS assigns no such address to lo0,
so Docker refused to create the container and opening the peer link failed.
Adding that address requires root, and root is used only for `/etc/resolver`.

Two further defects stopped the same command. A configuration with no route of
its own wrote a resolver directive with no address, and nginx fails to start on
one. A machine with the peer link open and no route stopped its proxy instead of
running it, so nothing served the document used to approve the machine. The
resolver directive is written only when there is an address, and the proxy that
publishes the link runs with no route.

The Docker proxy uses Docker's resolver address instead of the network gateway,
because Docker does not answer on the gateway.

Verification: a test binds the published address, which reproduces the first
defect. `bay peer open` reported the link open at 192.168.0.10:8443 and
the proxy running with 80 and 443 on 127.0.0.1 and 8443 on the network. A request
to the network address returned the document with this machine's name, address
and authority. `make check` passes.

### Refuse the peer link before storing the setting

The proxy configuration refuses the peer link on a machine running both engines.
`peer open` and the window's button stored the setting first and met the refusal
after, so a machine that was refused kept the link recorded as open and every
later configuration write failed for the same reason, with no way to recover but
editing the settings file.

The refusal is one function, called by the setting and by the configuration, and
opening calls it before writing. Closing is never refused, so a machine that
gained a second engine while the link was open can still close it.

Verification: `make check`, including tests over one engine, both engines, no
engine, and closing after a second engine appeared. Without the change two of
them fail: opening with two engines is allowed and leaves the setting on.

### Show peers in the window

The window lists the machines this machine reaches domains through. The screen
shows the link address, the announcement interval, the approved machines with
their domains, and the machines announcing on this network. The link is opened
and closed from the same screen.

Verification: the link was opened and closed from the window, and the command
line reported the same state.

### Support Apple container and Docker together

A machine runs Apple `container` and Docker at the same time and serves both.
Neither is preferred. `bay status` now runs on a machine that has one
engine's command and not the other's; before this change it did not start.

Containers are read from every engine present and merged into one list, each
recording its engine. An engine that is absent, or whose daemon does not
respond, returns no containers and does not cause an error.

Each engine runs its own proxy, because a container is reachable only on its own
engine's network. The host reaches an Apple proxy at the proxy's own address and
a Docker proxy at 80 and 443 published on 127.0.0.1. A service's own port is
published on neither. `bayd` returns the address of the proxy for the
engine of the requested name.

One domain is claimed once per machine. The claim is resolved over the
containers of every engine before any proxy is configured.

The DNS server's per-service mode is removed. It returned the container's own
address, which requires a network the host has a route to, so it could not serve
a Docker container. Its `-proxy` and `-aliases` flags are removed with it, and
the agent is registered without them.

`docs/spec/architecture.md` states ten numbered rules with the condition each
depends on. `AGENTS.md` states where the code implements each rule and which one
it does not.

The peer link is not moved. It uses one port, and rule 8 places it in a host
process that reaches both engines. Until that process exists, opening the link
on a machine with two engines returns an error instead of serving it from one
engine and omitting the other engine's domains.

Verification: `make check` passes and `go test -race ./internal/stack` passes in
4.430 seconds. The unit tests cover parsing `docker inspect`, selecting one
address for a container on several networks, an engine that does not respond, no
engine present, and one domain claimed across engines. Against a running Docker
daemon, `stack.List` read this machine's five containers, each recording its
engine, the three running ones with their bridge addresses and the two exited
ones with none. The field names `docker inspect` returns were compared with the
parser. Nothing was verified on Apple `container`, which this machine does not
have.

### Two ways to find a machine, and one way to prove it

A machine with the link open announces itself to the network every second and a
half; `bay peer find` lists what it hears and `peer add <name>` approves
one by the name it announced. An address given by hand is the other way, and it
is what works across subnets and where a network filters broadcast. Neither
replaces the other. What is announced is only where to look: the authority is
read from the machine itself, so a machine is still proved by its certificate
and by nothing else.

The announcement is a broadcast rather than a multicast, because access points
commonly prune multicast between clients while still flooding broadcast to the
subnet.

The resident agent follows a peer that comes back at another address, which is
what happens when the network hands out a different one. The announcement is
matched by the peer's authority fingerprint and only the address is taken from
it.

Two faults were found on the way. The agent registered with launchd was reported
current whatever program it ran, so one registered from a copy that had been
moved kept being accepted; the program is part of it now, and re-registering
waits for the job launchd is still holding to go before it registers the new
one. Both were reached on this machine: the agent had been running from a
scratch build for a day.

Verification: `make check`, including tests over hearing an announcement, a
closed link announcing nothing, a machine ageing out, and one announcing twice.
On this machine `peer find` listed its own announcement and `peer add max`
approved it by name; the link then served a name to an approved certificate,
200, and `peer remove` and `peer close` left the machine's own sites answering.

### bay peer

Two machines on one network, each running bay, reach each other's
domains. Neither machine's network configuration changes.

`bay peer open` answers a link on a published port and prints the
address to give the other machine. `peer add <address>` reads what that machine
says about itself, checks that it holds the authority it names, prints the
fingerprint and asks before approving. `peer` lists what is approved and whether
the link is open. `peer remove` withdraws an approval and `peer close` stops
answering.

A peer is what its authority is: it is stored under that authority's
fingerprint, and its name and address are attributes that change without making
it a different peer.

A published port does not keep the source address, so a machine is not
recognised by one. It presents a certificate its own authority issued, and a
link serves only a holder of one from an approved authority. Nothing is added to
the keychain: an approved authority is a file the proxy reads.

A peer's domain is answered here, with a certificate this machine issues from
its own authority, and forwarded over that peer's link. A browser is therefore
offered a certificate from the authority its own machine already trusts. A name
this machine serves is always this machine's.

Verification: `make check`. On this machine the link was opened, the document
answered with the name, the address, the domains and the authority, and `peer
add` approved it after checking the certificate against the authority it named.
A served name on the link was then refused without a client certificate, 400,
and served with one, 200. `peer remove` and `peer close` released the port and
left the machine's own sites answering 200.

### Unused certificates are their own list

A certificate something asks for and one left behind are answered differently:
one is reissued, the other is removed. The certificates screen shows them as two
lists. UNUSED carries `Remove all N` in its header beside `Remove` on each row,
and asks before removing, because a removed certificate is gone.

What the bulk action removes is read from the machine at the moment it runs, not
carried through the question, so a name that came into use while the question
was open is kept.

Verification: `make check`, including a test over what the bulk action selects.
On this machine the header read `전체 17개 제거`, the question was read on
screen and cancelled, and the twenty-four certificates are unchanged.

### A certificate is unused only when nothing asks for its name

A certificate was called unused when no route served its name. Whether a project
runs is not what makes its certificate unused: a stopped project serves nothing,
so all of its certificates were reported as unused and offered for removal,
which is what its next start needs. A certificate is unused now only when
neither a route nor a registered project asks for its name, which is what a
project whose files are gone leaves behind.

A project whose file is still there but could not be read asks for something
that is not known. Nothing is called unused while that is true, because the
alternative is a button offering to remove a certificate the project needs.

Verification: `make check`, including tests over a declared domain with no
route, a served domain, case, and a project whose file is present but
unreadable. On this machine, with every project stopped, the list called
`api.platform3.test` unused before the change; it now names the project that
declares it, and only the fourteen names no project asks for stay flagged.

### A long name in a list keeps what tells it apart

The certificate list gives each name a fixed column. Names that share a prefix
and differ in a hash all read the same in it: twenty-four certificates, fourteen
of them shown as `console.platform-nat…`. A name is what tells one row from
another, so it now takes the width the row does not otherwise need, and what
does not fit is dropped from the middle rather than the end. The short phrase
beside it keeps its own width instead of being squeezed out.

Verification: the list was read on screen at a window width where the names
still do not fit whole. Each row reads `console.plat…605595.test`, and every row
carries `사용하는 경로 없음` in full.

## 2026-09-08

### bay sync

The proxy configuration is written by every command that starts or stops a
container, and by nothing else. A change to how it is written therefore could
not reach a machine whose containers were already running without restarting
them. `bay sync` rewrites it from the containers that are running,
issues any certificate a route needs, reloads the proxy and waits until it
serves the new configuration. It starts, stops and changes no container.

The window carries the same action under Settings, MAINTENANCE. It also logs
every outcome now, not only the failures: what the window shows is otherwise
gone as soon as the next action replaces it, and there was no way to read back
what an action reported.
`docs/operations/troubleshooting.md` names the symptom it answers, and how to
read `X-Bay-Route` to tell which of a route's two targets replied.

Verification: run on this machine it rewrote five routes and reloaded the proxy;
every domain then answered 200 in under 45 ms and reported
`x-bay-route: address`. From the window it rewrote
`~/.bay/conf.d/stack.conf`, which the file's modification time confirms,
and the window and the log both reported `프록시 설정을 다시 썼습니다 · 경로 5개`.

### A route is sent to the address it was built from

A report said the proxy connected to an address the recreated container no
longer held, and the request timed out. Measuring the runtime's DNS shows why:
after a container is recreated it answers with the previous address for about
fifteen seconds. The proxy named its backends and resolved them per request, so
a service that had just been started was sent to the address it held before.

A route now carries the address taken from the runtime when the configuration
was written, and the request goes there. The container's name is kept as
the second target: a connection is given two seconds instead of the minute nginx
waits by default, and a route whose address stops answering is retried against
the name, which recovers a container recreated by other means. Every
response says which answered, in `X-Bay-Route`.

Verification: `make check` covers the configuration, including that every
`proxy_pass` is bounded and that a route with an address carries the name to
fall back to. Against the runtime,
`BAY_E2E=1 go test ./internal/stack -run TestRecreatedServiceIsReachedAtOnceActualRuntime`
recreates a container, writes the configuration for it and measures one request:
answered through the address in 21 ms. The same test against the previous
design, which named the container, returned 504 after 1 m 0.027 s.
`-run TestRecreatedBehindBayRecoversActualRuntime` recreates a container
behind bay and measures recovery through the name at 2 s, with no
request taking longer than 2.024 s. Both run their own nginx, so the projects on
the machine keep serving.

### Verify the current backend instance after restart

Proxy configuration generations include the routed container's current IPv4 address and start timestamp. Restart commands wait for a worker serving that instance's generation. Named backends, DNS cache duration and lifecycle time limits are unchanged.

`TestBackendRestartWaitsForCurrentProxyGeneration` reproduces premature completion after an address or start-time change in 0.949 seconds. The corrected integration and reload regressions pass with race detection in 3.223 seconds. The full stack race suite passes in 3.085 seconds and `make check` passes. Installation and actual consumer browser verification remain pending.

### Size nginx's server-name bucket for long route names

`RenderNginx` sets `server_names_hash_bucket_size 512` in the generated HTTP
configuration. Route names and reload behavior are unchanged. Certificate
storage still cannot create the filenames required by a 253-byte domain.

Verification: before the correction, the isolated
`TestRenderNginxLongNamesActualRuntime` ran the existing proxy image's actual
`nginx -t` against generated configuration for a 61-byte name and failed with
`server_names_hash_bucket_size: 64` (test 0.990 seconds, package 1.317 seconds).
With bucket 512, the unchanged test and image pass (test 1.010 seconds,
61-byte subtest 0.930 seconds, package 1.309 seconds). The test owns its container
and temporary certificates and does not change the shared proxy, DNS or images.
With native tests disabled, `make check` passes and
`go test -race ./internal/stack -count=1 -timeout=120s` passes in 2.200 seconds.
The installed binaries record clean source `9823d0e` after
`make -B install PREFIX=/opt/homebrew`; `doctor` reports `nothing to do`.
Application runtime and browser verification are recorded by each consumer.

### Proxy reload failures retain the original error

`ReloadProxy` invokes `container exec bay-edge nginx -s reload` once.
A failed command returns its original error, including nginx stderr, without
stopping, starting, deleting or recreating the shared proxy.

Verification: `TestReloadProxyOnlyExecutesReload` first reproduced a swallowed
error and unexpected stop/start calls through a fake CLI. The corrected function
passes the success and failure cases with race detection. `make check` passes,
and `go test -race ./internal/stack -count=1 -timeout=120s` passes in 1.958 seconds
with native tests disabled. No installation or actual container lifecycle
command was run for this change.

### Status and doctor read public state without creating or registering it

`status` no longer registers or refreshes the selected Compose project. `doctor`
and the command/window snapshot read public certificates without creating a CA
or opening its private key. Missing and unreadable public certificates are
reported, including `certificates.authority.readError` in JSON. `doctor` still
includes the selected project's domains when describing setup, without saving
them. `up` and explicit installation retain their setup behavior.

Verification: isolated CLI regressions reproduced state creation and failure on
a malformed private key before the correction. They now preserve absent state
and existing registration bytes, modes and modification times, report malformed
public certificates, and allow public queries with an absent or malformed key.
`make check` and `go test -race ./cmd/bay ./internal/stack
./internal/contract -count=1 -timeout=120s` passed with native tests disabled.
No installation or container lifecycle operation was run for this change.

### The window is built from the design file, and reads in English or Korean

`design/Mockups.dc.html` holds ten screens of the window and is the source of its
sizes and colours. It was rendered and measured, and the window now takes those
numbers: a sidebar row of 27 points, a group header of 29, a card row of 42 with
a button and 35 without, a 9 point status dot with a 4 point halo on the verdict,
a chip drawn as a border with no fill, and two separate weights for a card's
outline and the hairline between its rows.

Three screens the window did not have are implemented. Service shows one
service's route, its container and the tail of its output. Settings holds the
application's own settings, the machine's, and the maintenance actions. The
dashboard reports a proxy that is not answering while containers run as a fault,
with the addresses no longer offered as links. Adding a domain is asked on a
sheet attached to the window, which shows the resulting name while it is typed.
`Add project…` registers a Compose file chosen from the file panel.

Selecting a project lists its services under it, indented, at the same row height
as the project. There is no disclosure control, because the list follows the
selection. Every row carries a status dot, so a stopped service keeps its place
in the column.

Settings offers the language: System, English or Korean. System follows the
locale the system prefers. English is the wording written at the call site and
Korean is looked up by it, so a line with no translation is shown in English.
Changing the choice redraws the window without a restart. The command line stays
in English, because its output is the specification the generated documents come
from.

The resolver files are named as a set, `/etc/resolver/{devel,staging,test}`,
because a list of whole paths did not fit the row and was truncated in the
middle, which hid a name.

The sidebar and the pane meet on a line, drawn at the weight a card is outlined
with. The project screen opens its Compose file in the text window, read from
the first line, so the file the project is edited in can be read without leaving
the window.

Three layout faults are corrected. A row aligned to the top did not grow to hold
a two-line value, which cut the second line off at the card's edge; the padding
under the value is now stated. A line in the output pane was as wide as the pane
rather than its content, which covered the padding on both sides. A key and value
line runs its text together, so its spacing is zero, which left two buttons at
its trailing edge touching; the controls there are now set apart one by one.

Verification: `make check` passes, including `go test ./internal/i18n`, which
reads the window's sources and fails on a line with no Korean, a translation
taking different values than the English it replaces, or a rejected phrasing. The
design file was rendered in a browser and measured; the built window was captured
with `screencapture -l <window>` and measured against the same numbers, with the
sidebar row pitch at 29 points for every row. Every screen was opened and
captured: dashboard, project, service, domains, certificates, settings, the add
domain sheet, and the first run and proxy down states.

### Local image creation verifies the selected digest before starting

Locally available tags need not have equivalent name@digest aliases. Services now use the declared reference for `create`, verify the stopped container's actual image, ownership and configuration, then start that verified container. Initializers use `start --attach` and still require real exit zero. A tag change during creation is rejected before a process starts. Creation errors identify their stage and a bounded category without printing runtime arguments or process output.

Verification: the real local-tag fixture failed before the correction and passes with unchanged reuse, nonzero exit rejection, redacted creation failures and a public stdout marker retained in native logs. Unit tests reject altered created-container image/owner/config and bound diagnostic storage. Existing service, readiness and volume regressions remain required; binaries are not installed by verification. Applications must keep values forbidden in logs out of their process output.

### Compose declares and preserves managed named volumes

Managed `volumes` declarations create missing local volumes with optional `driver_opts.size`. Existing volumes must match project/declaration ownership and exact configured size; another owner's data, implicit resizing and unsupported drivers/options are refused. `down` preserves volumes. External volumes still require existence and are never created or relabelled.

Verification: `make check`, `go test -race ./internal/stack ./internal/contract`, and the tracked `BAY_SERVICE_E2E=1 go test -race ./internal/stack -run TestManagedVolumeActualRuntime -count=1 -timeout=120s` exercise creation, reuse, data preservation after service removal, foreign-owner rejection and resize rejection using only a unique fixture volume/container. Existing containers and shared proxy are preserved; binaries are not installed.


### Compose startup preserves owned services and waits for dependencies

Compose values now resolve project `.env` and shell interpolation. Relative bind paths use the Compose directory; external named volumes are checked before startup. `user`, `read_only` and `cap_drop` are passed to the runtime. Dependency cycles, missing services and unsupported lifecycle options are rejected.

`up` reuses unchanged configuration and local image digests, preserving a running database. Containers with another project's or missing ownership labels are refused before changes. Startup waits for declared healthchecks and actual successful one-shot exits. Private completion records identify the stopped initializer by configuration, image and runtime creation/start timestamps. Explicit restart retries initialization; missing or changed evidence never means success. Project mutations use an advisory lock and teardown respects dependency order without removing volumes.

Verification: `make check`, `go test -race ./internal/stack ./internal/contract`, and `BAY_SERVICE_E2E=1 go test ./internal/stack -run TestServiceLifecycleActualRuntime -count=1 -timeout=120s`. The isolated service test checks actual process restrictions, initialization, failure and unchanged reuse while preserving every preexisting container. It does not register routes, replace the shared proxy, run an application database or install binaries. Runtime identity limits and supported syntax are stated in [the lifecycle specification](docs/spec/service-lifecycle.md).

TCP readiness reporting remains after route synchronization. Completed initializers are excluded, and an empty selection neither probes nor reports connection success. Both cases failed before correction and pass in tracked lifecycle tests. The existing shared-proxy readiness test remains unchanged and skips while that proxy is in use; `TestReadinessActualRuntime` verifies both snapshot paths, delayed listening and WaitReady using an isolated service.

## 2026-09-07

### Starting is reported separately from running

A container reports as running before the process inside listens on its port.
Requests during that window fail: the proxy returns 502 and a connection from
another service times out. The tool reported such a service as running.

`status`, the snapshot and the application now report it as `starting`, and
`up`, `start` and `restart` wait up to 20 seconds for the processes to accept
connections, naming any service that takes longer.

Verification: `BAY_E2E=1 go test ./internal/stack -run
TestStartingIsDistinctFromRunning`. A project with a service that listens after
25 seconds reported `still starting after 20s`, and `status` showed the service
as `starting` while the proxy returned 502 for it.

### Usage document rewritten and executed

`docs/operations/using.md` and its Korean twin now cover checking the machine,
adding a project, how a service's name is chosen, services with no domain,
service-to-service addressing, port resolution, domain management from the
command line, the daily commands, and the path a request takes.

Every claim was executed against a project created from the document. One
behavior the document did not state was found and added: a restarted service
takes a few seconds to become reachable by name from other services, because
the runtime publishes the new address after the container is running.

Verification: `bay up`, `stop`, `start`, `status --json`, the HTTP
redirect, service-to-service access by name, and the refusal of a domain
another project serves were each run against that project.

### Domains can be managed from the command line

`bay domain` lists the machine's domains, and `add`, `remove` and
`default` change them. Until now domains could only be changed in the
application, so a caller with the command line could not set one up.

A failed apply restores the settings file, so a refused authorization no longer
leaves a domain recorded but not delegated.

`bay brief` now carries the first steps, what `up` does, a complete
Compose example and the domain commands, so one command covers the whole
contract.

Verification: `bay domain` lists the delegated domains; adding an
existing domain, removing the default and an unknown action are each refused by
name; after a refused authorization `~/.bay/machine.json` is unchanged.

### Installation

`make install` copies the built binaries to `PREFIX/bin` and the application to
`APPDIR`, and `make uninstall` removes them. The previous `install` target ran
the machine setup, which `bay install` already does.

The command line can also be installed with `go install` from the public
module, without a clone.

Prebuilt downloads are not offered. A quarantined unsigned binary is terminated
before it runs, and signing it requires an Apple Developer ID.

Verification: `go install` of both commands into an empty GOBIN produced working
binaries; a clone of the public repository built with `make`; `make install`
into a temporary prefix installed the binaries and the application, the
installed application launched from its new location, and `make uninstall`
emptied both directories; a release archive marked with `com.apple.quarantine`
exited with 137.

### Appearance control verified in both directions

Selecting Light in the running window changed the window to light while the
system appearance was Dark, and `defaults read dev.bay.bar appearance`
returned `light`. The stored value is applied at the next start.

Verification: `defaults read` after the selection, and a capture of the window.

### A project no longer takes a domain another project serves

Starting a project that claimed a domain a running project already served
removed the running route and printed a warning. `up` and `start` now refuse
before creating any container and name the project holding the domain.

The refusal was found by following `docs/operations/using.md`: a new project
using the default domain took `web.test` from a running project.

Verification: `BAY_E2E=1 go test ./internal/stack -run
TestUpRefusesADomainAnotherProjectServes`, and the operations document was
executed end to end afterwards.

### End-to-end tests assert about their own routes

The tests asserted the machine's total route count, so they failed whenever
another project was running. Each test now counts only the routes under its own
domains.

`docs-check` now rejects a feature row whose evidence names no command, and
verifies that documents name only commands, Compose keys and labels the code
declares. The empty-cell check it replaced passed on any non-empty text.

Verification: `BAY_E2E=1 BAY_E2E_FORCE=1 go test
./internal/stack -count=1` passes with two other projects running, and
`make check` passes.

### Documentation restructured

Documents are split by subject: `docs/spec/` holds contracts, `docs/operations/`
holds procedures, `docs/features.md` holds implementation status and evidence,
and `CHANGELOG.md` holds behavior changes. `README.md` links to them.
`GUIDE.md` was removed; it described a project format the code no longer reads.

Each reader-facing document has a Korean twin with the suffix `.ko.md`.
`make docs-check` verifies links, twins and feature rows, and `make check` runs
it.

Comments across `cmd/` and `internal/` were rewritten to name the action, state
the subject and the object, and give the cause in one sentence.

Verification: `make check` passes, which runs the build, `go test`, `docs-check`
and `go vet`.

### Documentation moved into the commands

`bay brief`, `bay schema` and `bay help <command>`
print the usage contract, the Compose file contract and per-command effects. A
project that installs only the binaries receives these; it does not receive this
repository.

The command and Compose declarations live in `internal/contract`. The
specification documents embed generated sections produced from the same
declarations, and `make docs-check` fails when a document and the code differ.

Verification: `bay brief`, `bay schema` and `bay help
up` produce output; `make docs-generate` wrote the generated sections and a
second run reported no change.

### Window rebuilt as a source list with a dashboard

The window shows a sidebar with Dashboard, Domains, Certificates and one entry
per project, and a detail pane for the selection. The dashboard lists every
project with its service states and every address the proxy serves. An
appearance control in the title bar selects Auto, Dark or Light and stores the
choice in user defaults.

Verification: the application was launched and each screen captured.

### Compose files replace the previous project format

A project is a Compose file. Settings live under `x-bay` and in
`bay.*` service labels. Keys this tool does not read are ignored.

Verification: `go test ./internal/stack` covers both label spellings, port
resolution from `bay.port`, `expose` and `ports`, command and
entrypoint in both forms, and file search in a directory. A project was created
and started from a Compose file containing an internal database service.

### Domains became machine state

A domain is delegated once per machine and recorded in
`~/.bay/machine.json`. A project uses the machine default unless its
Compose file pins one. Removing a domain a project pins is refused.

Verification: `go test ./internal/stack` covers add, remove, default change and
the refusal. A project's Compose file was emptied of domain settings and still
resolved through the machine default.

### Internal services

A service labelled `bay.internal` runs without a domain, a route or a
certificate. Other services reach it at
`<project>-<service>.container.test:<port>`.

Verification: `go test ./internal/stack` covers the label and the refusal to
combine it with a domain. An end-to-end test confirms the snapshot reports no
route and no URL, and a project with a database service reached it by name.

### Commands wait for the proxy

`nginx -s reload` returns after sending the signal, and a worker being replaced
can accept a connection it does not serve. The proxy now reports the
configuration generation at `/__bay/health`, and the commands poll it
before returning.

Verification: the end-to-end test that previously failed on a 15 second client
timeout now completes in under four seconds.

### End-to-end tests no longer disturb a running machine

The proxy is machine-level, so a test using a different state directory removed
the proxy a user was running. The tests now skip when a proxy belonging to
another state directory is running, unless `BAY_E2E_FORCE` is set.
`EnsureProxy` recreates a proxy whose mounted directories differ from the state
directory in use.

Verification: with a proxy running from `~/.bay`, the end-to-end tests
report skip with the state directory named.

### DNS returns SERVFAIL while the proxy is down

The DNS server previously returned NXDOMAIN when the proxy was absent, and
macOS cached that answer, so names stayed unresolvable after the proxy
returned. It now returns SERVFAIL, which is not cached.

Verification: with the proxy stopped, a query returned SERVFAIL.

### Certificate authority trusted without administrator rights

Trusting the authority previously used the administrator certificate store,
which requires root and cannot present the confirmation the operation needs.
It now uses the user's trust settings.

Verification: `security add-trusted-cert -r trustRoot` on the authority
returned success without administrator rights, and `security verify-cert`
reported the certificate as valid.
