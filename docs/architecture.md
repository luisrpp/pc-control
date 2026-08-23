# pc-control v0.2 Architecture

## Status

Approved architecture for v0.2. This document derives from the approved
[product requirements](prd.md).

## Architectural intent

pc-control is a small, headless Go service that accepts private,
programmatic wake and graceful-shutdown commands for one preconfigured
workstation. It makes exactly one Wake-on-LAN attempt for each accepted wake
command and exactly one remote shutdown operation for each accepted shutdown
command.

The design keeps application behavior independent of HTTP, UDP, SSH,
Tailscale, Synology, containers, and other deployment-specific concerns. Side
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
```

### HTTP/JSON adapter

The inbound adapter exposes the small, command-oriented programmatic
interface. It is responsible only for HTTP concerns: decoding and validating
transport input, invoking the wake or shutdown use case, mapping each
immediate outcome to an HTTP/JSON response, and request-scoped diagnostics.

It does not construct Wake-on-LAN packets, perform UDP or SSH operations,
determine workstation state, execute arbitrary remote commands, or contain
Tailscale and deployment behavior. The standard Go HTTP facilities are the
default choice; a web or RPC framework requires a concrete need.

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
Tailscale-specific concepts, or deployment logic.

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
authorization, user-management, Tailscale-specific concepts, or deployment
logic.

### Wake-on-LAN adapter

The outbound adapter implements the wake-sender port using Go's native UDP
networking. It explicitly constructs the Wake-on-LAN magic packet and sends
one packet according to externally supplied workstation network settings.

Its success result means that the local send operation succeeded. It cannot
confirm packet delivery, receipt by the workstation, or boot completion. An
immediate inability to construct or send the packet is reported as failure.

The WOL adapter does not invoke an operating-system utility or require an external
Wake-on-LAN library.

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

A small composition layer creates and connects the HTTP adapter, wake and
shutdown use cases, UDP and SSH adapters, configuration, and logging. Runtime
configuration remains outside the application core. Environment variables are
the initial default configuration mechanism; no general configuration subsystem
is needed.

Configuration must provide the workstation's Wake-on-LAN parameters, SSH
shutdown target and credential-file parameters, shutdown timeout, and HTTP
listen address and port. Secrets are supplied as mounted files rather than
environment-variable contents. Exact variable names, validation rules, and
network defaults are specified by feature specifications. Invalid or incomplete
required configuration, unreadable credential or known-hosts files, and
malformed key material prevent normal startup and are reported through safe
operational diagnostics.

## Network, authorization, and deployment boundaries

pc-control does not implement authorization or own the private-network
exposure policy in v0.2. Tailscale and surrounding network/deployment controls
are responsible for limiting access to authorized clients and avoiding public
Internet exposure.

The HTTP listen address and port are externally configurable. Their concrete
defaults, and the mechanism used to expose the service through Tailscale or
other network infrastructure, are deployment/configuration decisions and are
not established by this architecture.

The core application has no dependency on a particular private-network service,
hosting platform, container runtime, or host. It must run locally for
development and testing. Native-process and container packaging remain deferred
deployment decisions. Any selected
runtime/deployment arrangement must allow the UDP adapter to reach the local
LAN broadcast domain needed for Wake-on-LAN and the SSH adapter to reach the
configured workstation SSH service.

## Operational behavior

pc-control emits basic structured logs for startup, configuration failures,
wake and shutdown requests, successful boundary operations, and failures.
Logs are for operational diagnosis only; they are not a user-visible request
history or audit trail. Diagnostics must not disclose private-key material,
credentials, raw SSH errors, remote command output, or other sensitive
deployment details to HTTP clients.

No health endpoint is part of v0.2 architecture. One may be added only when a
selected deployment mechanism establishes a concrete need.

## Testability consequences

The wake and shutdown application use cases are testable deterministically by
substituting their narrow outbound ports. HTTP handling, native UDP sending,
and native SSH behavior can be verified through integration tests at their
respective boundaries. SSH tests use a local fake SSH server and never shut
down a real workstation. Application behavior therefore need not depend on
real Tailscale, Synology, containers, physical Wake-on-LAN hardware, or a
workstation during testing.

The detailed test suite, test tooling, and test cases are deferred to later
delivery phases.

## Deferred decisions

- Future HTTP endpoint, request/response formats, and status/error mapping.
- Future configuration variable names, validation details, and network defaults.
- Concrete listen-address defaults and Tailscale/network publication setup.
- Native-process versus container packaging and Synology service integration.
- Exact target-side SSH restriction mechanism, provided it enforces the
  required shutdown-only capability.
- Logging library, log schema, and Go package layout.
- Signal handling, process supervision, and other runtime details.
- Specifications, tests, and implementation.
