# Peers

Status: not implemented. This document states the design; nothing below is in
the binaries yet.

Two machines on one network, each running bay, reach each other's
domains. Neither machine's network configuration changes: no router setting, no
DNS server on the network, no hosts file, no privileged port.

That is possible because both machines already answer `.test` themselves.
`/etc/resolver/test` on each sends those names to that machine's own
`bayd`, so a name that belongs to the other machine is answered by
software this project already owns.

## Identity

A peer is identified by the fingerprint of its certificate authority. The
fingerprint is what a peer is stored under, what the approval is given to, and
what the proxy admits.

The machine's name and address are attributes read from the peer. They change
without making it a different peer, they are never compared, and they never
appear in a domain. A name is shown; nothing is decided by it.

## What a machine publishes

The proxy already serves `/__bay/health` over plain HTTP. Beside it,
`/__bay/peer` returns what another machine needs:

| Field | Meaning |
| --- | --- |
| `name` | The machine's `LocalHostName`, for display |
| `address` | The address the machine is reachable at on the network |
| `domains` | The domains it serves |
| `ca` | Its certificate authority, in PEM |

## Approving a peer

One machine is given the other's address. It reads `/__bay/peer` and
shows the name, the domains and the authority's fingerprint. Approving it does
three things: the peer is stored under its fingerprint, the authority is added
to the user's trust settings, and the peer's address is admitted by the proxy.

Adding the authority to the user's trust settings needs no administrator rights,
which is how the machine's own authority is already trusted.

## Reachability

The proxy publishes ports 80 and 443. Publishing them needs no administrator
rights: the runtime's network helper already holds them.

Only an approved peer's address is admitted. A request from any other address is
refused, whatever name it asks for.

## Name resolution

`bayd` answers a name in this order:

1. A container on this machine serves it: the answer is this machine's proxy.
2. An approved peer serves it: the answer is that peer's address.
3. Otherwise: as it answers today.

The first rule comes before the second, so a name this machine serves is always
this machine's.

## Duplicate domains

Two machines can end up serving the same domain, because a machine does not ask
a peer before starting a project: a project must start whether or not the other
machine is reachable.

Nothing is refused for it. Each machine serves its own, by the resolution order
above, and the window says the name is served in more than one place. Changing
the domain on one side ends it.

## What this does not do

- No machine name in a domain. There is no `web.<machine>.test`.
- No uniqueness of machine names. Two peers may carry the same name.
- No change to the network. The router, the network's DNS and the other
  machine's system settings are untouched.
- No access for anything that is not an approved peer, including a machine that
  knows both the address and the name.

## Measured before designing

| Question | Answer |
| --- | --- |
| Can the proxy hold port 443 without administrator rights? | Yes. `container run -p 443:80` bound `*.443` and the machine's network address answered 200 |
| What does an address alone reach? | 404, offered the default certificate, which does not match the address |
| What does an address plus the right name reach? | The site |
| What does an address plus the right name reach from an address the proxy does not admit? | 403 |
