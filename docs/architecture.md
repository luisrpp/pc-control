# pc-control v0.2 Architecture

## Status

Approved architecture for v0.2. This document derives from the approved
[product requirements](prd.md).

## Architectural intent

pc-control is a small, headless Go service that accepts private,
programmatic wake and graceful-shutdown commands and reachability-status
requests for one preconfigured workstation. It makes exactly one Wake-on-LAN
attempt for each accepted wake command, exactly one remote shutdown operation
for each accepted shutdown command, and exactly one logical TCP probe
operation for each accepted status request.

The design keeps application behavior independent of HTTP, UDP, SSH,
private-network services, hosting platforms, containers, and other
deployment-specific concerns. Side
effects are concentrated in adapters at the system boundary. Dependencies point
inward toward the application core.

This architecture deliberately does not introduce a repository layer, event
bus, plugin system, device registry, generalized RPC framework, or a
multi-workstation model.

## System structure

```text
HTTP/JSON adapter  ->  Wake application use case      ->  Wake sender port
                                                         ->  Native UDP WOL adapter
                   ->  Shutdown application use case  ->  Shutdown port
                                                         ->  Native SSH shutdown adapter
                   ->  Status application use case    ->  Probe port
                                                         ->  Native TCP probe adapter
```

### HTTP/JSON adapter

The inbound adapter exposes the small, command-oriented programmatic
interface. It is responsible only for HTTP concerns: decoding and validating
transport input, invoking the wake, shutdown, or status use case, mapping each
immediate outcome to an HTTP/JSON response, and request-scoped diagnostics.

It does not construct Wake-on-LAN packets, perform UDP, TCP, or SSH
operations, determine physical workstation state, execute arbitrary remote
commands, or contain private-network or deployment behavior. The standard Go
HTTP facilities are the default choice; a web or RPC framework requires a
concrete need.

The endpoint path, request body, response schema, and HTTP status mapping are
deferred to feature specification.

### Wake application use case

The application core defines the wake command and its immediate sent-or-failed
result. It depends on a narrow wake-sender port rather than on HTTP or UDP.

For each request that reaches the use case, it invokes the wake sender exactly
once. It does not check whether the workstation is online or already running,
does not retry failures, and does not persist or deduplicate requests.
Duplicate requests are acceptable and each results in an independent
Wake-on-LAN attempt.

The core contains no application-level login, authorization, user-management,
private-network-specific concepts, or deployment logic.

### Shutdown application use case

The application core defines the shutdown command and its immediate
initiated-or-failed result. It depends on a narrow shutdown port rather than
on HTTP, SSH, command strings, or credentials. Its boundary is semantic, for
example `Shutdown() error`.

For each request that reaches the use case, it invokes the shutdown port
exactly once. It does not perform an availability, online-state, or power-state
check; retry failures; persist or deduplicate commands; or verify that the
workstation eventually powers off. Duplicate and concurrent accepted commands
are independent operations.

The core contains no SSH-specific concepts and exposes neither a shell nor a
general remote-command abstraction. It has no application-level login,
authorization, user-management, private-network-specific concepts, or
deployment logic.

### Status application use case

The application core defines a status request and its immediate online or
offline result. It depends on a narrow semantic probe port, for example
`Probe() error`, rather than on TCP, sockets, SSH, addresses, or timeouts.

For each accepted status request, it invokes the probe port exactly once. A
successful probe result maps to online; a failed or timed-out probe result maps
to offline. It does not retry, fall back to another target, perform Wake-on-
LAN, authenticate with SSH, send SSH protocol traffic, execute a command,
persist results, or deduplicate requests. Duplicate and concurrent accepted
requests are independent operations.

Online means only that the configured TCP dial operation succeeded. Offline
means only that it failed or reached the configured probe timeout. Neither
result represents a physical power-state determination, boot-completion
check, successful SSH authentication, or general workstation health.

The core contains no networking or SSH-specific concepts and does not create a
generic networking abstraction.

### Wake-on-LAN adapter

The outbound adapter implements the wake-sender port using Go's native UDP
networking. It explicitly constructs the Wake-on-LAN magic packet and sends
one packet according to externally supplied workstation network settings.

Its success result means that the local send operation succeeded. It cannot
confirm packet delivery, receipt by the workstation, or boot completion. An
immediate inability to construct or send the packet is reported as failure.

The WOL adapter does not invoke an operating-system utility or require an external
Wake-on-LAN library.

### TCP status probe adapter

The outbound status adapter implements the probe port with Go's standard
library TCP networking. For each invocation, it performs one TCP dial
operation to the configured shutdown SSH host and port under the configured
probe timeout, and closes a successful connection without sending application
or SSH protocol bytes.

The adapter performs no SSH authentication, remote command, Wake-on-LAN
operation, retry loop, fallback loop, repeated probe, or second dial after a
failure. It introduces no manual DNS resolution or address-selection policy:
normal hostname resolution and address selection inside the Go standard
library networking stack are implementation details, not pc-control retries.

### SSH shutdown adapter

The outbound shutdown adapter implements the shutdown port using a Go-native
SSH client. It performs one configured SSH shutdown operation for each port
invocation, using a single configured end-to-end timeout that covers
connection, authentication, the remote operation, and completion observable by
pc-control. It does not invoke an external `ssh` executable.

The initial adapter issues the fixed client-side SSH command `systemctl
poweroff`. It provides no API for callers to select or supply a command. The
configured credential and target provisioning must grant only the capability
to initiate the configured graceful shutdown. A forced command in
`authorized_keys` is a preferred way to enforce this, but the application
contract does not depend on any particular target-side restriction mechanism.
In particular, an SSH server may ignore or replace the requested command.

The adapter requires host-key verification against configured known-hosts data
on every connection. It does not use trust on first use. It authenticates with
a dedicated private key read from a deployment-provided file. The private key,
known-hosts data, target address, user, port, and timeout are composition
configuration, not application-core concerns.

Its success result means that pc-control observed successful initiation or
acceptance of the remote shutdown capability. It does not prove that the
literal client-side command was executed, that the workstation completed its
shutdown, or that it is offline. Connection loss after the remote operation
may have begun leaves the workstation state indeterminate; the adapter reports
failure and does not retry.

### Composition and configuration

A small composition layer creates and connects the HTTP adapter, wake,
shutdown, and status use cases, UDP, SSH, and TCP adapters, configuration, and
logging. Runtime configuration remains outside the application core.
Environment variables are the initial default configuration mechanism; no
general configuration subsystem is needed.

Configuration must provide the workstation's Wake-on-LAN parameters, SSH
shutdown target and credential-file parameters, shutdown timeout, status probe
timeout, and HTTP listen address and port. The status adapter reuses the SSH
shutdown host and port; it has no second target configuration. Secrets are
supplied as mounted files rather than environment-variable contents. Exact
variable names, validation rules, and network defaults are specified by feature
specifications. Invalid or incomplete required configuration, unreadable
credential or known-hosts files, malformed key material, and invalid status
probe timeout prevent normal startup and are reported through safe operational
diagnostics.

## Network, authorization, and deployment boundaries

pc-control does not implement authorization or own the private-network
exposure policy in v0.2. Surrounding network and deployment controls are
responsible for limiting access to authorized clients and avoiding public
Internet exposure.

The HTTP listen address and port are externally configurable. Their concrete
defaults, and the mechanism used to expose the service through private-network
or other network infrastructure, are deployment/configuration decisions and are
not established by this architecture.

The core application has no dependency on a particular private-network service,
hosting platform, container runtime, or host. It must run locally for
development and testing. Native-process and container packaging remain deferred
deployment decisions. Any selected
runtime/deployment arrangement must allow the UDP adapter to reach the local
LAN broadcast domain needed for Wake-on-LAN and the SSH and TCP status adapters
to reach the configured workstation SSH TCP endpoint.

## Operational behavior

pc-control emits basic structured logs for startup, configuration failures,
wake, shutdown, and status requests, successful boundary operations, and
failures.
Logs are for operational diagnosis only; they are not a user-visible request
history or audit trail. Diagnostics must not disclose private-key material,
credentials, raw SSH errors, remote command output, or other sensitive
deployment details to HTTP clients.

`GET /v1/status` is a workstation TCP-endpoint observation, not a pc-control
service health endpoint. No service health endpoint is part of v0.2
architecture. One may be added only when a selected deployment mechanism
establishes a concrete need.

## Testability consequences

The wake, shutdown, and status application use cases are testable
deterministically by substituting their narrow outbound ports. HTTP handling,
native UDP sending, native SSH behavior, and native TCP probing can be
verified through integration tests at their respective boundaries. SSH tests
use a local fake SSH server and TCP tests use loopback-only listeners; neither
contacts or shuts down a real workstation. Application behavior therefore need
not depend on real private-network services, deployment platforms, containers,
physical Wake-on-LAN hardware, or a workstation during testing.

The detailed test suite, test tooling, and test cases are deferred to later
delivery phases.

## Deferred decisions

- Future HTTP endpoint, request/response formats, and status/error mapping.
- Future configuration variable names, validation details, and network defaults.
- Concrete listen-address defaults and private-network publication setup.
- Native-process versus container packaging and host service integration.
- Exact target-side SSH restriction mechanism, provided it enforces the
  required shutdown-only capability.
- Logging library, log schema, and Go package layout.
- Signal handling, process supervision, and other runtime details.
- Specifications, tests, and implementation.
