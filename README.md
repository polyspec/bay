# bay

`bay` runs Compose projects on Apple `container` and on Docker, and
serves each service over HTTPS at a local domain name. No service port is
published, and no entry is added to `/etc/hosts`.

A machine can run both engines at the same time. Each engine runs its own proxy
and serves its own containers. An engine that is absent, or whose daemon does
not respond, contributes no containers and does not cause an error.

A project is a Compose file. A service with no extra settings is served at
`<service>.test`.

## Start

```sh
go install github.com/polyspec/bay/cmd/bay@latest
go install github.com/polyspec/bay/cmd/bayd@latest
bay install         # once per machine; asks for the password
cd ~/work/your-project
bay up
```

For the menu bar application as well, clone this repository and run `make`
then `sudo make install`. See [Install](docs/operations/install.md).

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

macOS with Apple `container` 1.3 or later or with Docker, and Go 1.27 or later.
The application also needs the Xcode command line tools.
