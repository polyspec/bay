# Features

One row per feature. Status is the implementation state, Evidence names the
verification that was run against the current code, and Shipped states whether
the feature is in the binaries under `bin/`.

An earlier verification is not evidence for changed code. When a feature
changes, its evidence is replaced by the verification run for that change.

| Feature | Status | Evidence | Shipped |
| --- | --- | --- | --- |
| Compose-managed durable named volumes | Implemented | `make check` and `go test -race ./internal/stack` cover ownership and declaration validation; `BAY_SERVICE_E2E=1 go test -race ./internal/stack -run TestManagedVolumeActualRuntime` confirms data and volume identity survive service removal/recreation | Local build; not installed |
| Compose runtime constraints and ordered, reusable service startup | Implemented | `make check` covers interpolation, mounts, dependency errors, ownership, unchanged reuse and completion evidence; `BAY_SERVICE_E2E=1 go test ./internal/stack -run TestServiceLifecycleActualRuntime` checks actual restrictions, initialization and preexisting container preservation | Local build; not installed |
| Compose file as the project format | Implemented | `go test ./internal/stack` and `go test ./internal/contract` cover key parsing, both label spellings, port resolution, file search and the documented example | Yes |
| Machine-level domains with per-project override | Implemented | `go test ./internal/stack` covers add, remove, default change and pinned-domain refusal | Yes |
| Routes derived from container labels | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy` starts two projects and confirms each answers on its own domain | Yes |
| Single proxy shared by projects | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy` confirms one project stopping leaves the other serving | Yes |
| Proxy follows a container to a new address | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestServiceRestartKeepsRoute` recreates a service and confirms the proxy reaches the new address | Yes |
| Commands wait for the proxy to serve the new configuration | Implemented | `BAY_E2E=1 go test ./internal/stack` completes in under four seconds where it previously failed on a fifteen second client timeout | Yes |
| Internal services without a domain | Implemented | `go test ./internal/stack` covers the label, and an end-to-end test confirms no route and no certificate | Yes |
| A project cannot take a domain another running project serves | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestUpRefusesADomainAnotherProjectServes` confirms the refusal names the holding project and creates no container | Yes |
| Starting reported separately from running | Implemented | `TestReadinessActualRuntime` checks both snapshot paths, Ready/Live and WaitReady with an isolated listener; lifecycle tests exclude completed initializers and empty selections. The original `TestStartingIsDistinctFromRunning` is preserved and skips while the shared proxy is in use | Local build; not installed |
| Service-level start, stop, restart, logs | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestStoppingOneServiceWithdrawsOnlyItsRoute` and `-run TestLogsReachTheCaller` | Yes |
| Certificate issue and reissue | Implemented | `go test ./internal/stack` covers listing, expiry reporting, removal and authority rotation | Yes |
| Certificate authority trusted without administrator rights | Implemented | `security add-trusted-cert` run against the authority returned success, `security verify-cert` confirmed trust | Yes |
| Resolver entries written with acquired rights | Implemented | `/etc/resolver/test` created by the application through the system authentication panel | Yes |
| Domain management from the command line | Implemented | `bay domain` lists, and `add`, `remove` and `default` were exercised: adding an existing domain, removing the default and an unknown action are each refused by name, and a failed apply restores `~/.bay/machine.json` | Yes |
| Machine state snapshot | Implemented | `go test ./internal/stack` compares the snapshot against a running project | Yes |
| Menu bar application | Implemented | `open -n bin/baybar.app` followed by a synthetic click on a row button opened the log window, confirming the button to callback path | Yes |
| Source list window with dashboard | Implemented | `open -n bin/baybar.app` and `screencapture -l <window>` produced the dashboard with the sidebar, project rows and address list | Yes |
| Appearance control: Auto, Dark, Light | Implemented | Read path: with the system set to Dark, `defaults write dev.bay.bar appearance light` and a restart produced a light window with Light selected. Write path: selecting Light in the running window changed the window to light and `defaults read dev.bay.bar appearance` returned `light` | Yes |
| Installation from the public module | Implemented | `GOBIN=/tmp/x go install github.com/polyspec/bay/cmd/bay@latest` and the same for `bayd` produced working binaries, and `bay brief` ran from them | Yes |
| Installation from a clone | Implemented | `git clone` of the public repository followed by `make` produced all three outputs and `bin/bay brief` ran | Yes |
| `make install` and `make uninstall` | Implemented | `make install PREFIX=/tmp/prefix APPDIR=/tmp/apps` installed both binaries and the application; the installed application launched from its new location; `make uninstall` emptied both directories | Yes |
| Prebuilt downloads rejected as a distribution path | Implemented | A release archive marked with `com.apple.quarantine` was terminated on launch with exit 137, so the paths above build on the machine instead | Yes |
| Operations documents match the commands | Implemented | Each claim in `docs/operations/using.md` was executed against a project created from the document: `bay up`, `stop`, `start`, `logs`, `status --json`, the HTTP redirect, service-to-service access by name, and the refusal of a domain another project serves | Yes |
| Usage contract in the commands | Implemented | `bay brief`, `schema` and `help <command>` produce output | Yes |
| Generated specification sections | Implemented | `make docs-generate` writes the tables and `make docs-check` compares them | Yes |
| Korean documents for reader-facing files | Implemented | `make docs-check` reports every twin present with matching heading counts | Yes |
| Code comments in direct language | Implemented | A search for the rejected phrasings over `cmd/` and `internal/` returns no match outside tests | Yes |
