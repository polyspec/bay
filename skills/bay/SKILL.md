---
name: bay
description: Run Compose projects as HTTPS local domains (*.test) on Apple container or Docker with bay, reach the domains of peer machines, and check or repair the machine setup. Use when starting, stopping, inspecting or debugging a Compose project it runs, a *.test name, a certificate or trust error, a peer machine, or the tool itself.
metadata:
  schemaVersion: "1"
  generated: "bay skill print; do not edit, run bay skill install"
---

# bay

bay runs Compose projects on Apple `container` and on Docker, serves each service over HTTPS at a local domain name, and reaches the domains of approved peer machines. No service port is published.

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

## Commands

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

## Exit status

- `0` The command did what it states. For check, every part of the setup is in place.
- `1` The command failed, or check found a part missing. Standard error says why and names the command that reports more.
- `2` The command line names no command or an unknown one.

## Compose file

A project is a Compose file. A service is served at the domains listed in its
`x-bay.domains`; a service without the list is internal and is reached
by other services at `<project>-<service>.container.test:<port>`. Run
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
      DATABASE_URL: postgres://shop-db.container.test:5432/app
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
    # no domains: internal, reached at shop-db.container.test:5432
```

## Rules the tool holds to

- One proxy container per machine, named bay-edge. Projects share it. Its configuration is rebuilt from every running project.
- Routes are read from container labels, not from Compose files. Removing a project's containers removes its routes without editing a file.
- Domains are machine state, delegated once in /etc/resolver on macOS and named in a marked block of /etc/hosts elsewhere. A project inherits the machine default unless its Compose file pins one.
- Certificates are issued and reissued automatically. One per routed domain, plus a default certificate for unrouted names.
- Container addresses change on every start. The proxy names backends and resolves them per request, so addresses are never pinned.
- up, down, start, stop and restart return after the proxy serves the new configuration. Reloading nginx is asynchronous, so the commands poll the proxy's health endpoint.
- up, start and restart publish the route of a started service after the proxy connects to it. A healthcheck runs inside the service's container, so the proxy runs its own connection attempt to the service's address.
- Only machine setup requires administrator rights: /etc/resolver entries and the relay daemon on macOS; the firewall, the agent, the hosts block and the system trust store elsewhere. Trusting the certificate authority on macOS uses the user's trust settings.
- Every service container trusts this machine's authority unless its Compose file sets x-bay.trust: false. A program in a container verifies against its image's store, which does not hold the authority that issues the local names.
- Machine setup is applied only by install, which changes nothing that is in place; check and install -dry-run report it without changing it. A setup step done by hand is not checked, repeated or removed by anything.

## Symptoms

| Symptom | Cause | Run |
| --- | --- | --- |
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
- `~/.claude/skills/bay/, ~/.agents/skills/bay/` The agent skill, where those directories exist.
- `/etc/resolver/<domain>` macOS: delegates the domain to the DNS server.
- `~/Library/LaunchAgents/dev.bay.dns.plist` macOS: runs the DNS server.
- `/Library/LaunchDaemons/dev.bay.relay.plist` macOS: runs the relay containers use to reach peers.
- `/etc/systemd/system/bay-agent.service` Linux: runs the agent that keeps the hosts block and the peer document.
- `/etc/hosts (marked block)` Linux: names the domains the proxy serves.
- `/etc/bay/firewall.nft, bay-firewall.service` Linux: accepts new connections only from the hosts install -only-from named.
- `/usr/local/share/ca-certificates/bay.crt` Linux: the authority in the system trust store; the path follows the system.
