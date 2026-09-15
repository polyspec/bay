# Peers

Status: implemented on a machine running one engine. `bay peer` and
the window's Peers screen both open the link, approve a machine and withdraw an
approval. On a machine running both engines the link is refused, because rule 8
of the architecture puts it on a host process that reaches both and that process
is not built.

A response that crosses the link is answered 200 whether or not the other
machine finished sending it, so a body shorter than its own length arrives as a
success. Settings holds a switch that records one line per such request. Two
things in it report a loss. When the other machine declares a length, that
length and the length that reached the client are both counted in body bytes and
differ exactly when bytes were lost. When it declares none, the declared length
is empty and the error the proxy writes for that request is the only signal:
nginx reports the request itself as complete either way. The switch is off until
it is turned on, because it writes a line for every request over the link.

What an approved machine holds is read from that machine, every 30 seconds, by
the resident agent. The announcement carries only where to look, so a domain
added or withdrawn over there reaches here by reading the document again. The
authority behind the document must be the one that was approved: an address that
answers with another authority is another machine, and what it states about its
domains is not the approved machine's to change. A machine that does not answer
keeps what it holds, because it has not given up a domain, it has not been
asked.

Reading the record is one command against the engine the proxy runs on. A
request that lost bytes leaves an error beside its line:

```sh
docker logs bay-edge | grep -E '^link |prematurely closed'
```

A machine whose peer declares a length is read from the line alone: `declared`
and `sent` are both counted in body bytes and differ only when bytes were lost.
A machine whose peer sends chunked leaves `declared` empty, and the error is
what reports the loss.

Two machines on one network, each running bay, reach each other's
domains. Neither machine's network configuration changes: no router setting, no
DNS server on the network, no hosts file, no privileged port.

That is possible because both machines already answer `.test` themselves.
`/etc/resolver/test` on each sends those names to that machine's own
`bayd`, so a name that belongs to the other machine is answered by
software this project already owns.

## Identity

A peer is identified by the fingerprint of its certificate authority. The
fingerprint is what a peer is stored under and what the approval is given to.

The machine's name and address are attributes read from the peer. They change
without making it a different peer, they are never compared, and they never
appear in a domain. A name is shown; nothing is decided by it.

## Why a peer is not recognised by its address

The runtime rewrites the source address of a published port. A request from the
network and a request from this machine both reach the proxy as the network
gateway, so the proxy cannot tell them apart. An address is therefore not used
for any decision.

A peer proves who it is by holding a certificate its own authority issued. That
is what a machine checks before serving anything.

## What a machine publishes

Each machine publishes one port on the network, the peer link. Two things answer
on it.

| Path | Client certificate | Content |
| --- | --- | --- |
| `/__bay/peer` | Not required | `name`, `address`, `domains`, and the authority in PEM |
| Everything else | Required, from an approved peer's authority | The domains this machine serves |

The peer endpoint has to answer before there is any approval, so it cannot ask
for one.

## Two ways to find a machine

A machine with the link open announces itself to the network every second and a
half. `bay peer find` lists what it hears, and a machine that stops
announcing leaves the list, so one that was shut down is not offered.

The announcement is a broadcast rather than a multicast: access points commonly
prune multicast between clients while still flooding broadcast to the subnet, so
this reaches machines where a multicast would not.

An address given by hand is the other way, and it is the one that works across
subnets and where a network filters broadcast. Neither replaces the other, and
what is announced is only where to look: the authority is read from the machine
itself.

The address a peer announces is also followed. A peer that comes back at another
address, which is what happens when the network hands out a different one, is
matched by its authority's fingerprint and its stored address is replaced.

## Opening the link

The link is closed until it is opened. `bay peer open` answers it,
announces this machine, and prints the address to give the other machine;
`bay peer close` stops both.

Opening is refused on a machine running more than one engine, and the refusal
comes before anything is written: the link is one port and no engine's proxy
serves the other engine's domains, so a link served from one of them would
answer some of this machine's names and not the rest. Closing is never refused,
because a machine that acquired a second engine while the link was open has to
be able to close it.

With the link open and no peer approved, only the document below answers. This
machine's own domains appear on the link when there is an authority to check a
client against, which is what makes the first approval possible: a machine has
to answer before anyone can approve it.

## Approving a peer

One machine is given the other's address and reads `/__bay/peer`. It
cannot yet verify what it reads, so it shows the name, the domains and the
authority's fingerprint, and asks.

Approving stores the peer under that fingerprint and writes the peer's authority
into two places the proxy reads: the authorities whose client certificates are
accepted on the peer link, and the authorities a peer's own certificate is
verified against when this machine connects to it.

Nothing is written to the keychain. The peer's authority is a file the proxy
reads, not a trust setting on the machine.

An address that already carries an approved authority, now answering with
another one, is a different machine. That is said before the question, because
approving it silently would put two machines under one address.

## Certificates

Each machine keeps one authority, which its own system already trusts.

For a domain a peer serves, this machine issues a certificate **from its own
authority**. A browser here is therefore offered a certificate from the
authority this machine already trusts, and no peer's authority is installed
anywhere.

Each machine also holds one client certificate from its own authority. That is
what it presents on a peer's link, and what the peer matches against the
authorities it has approved.

## Name resolution

`bayd` answers a name in this order:

1. A container on this machine serves it: the answer is this machine's proxy.
2. An approved peer serves it: the answer is this machine's proxy as well.
3. Otherwise: as it answers today.

Both answers name the local proxy. For a peer's domain the proxy holds the
certificate it issued for that name, terminates the request, and forwards it
over the peer's link with this machine's client certificate.

The first rule comes before the second, so a name this machine serves is always
this machine's.

## The window

The Peers screen shows the link, the approved machines and what is being heard
on the network, in that order.

The link section carries the address to give another machine and the interval
between announcements, or a line saying the link is closed when it is. One
button opens and closes it.

An approved machine is a row with its name, its address, its domains and the
first characters of its authority's fingerprint, and a button that withdraws the
approval. Withdrawing asks first.

A machine heard announcing itself that is not approved is a row with an approve
button. Approving asks first, and the question carries the fingerprint read from
that machine, so what is approved is the authority and not the name. A machine
that announces nothing is still approved by address from the command line.

The sidebar's Peers row carries the number of approved machines and a dot: green
while the link is open, off while it is closed.

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
- No authority of one machine installed on another.
- No access without a client certificate from an approved authority, including
  from a machine that knows both the address and the name.

## Measured before designing

| Question | Answer |
| --- | --- |
| Does the source address survive a published port? | No. A request from the network and a request from this machine both arrived as `192.168.64.1`, the network gateway |
| Can the proxy hold a port below 1024 without administrator rights? | Yes. `container run -p 443:80` bound `*.443` and the machine's network address answered 200 |
| Can a machine require a peer's certificate? | Yes. Without one nginx answered 400; with one signed by an approved authority it served and named the peer, `CN=peer-b` |
| Does a browser need the peer's authority? | No. A client trusting only its own machine's authority asked for `web.test` and received the peer's content |

## Measured after building

| Question | Answer |
| --- | --- |
| Does the link answer the document? | Yes. `bay peer open` published the port and the document carried the name, the address, the domains and the authority |
| Is the authority the machine names proved? | Yes. `peer add` verified the certificate the machine presented against the authority in its document before printing the fingerprint |
| Is a served name offered on the link without a certificate? | No. 400 |
| Is it offered with one from an approved authority? | Yes. 200 |
| Does closing the link release the port? | Yes. Nothing listened on it afterwards, and the machine's own sites still answered |
| Does a machine hear another announcing itself? | Yes. `peer find` listed the machine's name, address and fingerprint |
| Can a machine be approved by the name it announced? | Yes. `peer add max` found the address, read the document and approved |
