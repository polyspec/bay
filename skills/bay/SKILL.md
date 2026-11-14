---
name: bay
description: Run Compose projects as HTTPS local domains (*.test) on Apple container or Docker with bay, reach the domains of peer machines, and check or repair the machine setup. Use when starting, stopping, inspecting or debugging a Compose project it runs, a *.test name, a certificate or trust error, a peer machine, or the tool itself.
metadata:
  schemaVersion: "1"
  generated: "bay skill print; do not edit, run bay skill install"
---

# bay

bay runs Compose projects on Apple `container` and on Docker, serves each service over HTTPS at a local domain name, and reaches the domains of approved peer machines. No service port is published.

## What it is for

- **Run a Compose project as its file says, on Apple container or Docker.** A key the Compose specification defines behaves as it does there, or is rejected by name; nothing is silently ignored.
- **Give every service a name, a certificate and trust.** <project>-<service>.bay.test, known before up from the file alone; a certificate for that name from this machine's authority; containers that trust the authority.
- **Serve the domains a project declares over HTTPS.** One proxy per machine, routes from container labels, certificates issued and renewed by the tool.
- **Reach the names of approved peer machines.** Over the mutually authenticated peer link, and only to machines approved by fingerprint.
- **Be a dependable step for a person, a script or an agent.** Exit status that says whether the request was done, JSON output where a program reads the result, nothing left behind by an interrupted command, and the same result when run twice.
- **Run a project's jobs one at a time on the machine that holds it.** A service declared with x-bay.job runs to completion when queued, by request or on its schedule, with the project's other services up around it; the machine runs one job at a time, keeps each one's output, exit status, duration and peak memory, and tells the Mac when a job here or on an approved peer ends.
- **Keep a Linux machine reachable while projects use it.** The containers bay runs share a slice that slows them past 70% of memory and holds them under 80% with at most 10% in swap, and the kernel keeps a reserve for the network driver, so a loaded machine is slow but answers.
- **Apply the machine setup only on request.** install applies what is missing; every other command reads the setup and names install when a part is missing.

## What it does not do

| Not done | Why | Instead |
| --- | --- | --- |
| Running, stopping or queueing a job for a project on another machine. | A project's files, volumes, state and setup are on the machine that runs it; a remote up or job would act on paths and setup it cannot see. Reading a peer's jobs over the link changes nothing there and is done. | Run the command on the machine that holds the project, for example over ssh: ssh host bay -f /path/compose.yaml job add -key $COMMIT test, and wait from here with bay job wait -peer host <id>. |
| Publishing a service port or a Unix socket on the host. | Every service is reached by name, so two projects never compete for a host port and a reachable service is always a named, certified one. | Connect to <project>-<service>.bay.test:<port>; a socket in a bind-mounted directory is plain Compose and works where the engine shares the host's kernel. |
| Waiting without a bound. | A wait with no limit hides a service that never starts; every wait states its bound before it begins. | Give a slow service a longer healthcheck start_period or interval. |
| Deciding a caller's policy: retries, time limits, which images to fetch. | These belong to the caller's rules; the tool states what it does and offers the switches. | bay pull, up -no-pull, healthcheck keys, exit status. |
| Changing the machine setup from a project command. | A setup change asks for administrator rights and affects every project; it happens when a person asks for it. | bay install -dry-run, then bay install. |

## How to use it

1. Read before changing: `bay check` (machine setup, exit 1 when a part is
   missing), `bay status --json` (what runs and at which names),
   `bay brief --json` (this contract, with `schemaVersion`).
2. Run project commands in the directory of the project's Compose file, or pass
   `-f path/compose.yaml`.
3. Machine setup is only `bay install`: run `bay install -dry-run`
   first, then `bay install`, then `bay check`. Never set up a
   resolver, hosts entry, trust store, launchd job, systemd unit or firewall by hand.
4. Commands with access `read` are safe to run at any time. Commands with access
   `remove` can remove what a person relies on; run them when the task asks for it.
5. A command whose "Asks" is set puts a question in front of a person. With no
   one at the keyboard, ask the user to run it, for example `! bay peer add <address>`.
6. A service reaches another project's or a peer's name over HTTPS with no extra
   setup: its container trusts this machine's authority. Do not add `-k`,
   `verify=False` or copied certificates.
7. Report what a command printed and its exit status; when it fails, run the
   command standard error names before trying anything else.
8. Run a test suite, a build or other work that ends as a job: a service with
   `x-bay.job: true`, queued with `bay job add -key <commit> <service>` and read
   with `bay job wait <id>`. The machine runs one job at a time, so heavy runs
   wait their turn instead of starving each other. On another machine, queue
   it there over ssh and wait with `bay job wait -peer <name> <id>`, which
   survives a dropped link; queueing again with the same `-key` never runs twice.

## Commands

| Command | Summary | Access | Asks a person for |
| --- | --- | --- | --- |
| `bay up [-no-pull] [--json]` | Start this project's services and register their routes. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay down` | Remove this project's services and withdraw its routes. | remove | nothing |
| `bay start [service...]` | Start services. With no name, starts every service in the project. | change | nothing |
| `bay stop [service...]` | Stop services. The containers remain and can be started again. | change | nothing |
| `bay restart [service...]` | Stop then start services. | change | nothing |
| `bay logs [-f] [-n N] <service>` | Print one service's output. | read | nothing |
| `bay plan [--json]` | Print each service's name, port, domains and certificate paths as up will create them. | read | nothing |
| `bay job add [-wait] [-key k] [-timeout d] <service> \| list [-peer name] \| show [-peer name] <id> \| logs [-f] [-peer name] <id> \| wait [-peer name] [-timeout d] <id> \| cancel <id> \| schedule \| unschedule` | Queue a project's job services and read how they ran. The machine runs one job at a time, oldest first. | change | nothing |
| `bay exec [-u user] [-w dir] [-e KEY=VALUE] <service> <command...>` | Run a command in a service's running container and exit with its status. | change | nothing |
| `bay pull [service...]` | Fetch the images of this project's services from their registries. | change | nothing |
| `bay images [--json] [service...]` | Report whether the local store holds each service's image. Fetches nothing. | read | nothing |
| `bay domain [add\|remove\|default] [name]` | List the machine's domains, or change them. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay status [--json]` | Print the proxy, the DNS server and every registered project. | read | nothing |
| `bay doctor` | Report what machine setup is missing. Changes nothing. | read | nothing |
| `bay check [-scope all\|run]` | Report each part of the machine setup and fail when any is missing. Changes nothing. | read | nothing |
| `bay peer [open\|close\|find\|add\|remove] [address\|name\|fingerprint]` | List the machines whose domains this one reaches, or change them. | remove | approval of the other machine's fingerprint, read from standard input; with no one to answer, nothing is approved |
| `bay sync` | Rewrite the proxy configuration from the containers that are running. | change | nothing |
| `bay cert client <name> [-out dir] [--json]` | Write a client certificate the machine's authority issues, for mutual TLS from outside the containers. | change | nothing |
| `bay install [-dry-run] [-only-from host,...\|none]` | Apply the machine setup that is missing. Running it again changes nothing. | change | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay uninstall` | Remove the machine setup install wrote. | remove | the administrator password, in a system dialog or on the terminal, when a step needs root |
| `bay brief [--json]` | Print the full usage contract on one screen. | read | nothing |
| `bay schema [--json]` | Print the Compose extension contract. | read | nothing |
| `bay skill [print\|install]` | Print the agent skill, or write it where agents read skills. | change | nothing |
| `bay mcp [install]` | Serve the commands to an agent as an MCP server on standard input and output, or register it. | change | nothing |
| `bay help [command]` | Print help for one command, or the command list. | read | nothing |
| `bay version [--json]` | Print this executable's version, commit, Go version, platform and contract schema version. | read | nothing |

## Flags before the command

- `-f path` The Compose file, or a directory to find one in. Default: the working directory.
- `-p name` The project name, instead of the Compose file's name or directory. Two runs of one file under two names do not overlap: their containers, default volume names and registrations are separate, and down -p removes only its own.
- `-state dir` The state directory. Default: ~/.bay.

## Exit status

- `0` The command did what it states. For check, every part of the setup is in place.
- `1` The command failed, or check found a part missing. Standard error says why and names the command that reports more.
- `2` The command line names no command or an unknown one.

## Compose file

A project is a Compose file. A service is served at the domains listed in its
`x-bay.domains`; a service without the list is internal and is reached
by other services at `<project>-<service>.bay.test:<port>`. Run
`bay schema` for every key and its default.

```yaml
name: shop

x-bay:
  domain: shop.test          # optional; the machine default is used otherwise

services:
  web:
    image: node:26-slim
    ports: ["3000"]          # the container port; no host port is published
    environment:
      DATABASE_URL: postgres://shop-db.bay.test:5432/app
    volumes:
      - ./src:/app/src
    command: ["npm", "run", "dev"]
    x-bay:
      domains: [web.shop.test]
    # served at https://web.shop.test/

  api:
    image: node:26-slim
    expose: ["8080"]
    x-bay:
      domains:
        - api.shop.test
        - admin.shop.test
    # served at https://api.shop.test/ and https://admin.shop.test/

  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
    # no domains: internal, reached at shop-db.bay.test:5432
```

## Rules the tool holds to

- One job at a time per machine, in the order jobs were queued. Two heavy runs on one machine compete for its memory; one after the other each gets it. A job keeps its result on the machine that ran it until it is read, so a link that drops loses nothing.
- One proxy container per machine, named bay-edge. Projects share it. Its configuration is rebuilt from every running project.
- Routes are read from container labels, not from Compose files. Removing a project's containers removes its routes without editing a file.
- Domains are machine state, delegated once in /etc/resolver on macOS and named in a marked block of /etc/hosts elsewhere. A project inherits the machine default unless its Compose file pins one.
- Certificates are issued and reissued automatically. One per routed domain, plus a default certificate for unrouted names.
- Container addresses change on every start. The proxy names backends and resolves them per request, so addresses are never pinned.
- up, down, start, stop and restart return after the proxy serves the new configuration. Reloading nginx is asynchronous, so the commands poll the proxy's health endpoint.
- up, start and restart publish the route of a started service after the proxy connects to it. A healthcheck runs inside the service's container, so the proxy runs its own connection attempt to the service's address.
- Only machine setup requires administrator rights: /etc/resolver entries and the relay daemon on macOS; the firewall, the agent, the hosts block and the system trust store elsewhere. Trusting the certificate authority on macOS uses the user's trust settings.
- Every service container trusts this machine's authority unless its Compose file sets x-bay.trust: false. A program in a container verifies against its image's store, which does not hold the authority that issues the local names.
- Every service is reached at <project>-<service>.bay.test, by other services and by this machine, on Apple container as on Docker. install sets Apple container's DNS domain to bay.test and Docker gives the same name as a network alias.
- An interrupted up leaves nothing it created. On SIGINT or SIGTERM it removes the containers it created and keeps what existed before and every volume.
- Machine setup is applied only by install, which changes nothing that is in place; check and install -dry-run report it without changing it. A setup step done by hand is not checked, repeated or removed by anything.

## Symptoms

| Symptom | Cause | Run |
| --- | --- | --- |
| A job stays queued. | A job queued before it runs, or the agent that runs the queue is not running. | `bay job list` |
| A name does not resolve. | The DNS agent is not registered. | `bay doctor` |
| 502 from the proxy. | The container runs but does not listen on the resolved port. | `bay logs <service>` |
| 404 from the proxy. | No route claims that name. The service is stopped, internal, or the domain differs. | `bay status` |
| Certificate warning. | The certificate authority is not trusted, or the name has no route. | `bay install` |
| A command asks for a password. | A new domain requires an /etc/resolver entry. | `bay doctor` |

## Files the tool owns

Do not edit these; the commands write them and an edit is replaced or breaks the check.

- `~/.bay/ca.crt, ca.key` The certificate authority.
- `~/.bay/certs/` Issued certificates.
- `~/.bay/conf.d/stack.conf` The generated proxy configuration.
- `~/.bay/machine.json` The machine's domains and settings.
- `~/.bay/groups.json` Registered projects.
- `~/.bay/peers.json, peers/` Approved peers and their authorities.
- `~/.bay/trust/` The certificates service containers trust.
- `~/.bay/jobs/` The job queue: one file per job, its output beside it, and the projects whose schedules are followed.
- `~/.claude/skills/bay/, ~/.agents/skills/bay/` The agent skill, where those directories exist.
- `/etc/resolver/<domain>` macOS: delegates the domain to the DNS server.
- `~/Library/LaunchAgents/dev.bay.dns.plist` macOS: runs the DNS server.
- `/Library/LaunchDaemons/dev.bay.relay.plist` macOS: runs the relay containers use to reach peers.
- `/etc/systemd/system/bay-agent.service` Linux: runs the agent that keeps the hosts block and the peer document.
- `/etc/hosts (marked block)` Linux: names the domains the proxy serves at 127.0.0.1, and each running service container at <container>.bay.test with its address, so a program on the host reaches an internal service by the name a container uses.
- `/etc/bay/firewall.nft, bay-firewall.service` Linux: accepts new connections only from the hosts install -only-from named.
- `/usr/local/share/ca-certificates/bay.crt` Linux: the authority in the system trust store; the path follows the system.
- `/etc/systemd/system/bay.slice` Linux: holds the containers bay runs below the machine's memory; past 70% they slow, at 80% the kernel kills inside the slice only, and swap is held to 10%.
- `/etc/sysctl.d/99-bay.conf` Linux: the free memory the kernel keeps for allocations that cannot wait, such as the network driver's, and when it starts reclaiming.
