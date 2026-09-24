# Using bay

Adding a project, serving it over HTTPS, and the daily commands.

## Check the machine first

```sh
bay doctor     # prints "nothing to do" when the machine is ready
bay status     # the proxy, and every project already registered
bay domain     # the domains this machine serves
```

`doctor` and `status` only read state. They do not register the selected Compose
file, create a certificate authority, or read its private key. Missing or
unreadable public certificates appear in the report; `status --json` includes
`certificates.authority.readError` when the public CA cannot be read. `doctor`
includes the selected project's domains in its setup report without saving them.
When it reports missing steps, run `bay install`; see [Install](install.md).
`install` applies the setup for the registered projects and registers no
project. A project is registered by `up`, or by Add project in the menu bar
window.

`status` lists the domains other projects already serve. Choose a different name
for yours: `up` refuses a domain another running project serves and names that
project.

## Add a project

A project is a Compose file. A service is served at the domains listed in its
`x-bay.domains`. The project domain is the machine default unless the
file names one.

```yaml
name: myapp

services:
  web:
    image: node:26-slim
    ports: ["3000"]
    command: ["npm", "run", "dev"]
    volumes:
      - ./src:/app/src
    x-bay:
      domains: [web.test]
```

```sh
cd ~/work/myapp
bay up
```

`https://web.test/` is served.

`ports: ["3000"]` states the port the container listens on. Nothing is published
to a host port: each container has its own address, and the proxy connects to it
directly.

## Choose the names a service is served at

`x-bay.domains` lists every name of a service. The proxy routes each
name to the same container and issues a certificate for each:

```yaml
  web:
    image: node:26-slim
    x-bay:
      domains:
        - shop.test
        - admin.shop.test
```

The container receives the requested name in the `Host` header. Every name must
be under one of the project's domains. A name another service claims, in this
project or in a running project, is refused with both services named.

## Services with no domain

A database, a queue or a worker lists no domains. It is internal: it receives no
route and no certificate:

```yaml
  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
```

The container runs and other services reach it by name.

## Service to service

Reach another service at `<project>-<service>.container.test:<port>`:

```yaml
  web:
    environment:
      DATABASE_URL: postgres://myapp-db.container.test:5432/app
```

Container addresses are assigned by DHCP and change on every start. Use the
name; the proxy and the services resolve it on each request.

A restarted service takes a few seconds to become reachable by name again.
Connections from other services fail with a timeout until the runtime publishes
the new address.

## Starting and running

A container reports as running as soon as the runtime starts it, which is before
the process inside listens on its port. During that window the proxy returns 502
and a connection from another service fails.

`status` reports such a service as `starting` and states that it is not
accepting connections yet:

```
running  routed   web    192.168.64.89   web.test
starting -        api    192.168.64.74   api.test · not accepting connections yet
```

`up`, `start` and `restart` wait up to 20 seconds for the processes to accept
connections. A service that takes longer is named in the output and the command
returns; the containers are running and the service becomes available on its
own.

## Ports

The container port is read from the first of:

1. the `bay.port` label;
2. the first entry of `expose`;
3. the container side of the first entry of `ports`;
4. 80.

The host side of `ports` is ignored.

## Domains

A domain is delegated once for the whole machine. Manage the list from the
command line:

```sh
bay domain                    # list them
bay domain add lab.test       # delegate another; asks for the password
bay domain default lab.test   # what projects use when they name none
bay domain remove lab.test
```

Adding a domain writes `/etc/resolver/<domain>`, which tells macOS to resolve
names under it with this tool. That write requires administrator rights, so the
command asks for the password once per domain. `remove` refuses the default
domain and a domain a project pins.

A project uses its own domain by naming it in the Compose file:

```yaml
x-bay:
  domain: myteam.test
```

Its services are then served at `<service>.myteam.test`.

A two-label domain such as `myteam.test` also covers names that have no route:
they receive 404. A single-label domain such as `test` does not, because clients
reject a wildcard whose parent is one label, so a mistyped name produces a
certificate warning instead.

The application's **Domains** screen performs the same actions.

## Daily commands

```sh
bay up                 # start the project and register its routes
bay down               # remove the project and withdraw its routes
bay stop db            # stop a service; the container remains
bay start db           # start it again
bay restart web
bay logs -f web        # follow one service's output
bay status             # the proxy and every registered project
bay status --json      # the same, for another program
bay sync               # rewrite the proxy configuration, change no container
```

The application performs the same actions. The project screen carries the start,
restart and stop actions in its header; each service row carries Logs, and
selecting a service opens its route, its container and the tail of its output.

## How a request is served

```
browser
  → /etc/resolver/<domain>     delegates the domain to the local DNS server
  → bayd               returns the proxy's address
  → bay-edge:443      selects the service by Host header
  → <project>-<service>.container.test
  → the container
```

One proxy serves the whole machine and every project shares it. Its
configuration is generated from the labels on running containers, so removing a
project removes its routes without editing a file, and two projects cannot
overwrite each other's configuration.

Plain HTTP on port 80 returns 308 to the HTTPS address.

## When something fails

[Troubleshooting](troubleshooting.md) lists each symptom with its cause and the
command that reports more. The three that cover most cases: a name that does not
resolve means the DNS agent is not registered, 502 means the container is not
listening on the resolved port, and 404 means no route claims the name.

## Handing the project to someone else

Commit the Compose file. Do not commit certificates or keys: each machine issues
its own from its own authority.

```sh
git clone ... && cd myapp
bay up
```

The first `up` asks for the password when the project uses a domain the machine
does not delegate yet.
