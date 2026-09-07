# Development

This document covers working on this repository. It is not the usage contract
for other projects; that is printed by `bay brief`.

## Build and check

```sh
make            # build bin/bay, bin/bayd, bin/baybar.app
make check      # gofmt, go vet, go test, docs-check
make e2e        # tests that start real containers
make install    # copy the built files to PREFIX/bin and APPDIR
make uninstall  # remove them
```

`make check` must pass before a change is finished.

## Layout

| Path | Content |
| --- | --- |
| `internal/stack` | Compose parsing, machine state, containers, proxy, certificates, DNS install |
| `internal/contract` | Command and Compose declarations; help, brief, schema and the generated document sections render from here |
| `cmd/bay` | Command line |
| `cmd/bayd` | DNS server |
| `cmd/baybar` | Menu bar application and window, in Go with an AppKit layer in Objective-C |
| `cmd/docsgen` | Writes the generated sections of the specification documents |
| `cmd/docscheck` | Verifies links, Korean documents and feature rows |
| `design/` | Design canvas sources; not part of the build |

## Documentation rules

1. Documents describe current behavior. When direction changes, edit
   `docs/spec/` first and mark what is not implemented.
2. A change that alters behavior updates the specification,
   `docs/features.md` and `CHANGELOG.md` in the same change.
3. English documents are canonical. Each reader-facing document has a
   `<name>.ko.md` twin, edited in the same change.
4. Record test results and deployment results separately. Do not reuse an
   earlier result as evidence for changed code.
5. Documents, comments and commit messages describe this project only.
6. Use direct language: name the action, state the subject and the object, give
   the cause in one sentence.

After changing a declaration in `internal/contract`, run:

```sh
make docs-generate
```

## Tests that start containers

`make e2e` starts real containers and takes over the machine's proxy, which is
shared. The tests skip when a proxy belonging to another state directory is
running. Set `BAY_E2E_FORCE=1` to run anyway; this removes the proxy
the machine is using.

## Rules the code holds to

| Rule | Where |
| --- | --- |
| One proxy per machine | `stack.ProxyName`, `stack.EnsureProxy` |
| Routes come from container labels | `stack.Routes`, `stack.Instances` |
| Domains are machine state | `stack.Machine.Domains` |
| Commands return after the proxy serves the new configuration | `stack.WaitForGeneration` |
| Only `/etc/resolver` writes need administrator rights | `stack.Install.privilegedSteps` |

## Adding a command

1. Declare it in `internal/contract.Commands`, with its effect and whether it
   can require administrator rights.
2. Handle it in `cmd/bay/main.go`.
3. Run `make docs-generate`, then `make check`.
4. Add a row to `docs/features.md` and an entry to `CHANGELOG.md`.
