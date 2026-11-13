# bay

`bay` runs Compose projects on Apple `container` and on Docker, and
serves each service over HTTPS at a local domain name. No service port is
published. Every service container trusts the certificates of these names, so
services reach each other's names over HTTPS too.

Machines approve each other as peers and reach each other's domains over a
mutually authenticated link. A Linux machine, such as a single-board computer
running Docker, is set up the same way and can be limited to accept
connections only from the machine it is reached through.

A machine can run both engines at the same time. Each engine runs its own proxy
and serves its own containers. An engine that is absent, or whose daemon does
not respond, contributes no containers and does not cause an error.

A project is a Compose file. A service with no extra settings is served at
`<service>.test`.

## Start

```sh
go install github.com/polyspec/bay/cmd/bay@latest
go install github.com/polyspec/bay/cmd/bayd@latest
bay install -dry-run   # what the setup changes
bay install            # once per machine; asks for the password
bay check              # every part of the setup, ok or missing
cd ~/work/your-project
bay up
```

`install` changes only what is missing, so it is run again after an update or
whenever `check` reports a missing part. On Linux it is run as root, and
`bay install -only-from 192.168.2.1` limits the machine to that
host; see [Peers](docs/spec/peers.md).

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

## Agents

An agent learns the tool from the same contract the commands print, and what
it reads follows the installed binary. `install` writes the skill to
`~/.claude/skills/bay/` and `~/.agents/skills/bay/` where those directories
exist, and registers `bay mcp` in Claude Code's user configuration where the
`claude` command is installed. `make install`, `up` and the MCP server at the
start of each session rewrite the skill when it differs from the binary's, and
`check` reports a skill or registration that is missing or out of date. The MCP
tools are described by the running binary itself, marked read-only, idempotent
or destructive from their access, with `brief`, `schema` and `status` as
resources. A command that asks a person something gets no answer from an
agent; the agent asks the user to run it. The section below is generated from the code.

<!-- generated: bay brief; run "make docs-generate" -->

```sh
bay install                               # also writes the skill below
bay skill install                         # ~/.claude/skills and ~/.agents/skills
bay mcp install                                    # registers it in Claude Code; install does too
bay brief --json                          # the contract, schemaVersion 1
```

| Command | Summary | Access | Asks a person for |
| --- | --- | --- | --- |
| `bay up` | Start this project's services and register their routes. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay down` | Remove this project's services and withdraw its routes. | remove | nothing |
| `bay start [service...]` | Start services. With no name, starts every service in the project. | change | nothing |
| `bay stop [service...]` | Stop services. The containers remain and can be started again. | change | nothing |
| `bay restart [service...]` | Stop then start services. | change | nothing |
| `bay logs [-f] [-n N] <service>` | Print one service's output. | read | nothing |
| `bay domain [add|remove|default] [name]` | List the machine's domains, or change them. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay status [--json]` | Print the proxy, the DNS server and every registered project. | read | nothing |
| `bay doctor` | Report what machine setup is missing. Changes nothing. | read | nothing |
| `bay check` | Report each part of the machine setup and fail when any is missing. Changes nothing. | read | nothing |
| `bay peer [open|close|find|add|remove] [address|name|fingerprint]` | List the machines whose domains this one reaches, or change them. | remove | approval of the other machine's fingerprint, read from standard input; with no one to answer, nothing is approved |
| `bay sync` | Rewrite the proxy configuration from the containers that are running. | change | nothing |
| `bay install [-dry-run] [-only-from host,...|none]` | Apply the machine setup that is missing. Running it again changes nothing. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay uninstall` | Remove the machine setup install wrote. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay brief [--json]` | Print the full usage contract on one screen. | read | nothing |
| `bay schema [--json]` | Print the Compose extension contract. | read | nothing |
| `bay skill [print|install]` | Print the agent skill, or write it where agents read skills. | change | nothing |
| `bay mcp [install]` | Serve the commands to an agent as an MCP server on standard input and output, or register it. | change | nothing |
| `bay help [command]` | Print help for one command, or the command list. | read | nothing |

| Exit status | Meaning |
| --- | --- |
| `0` | The command did what it states. For check, every part of the setup is in place. |
| `1` | The command failed, or check found a part missing. Standard error says why and names the command that reports more. |
| `2` | The command line names no command or an unknown one. |

<!-- end generated -->

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
| [Peers](docs/spec/peers.md) | Peer link, relay, Linux setup and firewall |
| [Agent skill](skills/bay/SKILL.md) | What an agent reads; generated by `bay skill print` |
| [Development](AGENTS.md) | Building, testing and the required checks |

Korean documents carry the suffix `.ko.md`.

## Requirements

macOS with Apple `container` 1.3 or later or with Docker, and Go 1.27 or later.
The application also needs the Xcode command line tools. A Linux peer needs
Docker, systemd and, for `-only-from`, nftables.
