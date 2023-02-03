# dhcpserver

Next Level's DHCP server written in Go

Always a work-in-progress

## Why our own?

ISPs often want nonstandard DHCP server behavior, because the DHCP
server ends up implementing customer-edge *policies* that are expected
or enforced by other parts of the network.

One of the policies that we want to enforce is that each subscriber
port is only ever allocated one specific IP, no matter how many
devices/MAC addresses request an IP, and regardless of the lease
expiration time. This means that most (possibly all) allocations we
will ever make are stateless.

## Configuration

We use [CoreDHCP](https://github.com/NextLevelInfrastructure/coredhcp),
where almost everything is implemented as a plugin. The order of
plugins in the configuration matters: every request is evaluated
calling each plugin in order, until one breaks the evaluation and
responds to, or drops, the request.

```
server4:

  plugins:

   # serverIP can be "relay_only" if we want to reject non-relay requests.
   # All relays that do DHCP snooping should appear here so that the relay
   # sees all DHCP client traffic including renewals. There is usually
   # no reason to include here any relay that isn't snooping and
   # whose clients have unicast connectivity to the server.
   - server_id:   serverIP relay1IP relay2IP relay3IP ...

   # If we wanted to assign a default router/netmask we'd do it here.

   - interfaceid: 86400 routers_leases.yml autorefresh

   - routercidr:        routers_leases.yml autorefresh

   # Cloudflare
   - dns:         1.1.1.1 1.0.0.1

server6:

  plugins:

   - server_id:   serverDUID

   - interfaceid: 86400 routers_leases.yml autorefresh

   # Cloudflare
   - dns:         2606:4700:4700::1111 2606:4700:4700::1001
```

## Build and run

First, make sure that the head of the NextLevelInfrastructure dhcpserver
and coredhcp repos is what you want to push.

Second, update dhcpserver, cd to the dhcpserver directory, and run
```
sudo docker build .
```

Third, push to production.

# Author

* [Daniel Dulitz](https://github.com/dulitz)
