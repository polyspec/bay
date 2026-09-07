# bay

`bay` runs Compose projects on Apple `container` and serves each
service over HTTPS at a local domain name. No host port is published, and no
entry is added to `/etc/hosts`.

A project is a Compose file. A service with no extra settings is served at
`<service>.test`.

## Start

```sh
make
bin/bay install     # once per machine; asks for the password
cd ~/work/your-project
bay up
```

## Usage instructions live in the commands

```sh
bay brief           # the whole usage contract, --json for programs
bay schema          # the Compose file contract, --json for programs
bay help up         # what one command changes
bay status --json   # the current machine state
```

A project that installs the binaries does not receive this repository, so the
commands carry the instructions.

## Documents

| Document | Content |
| --- | --- |
| [Install](docs/operations/install.md) | One-time machine setup and how to remove it |
| [Using](docs/operations/using.md) | Adding a project, domains, ports, daily commands |
| [Troubleshooting](docs/operations/troubleshooting.md) | Symptom, cause, action |
| [Architecture](docs/spec/architecture.md) | Components, request path, routing state |
| [Command contract](docs/spec/cli.md) | Commands, effects, rules, diagnostics |
| [Compose contract](docs/spec/compose-schema.md) | Keys, labels, port resolution |
| [Machine state](docs/spec/machine-state.md) | Every path created outside this repository |
| [Features](docs/features.md) | Implementation status and verification |
| [Changelog](CHANGELOG.md) | Behavior changes and their verification |
| [Documentation plan](docs/documentation-plan.md) | Where each kind of information is written |
| [Development](AGENTS.md) | Building, testing and the required checks |

Korean documents carry the suffix `.ko.md`.

## Requirements

macOS with Apple `container` 1.3 or later, and Go 1.27 or later to build.
