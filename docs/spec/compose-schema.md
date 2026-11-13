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
| `services.<name>.x-bay.trust` | boolean | true | Whether the container trusts the certificates this machine trusts: the system's public roots and this machine's authority, which issues the certificates of its projects and of its peers' names. They are mounted read-only over /etc/ssl/certs/ca-certificates.crt, /etc/pki/tls/certs/ca-bundle.crt and /etc/ssl/cert.pem and named by SSL_CERT_FILE, CURL_CA_BUNDLE, REQUESTS_CA_BUNDLE and NODE_EXTRA_CA_CERTS; for Java a PKCS #12 store is mounted at /etc/bay/truststore.p12 and named by JAVA_TOOL_OPTIONS. A path the service mounts itself and a variable it sets itself are left to the service. false gives the container its image's store only. |
| `services.<name>.x-bay.certificate` | boolean | true | Whether the container is given a certificate for its own name, issued by this machine's authority for <project>-<service>.bay.test, the container name and the service's domains, usable as a server and as a client. It is mounted read-only at /etc/bay/tls/cert.pem, its key at /etc/bay/tls/key.pem (readable by every user in the container) and the authority at /etc/bay/tls/ca.pem, and reissued at up when 30 days or less remain. Redis takes them as --tls-cert-file, --tls-key-file and --tls-ca-cert-file; MySQL and MariaDB as --ssl-cert, --ssl-key and --ssl-ca. PostgreSQL refuses a key other users can read, so its command first copies the key to a file it owns: sh -c 'install -o postgres -m 600 /etc/bay/tls/key.pem /tmp/key.pem && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/etc/bay/tls/cert.pem -c ssl_key_file=/tmp/key.pem'. A path the service mounts itself is left to the service. false gives no certificate. |

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
| `tmpfs` | string or list of string | empty | In-memory mounts at absolute paths in the container, for data that must not outlive it. Docker takes mount options after a colon (/run:size=64m); Apple container takes the path alone and an option is rejected. |
| `depends_on` | list or mapping | empty | Startup dependencies: service_started, service_healthy or service_completed_successfully. Unsupported options and cycles are rejected. |
| `healthcheck` | mapping | empty | Startup CMD/CMD-SHELL test; interval (default 30s), timeout (30s), retries (default 3, from 1 to 100), start_period (0) and disable. up, start and restart run it, as an exec in the container, right after the service starts and before the services that come after it in dependency order. The first attempt runs at once, not after one interval. An attempt that fails or takes longer than timeout is a failure; a failure while start_period has not passed since the start is not counted. The next attempt follows interval after a failure. The service is healthy at the first attempt that succeeds; it fails after retries counted failures, or when its container stops, or after start_period + retries x (interval + timeout) + 1s, and the command fails with the service's output. The bound is in the file on purpose: a wait with no limit hides a service that never starts; a slow service gets a longer start_period. There is no monitoring after startup. |
| `container_name` | string | <project>-<service> | Container name. |
| `networks` | list or mapping | the project network | First entry is used. |
| `mem_limit` | string | runtime default | Memory limit. |
| `cpus` | string | runtime default | CPU count. |
| `stop_grace_period` | duration | runtime default (5s) | Time the container gets to stop before it is killed, as a whole number of seconds such as 30s or 1m30s. Stop, restart, down and replacement pass it to the runtime. |
| `stop_signal` | string | runtime default | Signal sent to stop the container: one of SIGHUP, SIGINT, SIGQUIT, SIGTERM, SIGUSR1, SIGUSR2, SIGKILL. Stop, restart, down and replacement pass it to the runtime as --signal. |

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
