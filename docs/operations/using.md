# Using bay

Adding a project, running it, and the daily commands.

## If it is already installed on this machine

Nothing to install and nothing to set up. Check with:

```sh
bay doctor     # prints "nothing to do" when the machine is ready
bay status     # the proxy, and every project already registered
```

`status` also shows the domains other projects already serve. Pick a different
name for yours; `up` refuses a domain another running project serves and names
that project.

The menu bar application is `/Applications/baybar.app` when it was
installed with `make install`. Open it for the same actions in a window.

Everything else is in `bay brief`.

## Add a project

A project is a Compose file. A service with no extra settings is served at
`<service>.<machine default domain>`.

```yaml
name: shop

services:
  web:
    image: node:26-slim
    ports: ["3000"]
    command: ["npm", "run", "dev"]

  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
    labels:
      bay.internal: "true"
```

```sh
cd ~/work/shop
bay up
```

`https://web.test/` is served. `db` runs without a domain because it is marked
internal.

Run `bay schema` for every key this tool reads.

## Domains

A domain is delegated once per machine. A project uses the machine default
unless its Compose file pins one:

```yaml
x-bay:
  domain: shop.test
```

Manage the machine's domains from the command line:

```sh
bay domain                    # list them
bay domain add lab.test       # delegate another; asks for the password
bay domain default lab.test   # what projects use when they name none
bay domain remove lab.test
```

The application's **Domains** screen does the same. Adding a domain writes an
`/etc/resolver` entry, which is why it asks for the password. `remove` refuses
the default domain and a domain a project pins.

A domain with two labels, such as `shop.test`, also gives a valid wildcard
certificate for names that have no route. A single-label domain such as `test`
does not, so a mistyped name produces a certificate warning instead of a 404.

## Ports

The container port is read from the first of:

1. the `bay.port` label;
2. the first entry of `expose`;
3. the container side of the first entry of `ports`;
4. 80.

Host ports are ignored. Each container has its own address, so nothing is
published to the host.

## Service to service

Reach another service at `<project>-<service>.container.test:<port>`:

```yaml
    environment:
      DATABASE_URL: postgres://shop-db.container.test:5432/app
```

Container addresses change on every start, so use the name.

## Daily commands

```sh
bay up                 # start the project and register its routes
bay down               # remove the project and withdraw its routes
bay restart web        # one service
bay stop db            # keep the container, withdraw the route
bay logs -f web        # follow the output
bay status             # the proxy and every registered project
bay status --json      # the same, for another program
bay doctor             # what setup is missing
```

The application performs the same actions. The project screen carries Start,
Restart and Stop for the project; each service row carries Start, Stop and
Logs.

## Onboarding another person

Commit the Compose file. Do not commit certificates or keys: each machine issues
its own from its own authority.

```sh
git clone ... && cd shop
bay up
```

The first `up` asks for the password when the project uses a domain the machine
does not delegate yet.
