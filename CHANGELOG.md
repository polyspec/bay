# Changelog

Entries describe behavior changes and the verification run for each. Dates are
the day the change was made.

## 2026-09-08

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
