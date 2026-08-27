# pc-control

`pc-control` is a small, headless Go service for one preconfigured
workstation. It provides private HTTP operations to send a Wake-on-LAN packet,
initiate a graceful shutdown, and observe TCP reachability of the configured
SSH endpoint.

It has no application-level authentication. Place it behind an appropriate
private-network and access-control boundary; do not expose it directly to the
public Internet.

## API

All operations accept no query parameters or request body. A trailing bare
`?` is accepted. Invalid input returns `400`; an unsupported method on a
known endpoint returns `405` with the endpoint's `Allow` header.

| Request | Successful response | Meaning |
| --- | --- | --- |
| `POST /v1/wake` | `200 {"result":"sent"}` | One local UDP WOL send succeeded. |
| `POST /v1/shutdown` | `202 {"result":"initiated"}` | The configured SSH shutdown capability completed successfully. |
| `GET /v1/status` | `200 {"status":"online"}` or `200 {"status":"offline"}` | One TCP dial to the configured SSH host and port succeeded or failed. |

For example, using an intentionally fictitious service name:

```sh
curl -X POST http://control.example.invalid:8080/v1/wake
curl -X POST http://control.example.invalid:8080/v1/shutdown
curl http://control.example.invalid:8080/v1/status
```

### Semantics and limits

- Wake sends exactly one 102-byte WOL Magic Packet. `"sent"` confirms only
  that local UDP send operation, not delivery, receipt, boot, or power state.
- Shutdown performs exactly one SSH operation using the fixed client-side
  `systemctl poweroff` capability. `"initiated"` does not prove shutdown
  completion, an offline state, or that the literal command ran remotely.
- Status performs exactly one TCP dial and sends no application or SSH bytes.
  `"online"` means only that the configured TCP endpoint accepted that
  connection. `"offline"` means the dial failed or timed out; neither result
  establishes physical power state, boot completion, SSH authentication, or
  general workstation health.
- The service does not retry these operations, select targets, or keep request
  history.

### Target workstation support

pc-control is currently developed and tested against an Arch Linux workstation
using systemd and OpenSSH. Wake-on-LAN itself is not Arch-specific, and status
is a TCP reachability probe rather than an Arch-specific health check.
Graceful shutdown currently depends on the target's SSH configuration and the
systemd command `systemctl poweroff`. Other Linux distributions with compatible
systemd and OpenSSH setups may work, but are not currently tested or
supported. Windows is not currently supported for graceful shutdown.

The formal HTTP and operation contracts are in the [wake specification](specs/0001-wake-on-lan.md),
[shutdown specification](specs/0002-shutdown.md), and [status specification](specs/0003-status.md).

## Configuration

Configuration is supplied through environment variables. Required values must
be present and non-empty; leading or trailing whitespace is invalid. Optional
defaults apply only when the variable is absent.

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `PC_CONTROL_HTTP_LISTEN_ADDR` | Yes | — | Bind address with an explicit numeric port. It accepts an IP literal or empty-host wildcard, not a hostname. |
| `PC_CONTROL_WOL_MAC` | Yes | — | Target workstation MAC address. |
| `PC_CONTROL_WOL_DESTINATION` | Yes | — | IPv4 UDP destination for the Magic Packet, usually a deployment-selected broadcast address. |
| `PC_CONTROL_WOL_PORT` | No | `9` | UDP destination port. |
| `PC_CONTROL_SHUTDOWN_SSH_HOST` | Yes | — | SSH host or IP literal without a port; status reuses it. |
| `PC_CONTROL_SHUTDOWN_SSH_PORT` | No | `22` | SSH TCP port; status reuses it. |
| `PC_CONTROL_SHUTDOWN_SSH_USER` | Yes | — | Dedicated restricted shutdown account. |
| `PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH` | Yes | — | Read-only mounted dedicated private-key file. |
| `PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH` | Yes | — | Read-only mounted `known_hosts` data. |
| `PC_CONTROL_SHUTDOWN_TIMEOUT` | No | `10s` | Positive Go duration covering the complete SSH shutdown operation. |
| `PC_CONTROL_STATUS_PROBE_TIMEOUT` | No | `1s` | Positive Go duration that bounds the single status TCP dial. |

For example, the following values are fictitious and contain no credentials:

```sh
export PC_CONTROL_HTTP_LISTEN_ADDR='[2001:db8::10]:8080'
export PC_CONTROL_WOL_MAC='02:00:00:00:00:01'
export PC_CONTROL_WOL_DESTINATION='192.0.2.255'
export PC_CONTROL_SHUTDOWN_SSH_HOST='workstation.example.invalid'
export PC_CONTROL_SHUTDOWN_SSH_USER='pc-control'
export PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH='/run/pc-control/ssh/private_key'
export PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH='/run/pc-control/ssh/known_hosts'

go run ./cmd/pc-control
```

## SSH security

The shutdown credential is a capability, not a general login. Provision the
configured account and key so they can initiate only the intended graceful
shutdown; they must not provide interactive shell access or arbitrary command
execution. A target-side forced command is one suitable enforcement mechanism.

The private key must be supplied as a dedicated file mounted read-only, never
as an environment-variable value. Host-key verification against configured
`known_hosts` data is mandatory on every shutdown connection; unknown keys are
not accepted automatically. Keep both files out of source control and do not
log or expose their contents.

## Docker and Compose

Build and run with Compose after creating a deployment-specific `.env` from
the example file:

```sh
cp .env.example .env
docker compose up --build -d
```

The supplied Compose configuration uses host networking so the UDP adapter can
reach the network selected for WOL. It loads `.env`; populate every required
shutdown setting there. It mounts the deployment-provided SSH files read-only:

- `./secrets/private_key` → `/run/pc-control/ssh/private_key`
- `./secrets/known_hosts` → `/run/pc-control/ssh/known_hosts`

Provision those files locally, keep them out of source control, and ensure the
private key is readable by the container's runtime user. Choose deployment-
specific network exposure, secret delivery, file ownership, and paths outside
this repository. The service itself is not tied to any particular operating
system, private-network service, or hosting environment.

## Development

Go 1.24 or later is required to run from source.

```sh
go run ./cmd/pc-control
go test -run '^$' ./...
go test ./...
go vet ./...
git diff --check
```

## Architecture and further reading

The service uses narrow Ports & Adapters boundaries:

```text
HTTP adapter -> Wake use case -> Sender port <- UDP WOL adapter
             -> Shutdown use case -> Shutdown port <- SSH shutdown adapter
             -> Status use case -> Probe port <- TCP probe adapter
```

See the [PRD](docs/prd.md), [architecture](docs/architecture.md),
[architecture decisions](decisions/), and the feature test designs in
[specs](specs/).
