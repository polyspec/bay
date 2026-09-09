# Command line contract

Status: implemented.

Read-only commands follow the [diagnostic rules](diagnostics.md).

The tables below are generated from `internal/contract`. Run
`make docs-generate` after changing a command declaration, and `make docs-check`
verifies that this file matches the code.

<!-- generated: bay brief --markdown -->

| Command | Summary | Administrator rights |
| --- | --- | --- |
| `up` | Start this project's services and register their routes. | Can require |
| `down` | Remove this project's services and withdraw its routes. | No |
| `start [service...]` | Start services. With no name, starts every service in the project. | No |
| `stop [service...]` | Stop services. The containers remain and can be started again. | No |
| `restart [service...]` | Stop then start services. | No |
| `logs [-f] [-n N] <service>` | Print one service's output. | No |
| `domain [add|remove|default] [name]` | List the machine's domains, or change them. | Can require |
| `status [--json]` | Print the proxy, the DNS server and every registered project. | No |
| `doctor` | Report what machine setup is missing. Changes nothing. | No |
| `peer [open|close|find|add|remove] [address|name|fingerprint]` | List the machines whose domains this one reaches, or change them. | No |
| `sync` | Rewrite the proxy configuration from the containers that are running. | No |
| `install` | Apply the machine setup now instead of during the next up. | Can require |
| `uninstall` | Remove the DNS agent and the resolver entries. | Can require |
| `brief [--json]` | Print the full usage contract on one screen. | No |
| `schema [--json]` | Print the Compose extension contract. | No |
| `help [command]` | Print help for one command, or the command list. | No |

## Effects

### `up`

Reuses unchanged owned containers and proven completed initializers; replaces changed configuration or local image digests. Starts dependencies in order, checks declared health, issues certificates and updates the proxy. Delegates new project domains.

### `down`

Removes the project's containers. Removes the proxy when no route remains.

### `start`

Reconciles selected services and their dependencies, reuses unchanged containers, checks startup conditions, then updates the proxy.

### `stop`

Stops containers and withdraws their routes.

### `restart`

Recreates selected owned containers, including explicitly retried initializers, after preparing their dependencies; updates the proxy.

### `domain`

add writes an /etc/resolver entry for the domain. remove deletes it. default changes the domain projects use when their Compose file names none.

### `status`

Reads state without registering the selected Compose project, creating certificates, or changing machine setup. Missing or unreadable public certificates are reported; private keys are not read.

### `doctor`

Reads public certificate and setup state, including the selected project's domains without saving them. Creates no files and reads no private keys.

### `peer`

With no argument it lists them and says whether the link is open. "open" answers the link and announces this machine to the network, which another machine reads to approve this one; "close" stops both. "find" lists the machines announcing themselves. "add" takes an address, or the name of a machine that announced itself, reads what it says about itself, checks that it holds the authority it names, prints the fingerprint and asks before approving. "remove" withdraws an approval. Every change rewrites the proxy configuration. Nothing is added to the keychain.

### `sync`

Writes the server blocks and issues any certificate a route needs, then reloads the proxy and waits until it serves the new configuration. Starts, stops and changes no container.

### `install`

Writes /etc/resolver entries, adds the certificate authority to the user's trust settings, and registers the DNS launchd agent.

### `uninstall`

Removes /etc/resolver entries and the launchd agent. The certificate authority stays in the trust settings.

## Rules callers can rely on

| Rule | Reason |
| --- | --- |
| One proxy container per machine, named bay-edge. | Projects share it. Its configuration is rebuilt from every running project. |
| Routes are read from container labels, not from Compose files. | Removing a project's containers removes its routes without editing a file. |
| Domains are machine state, delegated once in /etc/resolver. | A project inherits the machine default unless its Compose file pins one. |
| Certificates are issued and reissued automatically. | One per routed domain, plus a default certificate for unrouted names. |
| Container addresses change on every start. | The proxy names backends and resolves them per request, so addresses are never pinned. |
| up, down, start, stop and restart return after the proxy serves the new configuration. | Reloading nginx is asynchronous, so the commands poll the proxy's health endpoint. |
| Only /etc/resolver writes require administrator rights. | Trusting the certificate authority uses the user's trust settings. |

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
