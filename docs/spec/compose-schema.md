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
| `x-bay.domain` | string | the machine default domain | Local domain this project's services use. Naming it here pins it to the project. |
| `x-bay.extra_domains` | list of string | empty | Further local domains this project's services may claim. |
| `x-bay.network` | string | default | Container network the project's services and the proxy share. |
| `name` | string | the Compose file's directory name | Project name. Prefixes every container name. |

## Service labels

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `bay.domain` | string | <service>.<project domain> | Domain the proxy routes to this service. |
| `bay.port` | integer | see the port order below | Port the service listens on inside the container. |
| `bay.internal` | boolean | false | Marks a service with no domain. It runs and other services reach it by name, but the proxy does not route to it and issues it no certificate. |
| `bay.tls` | boolean | false | Marks a backend that already serves HTTPS on its port. |

## Port resolution

First match wins.

1. the bay.port label
2. the first entry of `expose`
3. the container side of the first entry of `ports`
4. 80

## Standard Compose keys applied

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `image` | string | required | Image to run. |
| `command` | string or list | the image's command | Process arguments. |
| `entrypoint` | string or list | the image's entrypoint | Placed before command. |
| `environment` | mapping or list | empty | Environment variables. |
| `volumes` | list of string | empty | Bind mounts, `host:container[:ro]`. |
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
    # served at https://web.shop.test/

  api:
    image: node:26-slim
    expose: ["8080"]
    labels:
      bay.domain: api.shop.test

  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
    labels:
      bay.internal: "true"
```

<!-- end generated -->

## Validation

`bay` rejects a Compose file when:

- a service has no `image`;
- a project or service name contains a character other than a letter, a digit,
  `-` or `_`;
- a service's domain is outside the project's domains;
- two services in the project claim the same domain;
- a service marked `bay.internal` also sets `bay.domain`;
- `x-bay.extra_domains` contains an empty entry or repeats
  `x-bay.domain`.

Keys this tool does not read are ignored, so a file written for other Compose
tools loads without change.
