# pc-control

`pc-control` is a standalone, portable, headless Go service for requesting a
Wake-on-LAN (WOL) wake-up of one preconfigured workstation.

## v0.1 capabilities and limits

v0.1 provides one HTTP command, `POST /v1/wake`. Each accepted request sends
exactly one WOL Magic Packet to the configured destination and immediately
returns success or failure for that local UDP operation.

It supports exactly one workstation. It does not check whether the
workstation is already running, confirm packet delivery or boot completion,
retry failed sends, keep request history, or provide a web UI or
application-level authentication.

## Requirements

- Go 1.24 or later to run from source.
- A target machine with Wake-on-LAN enabled.
- A host capable of running the service and sending UDP traffic to the
  target LAN.

## Configure and run

Configuration is supplied through environment variables:

| Variable | Required | Description |
| --- | --- | --- |
| `PC_CONTROL_HTTP_LISTEN_ADDR` | Yes | HTTP bind address with an explicit numeric TCP port. |
| `PC_CONTROL_WOL_MAC` | Yes | MAC address of the one target workstation. |
| `PC_CONTROL_WOL_DESTINATION` | Yes | IPv4 UDP destination for the WOL Magic Packet. |
| `PC_CONTROL_WOL_PORT` | No | UDP destination port; defaults to `9` when absent. |

`PC_CONTROL_WOL_DESTINATION` is the **UDP destination**, normally the LAN
broadcast address. It is not the target workstation's potentially changing
DHCP/IP address.

For example, with placeholder values only:

```sh
export PC_CONTROL_HTTP_LISTEN_ADDR='127.0.0.1:8080'
export PC_CONTROL_WOL_MAC='00:11:22:33:44:55'
export PC_CONTROL_WOL_DESTINATION='192.168.1.255'
# Optional; omit it to use the default UDP port 9.
export PC_CONTROL_WOL_PORT='9'

go run ./cmd/pc-control
```

The listen address must be an IPv4 literal, bracketed IPv6 literal, or an
empty-host wildcard such as `:8080`; hostnames are not supported. The WOL
destination must be a dotted-decimal IPv4 address.

## Request a wake

Send an empty `POST` request to `/v1/wake`:

```sh
curl -X POST http://127.0.0.1:8080/v1/wake
```

A successful request returns:

```json
{"result":"sent"}
```

> **Important:** `"sent"` means that the local UDP send succeeded. It does
> not confirm that the packet was delivered, that the workstation received
> it, or that the workstation woke.

The endpoint accepts no query parameters or request body. A local WOL send
failure returns `503 Service Unavailable`.

## Verify

```sh
go test ./...
go vet ./...
```

## Architecture

pc-control follows a small Ports & Adapters design:

```text
HTTP adapter -> Wake use case -> Sender port <- UDP WOL adapter
```

The HTTP adapter handles the command and JSON response. The wake use case
performs one send through its sender port, and the UDP WOL adapter constructs
and sends the Magic Packet.

## Security

pc-control v0.1 has **no application-level authentication** and therefore
must not be exposed directly to the public Internet. Protect access with a
trusted network, VPN, firewall, reverse proxy, access-control layer, or
equivalent surrounding deployment control.

## Current scope / future work

Workstation status, shutdown controls, multiple machines, deployment
automation, and Second Brain integration are not part of v0.1.

For detailed contracts and contributor context, see the [PRD](docs/prd.md),
[architecture](docs/architecture.md), [Wake-on-LAN specification](specs/0001-wake-on-lan.md),
[accepted ADRs](decisions/), and [test design](specs/0001-wake-on-lan-test-design.md).
