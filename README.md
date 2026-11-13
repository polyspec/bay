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

### Pinned release (Linux, CI)

Each release carries `bay` and `bayd` for macOS and Linux on arm64 and amd64,
with `SHA256SUMS`. A build from a release or from `make` carries bayinit, which
puts this machine's authority in a container's own store; `go install` does not,
and `bay check` says so. On Linux the setup and the commands run as root, so a
CI job runs them with `sudo`:

```sh
VERSION=v0.0.1
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
NAME=bay_${VERSION}_linux_${ARCH}
curl -fsSLO https://github.com/polyspec/bay/releases/download/$VERSION/$NAME.tar.gz
curl -fsSLO https://github.com/polyspec/bay/releases/download/$VERSION/SHA256SUMS
grep " $NAME.tar.gz\$" SHA256SUMS | sha256sum -c -
tar -xzf $NAME.tar.gz
sudo install -m 755 $NAME/bay $NAME/bayd /usr/local/bin/
sudo bay install            # asks nothing as root
sudo bay check -scope run   # exit 1 when a part a run needs is missing
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

### What it is for

- **Run a Compose project as its file says, on Apple container or Docker.** A key the Compose specification defines behaves as it does there, or is rejected by name; nothing is silently ignored.
- **Give every service a name, a certificate and trust.** <project>-<service>.bay.test, known before up from the file alone; a certificate for that name from this machine's authority; containers that trust the authority.
- **Serve the domains a project declares over HTTPS.** One proxy per machine, routes from container labels, certificates issued and renewed by the tool.
- **Reach the names of approved peer machines.** Over the mutually authenticated peer link, and only to machines approved by fingerprint.
- **Be a dependable step for a person, a script or an agent.** Exit status that says whether the request was done, JSON output where a program reads the result, nothing left behind by an interrupted command, and the same result when run twice.
- **Keep a Linux machine reachable while projects use it.** The containers bay runs share a slice that slows them past 70% of memory and holds them under 80% with at most 10% in swap, and the kernel keeps a reserve for the network driver, so a loaded machine is slow but answers.
- **Apply the machine setup only on request.** install applies what is missing; every other command reads the setup and names install when a part is missing.

### What it does not do

| Not done | Why | Instead |
| --- | --- | --- |
| Running or stopping a project on another machine. | A project's files, volumes, state and setup are on the machine that runs it; a remote up would act on paths and setup it cannot see. | Run the command on the machine that holds the project, for example over ssh: ssh host bay -f /path/compose.yaml up. |
| Publishing a service port or a Unix socket on the host. | Every service is reached by name, so two projects never compete for a host port and a reachable service is always a named, certified one. | Connect to <project>-<service>.bay.test:<port>; a socket in a bind-mounted directory is plain Compose and works where the engine shares the host's kernel. |
| Waiting without a bound. | A wait with no limit hides a service that never starts; every wait states its bound before it begins. | Give a slow service a longer healthcheck start_period or interval. |
| Deciding a caller's policy: retries, time limits, which images to fetch. | These belong to the caller's rules; the tool states what it does and offers the switches. | bay pull, up -no-pull, healthcheck keys, exit status. |
| Changing the machine setup from a project command. | A setup change asks for administrator rights and affects every project; it happens when a person asks for it. | bay install -dry-run, then bay install. |

```sh
bay install                               # also writes the skill below
bay skill install                         # ~/.claude/skills and ~/.agents/skills
bay mcp install                                    # registers it in Claude Code; install does too
bay brief --json                          # the contract, schemaVersion 1
```

| Command | Summary | Access | Asks a person for |
| --- | --- | --- | --- |
| `bay up [-no-pull] [--json]` | Start this project's services and register their routes. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay down` | Remove this project's services and withdraw its routes. | remove | nothing |
| `bay start [service...]` | Start services. With no name, starts every service in the project. | change | nothing |
| `bay stop [service...]` | Stop services. The containers remain and can be started again. | change | nothing |
| `bay restart [service...]` | Stop then start services. | change | nothing |
| `bay logs [-f] [-n N] <service>` | Print one service's output. | read | nothing |
| `bay plan [--json]` | Print each service's name, port, domains and certificate paths as up will create them. | read | nothing |
| `bay exec [-u user] [-w dir] [-e KEY=VALUE] <service> <command...>` | Run a command in a service's running container and exit with its status. | change | nothing |
| `bay pull [service...]` | Fetch the images of this project's services from their registries. | change | nothing |
| `bay images [--json] [service...]` | Report whether the local store holds each service's image. Fetches nothing. | read | nothing |
| `bay domain [add|remove|default] [name]` | List the machine's domains, or change them. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay status [--json]` | Print the proxy, the DNS server and every registered project. | read | nothing |
| `bay doctor` | Report what machine setup is missing. Changes nothing. | read | nothing |
| `bay check [-scope all|run]` | Report each part of the machine setup and fail when any is missing. Changes nothing. | read | nothing |
| `bay peer [open|close|find|add|remove] [address|name|fingerprint]` | List the machines whose domains this one reaches, or change them. | remove | approval of the other machine's fingerprint, read from standard input; with no one to answer, nothing is approved |
| `bay sync` | Rewrite the proxy configuration from the containers that are running. | change | nothing |
| `bay cert client <name> [-out dir] [--json]` | Write a client certificate the machine's authority issues, for mutual TLS from outside the containers. | change | nothing |
| `bay install [-dry-run] [-only-from host,...|none]` | Apply the machine setup that is missing. Running it again changes nothing. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay uninstall` | Remove the machine setup install wrote. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay brief [--json]` | Print the full usage contract on one screen. | read | nothing |
| `bay schema [--json]` | Print the Compose extension contract. | read | nothing |
| `bay skill [print|install]` | Print the agent skill, or write it where agents read skills. | change | nothing |
| `bay mcp [install]` | Serve the commands to an agent as an MCP server on standard input and output, or register it. | change | nothing |
| `bay help [command]` | Print help for one command, or the command list. | read | nothing |
| `bay version [--json]` | Print this executable's version, commit, Go version, platform and contract schema version. | read | nothing |

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

## License

MIT. See [LICENSE](LICENSE).
