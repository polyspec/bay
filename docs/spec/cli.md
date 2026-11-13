# Command line contract

Status: implemented.

Read-only commands follow the [diagnostic rules](diagnostics.md).

The tables below are generated from `internal/contract`. Run
`make docs-generate` after changing a command declaration, and `make docs-check`
verifies that this file matches the code.

<!-- generated: bay brief --markdown -->

| Command | Summary | Access | Administrator rights |
| --- | --- | --- | --- |
| `up [-no-pull] [--json]` | Start this project's services and register their routes. | change | Can require |
| `down` | Remove this project's services and withdraw its routes. | remove | No |
| `start [service...]` | Start services. With no name, starts every service in the project. | change | No |
| `stop [service...]` | Stop services. The containers remain and can be started again. | change | No |
| `restart [service...]` | Stop then start services. | change | No |
| `logs [-f] [-n N] <service>` | Print one service's output. | read | No |
| `plan [--json]` | Print each service's name, port, domains and certificate paths as up will create them. | read | No |
| `exec [-u user] [-w dir] [-e KEY=VALUE] <service> <command...>` | Run a command in a service's running container and exit with its status. | change | No |
| `pull [service...]` | Fetch the images of this project's services from their registries. | change | No |
| `images [--json] [service...]` | Report whether the local store holds each service's image. Fetches nothing. | read | No |
| `domain [add|remove|default] [name]` | List the machine's domains, or change them. | remove | Can require |
| `status [--json]` | Print the proxy, the DNS server and every registered project. | read | No |
| `doctor` | Report what machine setup is missing. Changes nothing. | read | No |
| `check [-scope all|run]` | Report each part of the machine setup and fail when any is missing. Changes nothing. | read | No |
| `peer [open|close|find|add|remove] [address|name|fingerprint]` | List the machines whose domains this one reaches, or change them. | remove | No |
| `sync` | Rewrite the proxy configuration from the containers that are running. | change | No |
| `cert client <name> [-out dir] [--json]` | Write a client certificate the machine's authority issues, for mutual TLS from outside the containers. | change | No |
| `install [-dry-run] [-only-from host,...|none]` | Apply the machine setup that is missing. Running it again changes nothing. | change | Can require |
| `uninstall` | Remove the machine setup install wrote. | remove | Can require |
| `brief [--json]` | Print the full usage contract on one screen. | read | No |
| `schema [--json]` | Print the Compose extension contract. | read | No |
| `skill [print|install]` | Print the agent skill, or write it where agents read skills. | change | No |
| `mcp [install]` | Serve the commands to an agent as an MCP server on standard input and output, or register it. | change | No |
| `help [command]` | Print help for one command, or the command list. | read | No |
| `version [--json]` | Print this executable's version, commit, Go version, platform and contract schema version. | read | No |

## Flags before the command

| Flag | Description |
| --- | --- |
| `-f path` | The Compose file, or a directory to find one in. Default: the working directory. |
| `-p name` | The project name, instead of the Compose file's name or directory. Two runs of one file under two names do not overlap: their containers, default volume names and registrations are separate, and down -p removes only its own. |
| `-state dir` | The state directory. Default: ~/.bay. |

## Exit status

| Status | Meaning |
| --- | --- |
| `0` | The command did what it states. For check, every part of the setup is in place. |
| `1` | The command failed, or check found a part missing. Standard error says why and names the command that reports more. |
| `2` | The command line names no command or an unknown one. |

## Effects

### `up`

Reuses unchanged owned containers and proven completed initializers; replaces changed configuration or local image digests. Starts dependencies in order, checks declared health, issues certificates and updates the proxy. Delegates new project domains. Removes the containers of services the Compose file no longer declares, stopping a running one first, and withdraws their routes. -no-pull fails before creating anything when the local store lacks an image, instead of pulling it. --json prints each service's name, host name, address, port, URLs and certificate paths on standard output, and progress on standard error. On SIGINT or SIGTERM it ends the engine commands that run, removes the containers this up created and leaves the ones that were there before, withdraws the registration of a project that had no container before, keeps volumes, and exits with 128 plus the signal number.

### `down`

Removes the project's containers. Removes the proxy when no route remains.

### `start`

Reconciles selected services and their dependencies, reuses unchanged containers, checks startup conditions, then updates the proxy.

### `stop`

Stops containers and withdraws their routes.

### `restart`

Recreates selected owned containers, including explicitly retried initializers, after preparing their dependencies; updates the proxy.

### `exec`

Whatever the command does inside the container. Standard input, output and error are the caller's, with a terminal when standard input is one. The exit status is the command's own; a status of 1 with a line starting with bay: on standard error is bay's own failure.

### `pull`

Pulls each selected service's image, or every service's with no name, whether or not the local store holds it. Creates, starts and changes no container.

### `images`

Prints present or missing per service and exits 1 when any image is missing.

### `domain`

add writes an /etc/resolver entry for the domain. remove deletes it. default changes the domain projects use when their Compose file names none.

### `status`

Reads state without registering the selected Compose project, creating certificates, or changing machine setup. Missing or unreadable public certificates are reported; private keys are not read.

### `doctor`

Reads public certificate and setup state, including the selected project's domains without saving them. Creates no files and reads no private keys.

### `check`

Prints one line per part: ok or missing, and for a missing part what "install" would do about it. Exits non-zero when any part is missing, so a script can call it before relying on the machine. -scope run leaves out the parts that only tell agents about the tool (the agent skill and the MCP registration), so a test run can require exactly what running a project needs.

### `peer`

With no argument it lists them and says whether the link is open. "open" answers the link and announces this machine to the network, which another machine reads to approve this one; "close" stops both. "find" lists the machines announcing themselves. "add" takes an address, or the name of a machine that announced itself, reads what it says about itself, checks that it holds the authority it names, prints the fingerprint and asks before approving. "remove" withdraws an approval. Every change rewrites the proxy configuration. Nothing is added to the keychain.

### `sync`

Writes the server blocks and issues any certificate a route needs, then reloads the proxy and waits until it serves the new configuration. Starts, stops and changes no container.

### `cert`

Issues a certificate for name, usable as a client, kept in the state directory and reused while more than 30 days remain, and writes cert.pem, key.pem (mode 0600) and ca.pem to the directory -out names, the working directory by default.

### `install`

On macOS: writes /etc/resolver entries, adds the certificate authority to the user's trust settings, registers the DNS launchd agent and the peer relay launchd daemon. On another system: creates the certificate authority and, when -only-from names hosts, an nftables firewall that accepts new connections on the default route's interface only from them; the hosts are recorded, so the next install keeps them, and "none" removes the firewall. Each part already in place is left alone. -dry-run prints what would change and changes nothing. Registers no project.

### `uninstall`

On macOS removes /etc/resolver entries, the launchd agent and the relay daemon; the certificate authority stays in the trust settings. On another system removes the firewall.

### `skill`

print writes SKILL.md to standard output. install writes it to ~/.claude/skills/bay/ and ~/.agents/skills/bay/, for each of ~/.claude and ~/.agents that exists, only when the content differs. "bay install", "bay up", "make install" and the MCP server at its start do the same, and check reports a skill that is missing or out of date.

### `mcp`

Offers each command that is not terminal-only as a tool, annotated read-only, idempotent or destructive from its access, and brief, schema and status as resources. A tool runs this executable with the given arguments in the given project directory and returns its output and exit status. A command's standard input is empty, so a question to a person is answered no. install registers this executable in Claude Code's user configuration as "bay" through the claude command, only when the registration differs; "bay install" does the same where claude is installed, and check reports a missing or stale registration. Serving rewrites the agent skill when it differs from this executable's, so the skill follows an update.

## What it is for

- **Run a Compose project as its file says, on Apple container or Docker.** A key the Compose specification defines behaves as it does there, or is rejected by name; nothing is silently ignored.
- **Give every service a name, a certificate and trust.** <project>-<service>.bay.test, known before up from the file alone; a certificate for that name from this machine's authority; containers that trust the authority.
- **Serve the domains a project declares over HTTPS.** One proxy per machine, routes from container labels, certificates issued and renewed by the tool.
- **Reach the names of approved peer machines.** Over the mutually authenticated peer link, and only to machines approved by fingerprint.
- **Be a dependable step for a person, a script or an agent.** Exit status that says whether the request was done, JSON output where a program reads the result, nothing left behind by an interrupted command, and the same result when run twice.
- **Apply the machine setup only on request.** install applies what is missing; every other command reads the setup and names install when a part is missing.

## What it does not do

| Not done | Why | Instead |
| --- | --- | --- |
| Running or stopping a project on another machine. | A project's files, volumes, state and setup are on the machine that runs it; a remote up would act on paths and setup it cannot see. | Run the command on the machine that holds the project, for example over ssh: ssh host bay -f /path/compose.yaml up. |
| Publishing a service port or a Unix socket on the host. | Every service is reached by name, so two projects never compete for a host port and a reachable service is always a named, certified one. | Connect to <project>-<service>.bay.test:<port>; a socket in a bind-mounted directory is plain Compose and works where the engine shares the host's kernel. |
| Waiting without a bound. | A wait with no limit hides a service that never starts; every wait states its bound before it begins. | Give a slow service a longer healthcheck start_period or interval. |
| Deciding a caller's policy: retries, time limits, which images to fetch. | These belong to the caller's rules; the tool states what it does and offers the switches. | bay pull, up -no-pull, healthcheck keys, exit status. |
| Changing the machine setup from a project command. | A setup change asks for administrator rights and affects every project; it happens when a person asks for it. | bay install -dry-run, then bay install. |

## Rules callers can rely on

| Rule | Reason |
| --- | --- |
| One proxy container per machine, named bay-edge. | Projects share it. Its configuration is rebuilt from every running project. |
| Routes are read from container labels, not from Compose files. | Removing a project's containers removes its routes without editing a file. |
| Domains are machine state, delegated once in /etc/resolver on macOS and named in a marked block of /etc/hosts elsewhere. | A project inherits the machine default unless its Compose file pins one. |
| Certificates are issued and reissued automatically. | One per routed domain, plus a default certificate for unrouted names. |
| Container addresses change on every start. | The proxy names backends and resolves them per request, so addresses are never pinned. |
| up, down, start, stop and restart return after the proxy serves the new configuration. | Reloading nginx is asynchronous, so the commands poll the proxy's health endpoint. |
| up, start and restart publish the route of a started service after the proxy connects to it. | A healthcheck runs inside the service's container, so the proxy runs its own connection attempt to the service's address. |
| Only machine setup requires administrator rights: /etc/resolver entries and the relay daemon on macOS; the firewall, the agent, the hosts block and the system trust store elsewhere. | Trusting the certificate authority on macOS uses the user's trust settings. |
| Every service container trusts this machine's authority unless its Compose file sets x-bay.trust: false. | A program in a container verifies against its image's store, which does not hold the authority that issues the local names. |
| Every service is reached at <project>-<service>.bay.test, by other services and by this machine, on Apple container as on Docker. | install sets Apple container's DNS domain to bay.test and Docker gives the same name as a network alias. |
| An interrupted up leaves nothing it created. | On SIGINT or SIGTERM it removes the containers it created and keeps what existed before and every volume. |
| Machine setup is applied only by install, which changes nothing that is in place; check and install -dry-run report it without changing it. | A setup step done by hand is not checked, repeated or removed by anything. |

## Diagnostics

| Symptom | Cause | Command |
| --- | --- | --- |
| A name does not resolve. | The DNS agent is not registered. | `bay doctor` |
| 502 from the proxy. | The container runs but does not listen on the resolved port. | `bay logs <service>` |
| 404 from the proxy. | No route claims that name. The service is stopped, internal, or the domain differs. | `bay status` |
| Certificate warning. | The certificate authority is not trusted, or the name has no route. | `bay install` |
| A command asks for a password. | A new domain requires an /etc/resolver entry. | `bay doctor` |

<!-- end generated -->

## Output for other programs

`bay status --json` prints the machine state. `bay brief
--json` and `bay schema --json` print this contract. These three are
the supported way for another program to read state and rules.

## Project selection

`-f` accepts a Compose file or a directory. A directory is searched for
`compose.yaml`, `compose.yml`, `docker-compose.yaml`, `docker-compose.yml` in
that order. The default is the working directory.
