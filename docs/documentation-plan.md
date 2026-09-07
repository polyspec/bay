# Documentation plan

This document defines where each kind of information is written, how it is kept
current, and which checks enforce that. It applies to this repository.

## Problem

Documents in this repository state behavior the code no longer has. `README.md`
describes a `stack.yaml` file format that was replaced by Compose files.
`GUIDE.md` describes per-project domains, which are now machine-level. Both were
written and not verified after the behavior changed.

Usage instructions also do not reach their reader. A project that installs the
binaries does not receive this repository's documents, so an agent working in
that project cannot read them.

## Rules

1. Documents describe current behavior. When direction changes, the
   specification is edited first, and parts that are not implemented are marked.
2. A change that alters behavior updates the specification, `docs/features.md`
   and `CHANGELOG.md` in the same change.
3. English documents are canonical. Each reader-facing document has a Korean
   twin named `<name>.ko.md`. Both are edited together and state the same
   information.
4. Test results and deployment results are recorded separately. An earlier
   result is not used as evidence for changed code.
5. `make docs-check` runs in the default checks. It verifies links, twin files
   and status entries. Content accuracy is verified by reading the code and the
   tests.
6. Documents, comments and commit messages describe this project only. External
   projects are not named and influences are not attributed.
7. Comments, commit messages and documents use direct language: name the
   action, state the subject and the object, give the cause in one sentence.

## Layout

| Location | Content |
| --- | --- |
| `README.md` | Project summary, minimal start procedure, links to the rest |
| `docs/spec/` | Approved structure, contracts, rules, acceptance criteria |
| `docs/features.md` | Per-feature implementation status, verification evidence, deployment status |
| `docs/operations/` | Current install, run and verification procedures |
| `CHANGELOG.md` | Feature changes and their verification results |
| `docs/plans/` | Proposals awaiting approval; folded into the specification and deleted after approval |
| `AGENTS.md` | Development procedure and required checks for this repository |

`AGENTS.md` covers developing this repository. It is not the usage contract for
other projects.

## Usage instructions are carried by the commands

A project that installs `bay` receives the binary, not this
repository. Usage instructions therefore live in the commands:

| Command | Output |
| --- | --- |
| `bay brief` | The full usage contract on one screen: purpose, commands, Compose schema, invariants, diagnostics. `--json` for structured output. |
| `bay schema` | The Compose extension contract: `x-bay` keys, `bay.*` labels, port resolution order. `--json` for structured output. |
| `bay help <command>` | What the command does, what it changes on the machine, whether it needs administrator rights, and an example. |
| `bay status --json` | Current machine state. |

`brief` and `schema` are generated from the same declarations the code uses, so
they cannot disagree with the implementation.

## Files

### Specification, `docs/spec/`

| File | Content |
| --- | --- |
| `architecture.md` | Components, request path, ownership of routing state |
| `cli.md` | Command contract: arguments, effects, exit behavior, privilege requirements |
| `compose-schema.md` | `x-bay` keys, `bay.*` labels, defaults, validation rules |
| `machine-state.md` | Files and system state created outside the repository, and how each is removed |

### Operations, `docs/operations/`

| File | Content |
| --- | --- |
| `install.md` | One-time machine setup and what it changes |
| `using.md` | Adding a project, running it, daily commands |
| `troubleshooting.md` | Symptom, cause and diagnostic command |

### Status

`docs/features.md` holds one row per feature: implementation status,
verification evidence, and whether it is in the built binaries.

`CHANGELOG.md` holds dated entries describing behavior changes and the
verification that was run for each.

## Checks

`make docs-check` verifies:

- every Markdown link in `README.md`, `AGENTS.md`, `CHANGELOG.md` and `docs/`
  resolves to a file that exists;
- every reader-facing document has a `.ko.md` twin, and the twin has the same
  heading count;
- `docs/features.md` contains a status and an evidence cell for every row;
- `bay brief` and `bay schema` run and produce output.

`make check` runs `gofmt`, `go vet`, `go test` and `docs-check`.

## Work required

| Item | State |
| --- | --- |
| Write this plan | Done |
| `bay brief`, `schema`, per-command help | Not started |
| `docs/spec/` | Not started |
| `docs/operations/` | Not started |
| `docs/features.md`, `CHANGELOG.md` | Not started |
| `AGENTS.md` | Not started |
| Rewrite `README.md`, delete `GUIDE.md` | Not started |
| Korean twins | Not started |
| `make docs-check` | Not started |
| Rewrite existing code comments in direct language | Not started |
