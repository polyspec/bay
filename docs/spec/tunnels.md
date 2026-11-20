# Tunnels

[Korean](tunnels.ko.md).

Status: implemented. Not run against the public tunnel service from this
repository's tests; the unit tests cover the command against a stand-in engine.

`bay tunnel <domain>` publishes one served domain of the project at a public
address and prints the address. The publication ends when the command ends, or
when the project's containers are removed.

## What it is for

A served domain answers on this machine and on approved peers. A reader outside
has no way to reach it: the machine publishes one port to the network, the peer
link (architecture rule 7), and the peer link speaks mutual TLS between
machines this one approved. A tunnel gives a running service a public address
for as long as one command runs, and opens no port: the tunnel client makes one
outbound connection to the tunnel service's edge and keeps it.

```
reader → the tunnel service's edge    holds the public name and its certificate
           ▲
           │ one outbound connection, held by the client
         <project>-tunnel container   runs on this machine, on the engine and
           │                          network of the routed service
           ▼
         <container>.bay.test:<port>  the same name and port the proxy's route
                                      names; the runtime resolves it
```

## The command

`bay tunnel <domain>` runs in the directory of the project's Compose file, as
`up` does. The domain must be one the proxy serves for a running container of
this project: a domain with no route, a domain whose container is stopped, and
a domain held by another project or lost to a conflict are all refused with the
domain named. A project with a publication running is refused a second one
until it ends.

The publication is one container, named `<project>-tunnel`, created on the
engine of the routed container and on the routed service's network, labelled
`bay.role=service`, `bay.group` and `bay.service=tunnel` with an empty
`bay.domain` label: it is an internal service of the project, it gets no route,
and it appears in the project's container list as the service `tunnel`.

The container runs the tunnel client `cloudflare/cloudflared:2026.11.1`. Its
arguments name the routed service's own address, `--url
<scheme>://<container>.bay.test:<port>`, with the route's scheme. The machine's
authority is mounted read-only into the container and named with
`--origin-ca-pool`, and the backend name with `--origin-server-name`, so a
service that serves its own TLS is reached with a certificate the client
verifies against this machine's authority. The image is fetched when the local
store lacks it, with `--platform` on Apple `container` for the same reason a
service pull carries it; `-no-pull` refuses the fetch.

The command prints the public address once the client names it in its output.
Until then it states that it is waiting. The client's later output is not
printed; if the client ends before an address is named, the command reports
that with the output it collected, and removes the container. A signal that
ends the command stops and removes the container, so an interrupted
publication leaves nothing behind.

The publication ends the same way when anything else removes the container:
`down` and `stop` of the project treat it as one of the project's service
containers, and the next `up` removes it as a container of a service the
Compose file does not declare.

## What the reader reaches

The tunnel forwards to the service container itself, on the declared port —
the same destination the proxy's route names. The proxy is not in this path,
so the reader's response carries none of the proxy's headers, and the public
name is the tunnel service's, not a domain of this machine. The certificate the
reader verifies is the tunnel service's; between the client and the service,
this machine's authority covers the backend name when the service serves its
own TLS.

Confidential paths are unchanged: names between machines go over the peer
link, and a tunnel publishes exactly one domain while its command runs.
