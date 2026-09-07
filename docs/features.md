# Features

One row per feature. Status is the implementation state, Evidence names the
verification that was run against the current code, and Shipped states whether
the feature is in the binaries under `bin/`.

An earlier verification is not evidence for changed code. When a feature
changes, its evidence is replaced by the verification run for that change.

| Feature | Status | Evidence | Shipped |
| --- | --- | --- | --- |
| Compose file as the project format | Implemented | `go test ./internal/stack` and `go test ./internal/contract` cover key parsing, both label spellings, port resolution, file search and the documented example | Yes |
| Machine-level domains with per-project override | Implemented | `go test ./internal/stack` covers add, remove, default change and pinned-domain refusal | Yes |
| Routes derived from container labels | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy` starts two projects and confirms each answers on its own domain | Yes |
| Single proxy shared by projects | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestTwoGroupsShareOneProxy` confirms one project stopping leaves the other serving | Yes |
| Proxy follows a container to a new address | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestServiceRestartKeepsRoute` recreates a service and confirms the proxy reaches the new address | Yes |
| Commands wait for the proxy to serve the new configuration | Implemented | `BAY_E2E=1 go test ./internal/stack` completes in under four seconds where it previously failed on a fifteen second client timeout | Yes |
| Internal services without a domain | Implemented | `go test ./internal/stack` covers the label, and an end-to-end test confirms no route and no certificate | Yes |
| A project cannot take a domain another running project serves | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestUpRefusesADomainAnotherProjectServes` confirms the refusal names the holding project and creates no container | Yes |
| Service-level start, stop, restart, logs | Implemented | `BAY_E2E=1 go test ./internal/stack -run TestStoppingOneServiceWithdrawsOnlyItsRoute` and `-run TestLogsReachTheCaller` | Yes |
| Certificate issue and reissue | Implemented | `go test ./internal/stack` covers listing, expiry reporting, removal and authority rotation | Yes |
| Certificate authority trusted without administrator rights | Implemented | `security add-trusted-cert` run against the authority returned success, `security verify-cert` confirmed trust | Yes |
| Resolver entries written with acquired rights | Implemented | `/etc/resolver/test` created by the application through the system authentication panel | Yes |
| Machine state snapshot | Implemented | `go test ./internal/stack` compares the snapshot against a running project | Yes |
| Menu bar application | Implemented | `open -n bin/baybar.app` followed by a synthetic click on a row button opened the log window, confirming the button to callback path | Yes |
| Source list window with dashboard | Implemented | `open -n bin/baybar.app` and `screencapture -l <window>` produced the dashboard with the sidebar, project rows and address list | Yes |
| Appearance control: Auto, Dark, Light | Implemented | Read path: with the system set to Dark, `defaults write dev.bay.bar appearance light` and a restart produced a light window with Light selected. Write path: selecting Light in the running window changed the window to light and `defaults read dev.bay.bar appearance` returned `light` | Yes |
| Operations documents match the commands | Implemented | Each claim in `docs/operations/using.md` was executed: `bay up`, `stop`, `start`, `logs`, `status --json` against a project created from the document | Yes |
| Usage contract in the commands | Implemented | `bay brief`, `schema` and `help <command>` produce output | Yes |
| Generated specification sections | Implemented | `make docs-generate` writes the tables and `make docs-check` compares them | Yes |
| Korean documents for reader-facing files | Implemented | `make docs-check` reports every twin present with matching heading counts | Yes |
| Code comments in direct language | Implemented | A search for the rejected phrasings over `cmd/` and `internal/` returns no match outside tests | Yes |
