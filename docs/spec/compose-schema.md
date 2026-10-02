# Compose file contract

Status: implemented.

The tables below are generated from `internal/contract`. Run
`make docs-generate` after changing a key declaration, and `make docs-check`
verifies that this file matches the code.

<!-- generated: bay schema --markdown -->

Compose files this tool reads, in search order:
`compose.yaml`, `compose.yml`, `docker-compose.yaml`, `docker-compose.yml`.

Keys this tool adds are Compose extensions, so the same file stays
readable by other Compose tools.

## Project keys

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `x-bay.domain` | string | the machine default domain | Local domain the project's service domains fall under. Naming it here pins it to the project. |
| `x-bay.extra_domains` | list of string | empty | Further local domains the project's service domains may fall under. |
| `x-bay.network` | string | default | Container network the project's services and the proxy share. |
| `name` | string | the Compose file's directory name | Project name. Prefixes every container name. |

## Service keys

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `services.<name>.x-bay.domains` | list of string | none: the service is internal | Domains the proxy routes to this service, each under one of the project's domains. Each domain gets its own certificate and route. A service without it runs and other services reach it by name, but the proxy does not route to it and issues it no certificate. |

## Service labels

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `bay.port` | integer | see the port order below | Port the service listens on inside the container. |
| `bay.tls` | boolean | false | Marks a backend that already serves HTTPS on its port. |

## Port resolution

First match wins.

1. the bay.port label
2. the first entry of `expose`
3. the container side of the first entry of `ports`
4. none: the service has no port and is not checked for connections; a service with a domain is refused

## Standard Compose keys applied

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `image` | string | required | Image to run. |
| `command` | string or list | the image's command | Process arguments. |
| `entrypoint` | string or list | the image's entrypoint | Placed before command. |
| `environment` | mapping or list | empty | Environment variables. |
| `volumes` | list of string | empty | Compose-relative bind mounts or declared named volumes, `source:target[:ro]`. Top-level volumes supports name, external, driver: local and driver_opts.size; external volumes allow only name. |
| `user` | string | image default | Process user, name or uid[:gid]. Values may use project .env and environment interpolation. |
| `read_only` | boolean | false | Mount the container root filesystem read-only. |
| `cap_drop` | list of string | empty | Linux capabilities to drop, including ALL. |
| `depends_on` | list or mapping | empty | Startup dependencies: service_started, service_healthy or service_completed_successfully. Unsupported options and cycles are rejected. |
| `healthcheck` | mapping | empty | Startup CMD/CMD-SHELL test; interval, timeout, retries, start_period and disable. No continuous background monitoring. |
| `container_name` | string | <project>-<service> | Container name. |
| `networks` | list or mapping | the project network | First entry is used. |
| `mem_limit` | string | runtime default | Memory limit. |
| `cpus` | string | runtime default | CPU count. |

## Example

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

<!-- end generated -->

## Validation

`bay` rejects a Compose file when:

- a service has no `image`;
- a project or service name contains a character other than a letter, a digit,
  `-` or `_`;
- a service's `x-bay.domains` is not a list, is empty, holds an
  invalid domain name, repeats a domain, or holds a domain outside the
  project's domains;
- two services in the project claim the same domain; the error names both
  services;
- `x-bay.extra_domains` contains an empty entry or repeats
  `x-bay.domain`.

`up`, `start` and `restart` refuse a project when a running service of another
project serves one of its domains; the error names the domain and both
services with their projects.

Keys and labels this tool does not read are ignored, so a file written for
other Compose tools loads without change. A service without
`x-bay.domains` is internal. Values of `domains` use the same `.env`
and environment interpolation as other values.
