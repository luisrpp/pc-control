# pc-control v0.1 Architecture

## Status

Approved architecture for v0.1. This document derives from the approved
[product requirements](prd.md).

## Architectural intent

pc-control is a small, headless Go service that accepts a private,
programmatic wake command for one preconfigured workstation and makes exactly
one Wake-on-LAN attempt for each received request.

The design keeps application behavior independent of HTTP, UDP, Tailscale,
Synology, containers, and other deployment-specific concerns. Side effects
are concentrated in adapters at the system boundary. Dependencies point
inward toward the application core.

This architecture deliberately does not introduce a repository layer, event
bus, plugin system, device registry, generalized RPC framework, or a
multi-workstation model.

## System structure

```text
HTTP/JSON adapter  ->  Wake application use case  ->  Wake sender port
                                                    ->  Native UDP WOL adapter
```

### HTTP/JSON adapter

The inbound adapter exposes the small, command-oriented programmatic
interface. It is responsible only for HTTP concerns: decoding and validating
transport input, invoking the wake use case, mapping its immediate outcome to
an HTTP/JSON response, and request-scoped diagnostics.

It does not construct Wake-on-LAN packets, perform UDP operations, determine
workstation state, or contain Tailscale and deployment behavior. The standard
Go HTTP facilities are the default choice; a web or RPC framework requires a
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
Tailscale-specific concepts, or deployment logic.

### Wake-on-LAN adapter

The outbound adapter implements the wake-sender port using Go's native UDP
networking. It explicitly constructs the Wake-on-LAN magic packet and sends
one packet according to externally supplied workstation network settings.

Its success result means that the local send operation succeeded. It cannot
confirm packet delivery, receipt by the workstation, or boot completion. An
immediate inability to construct or send the packet is reported as failure.

v0.1 does not invoke an operating-system utility or require an external
Wake-on-LAN library.

### Composition and configuration

A small composition layer creates and connects the HTTP adapter, wake use
case, UDP adapter, configuration, and logging. Runtime configuration remains
outside the application core. Environment variables are the initial default
configuration mechanism; no general configuration subsystem is needed.

Configuration must provide the workstation's Wake-on-LAN parameters and the
HTTP listen address and port. Exact variable names, validation rules, and WOL
network defaults are deferred. Invalid or incomplete required configuration
prevents normal startup and is reported through logs.

## Network, authorization, and deployment boundaries

pc-control does not implement authorization or own the private-network
exposure policy in v0.1. Tailscale and surrounding network/deployment controls
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
LAN broadcast domain needed for Wake-on-LAN.

## Operational behavior

pc-control emits basic structured logs for startup, configuration failures,
wake requests, successful packet sends, and send failures. Logs are for
operational diagnosis only; they are not a user-visible request history or
audit trail.

No health endpoint is part of v0.1 architecture. One may be added only when a
selected deployment mechanism establishes a concrete need.

## Testability consequences

The wake application use case is testable deterministically by substituting
the narrow wake-sender port. HTTP handling and native UDP sending can be
verified later through integration tests at their respective boundaries.
Application behavior therefore need not depend on real Tailscale, Synology,
containers, or physical Wake-on-LAN hardware during testing.

The detailed test suite, test tooling, and test cases are deferred to later
delivery phases.

## Deferred decisions

- HTTP endpoint, request/response formats, and status/error mapping.
- Configuration variable names, validation details, and network defaults.
- Concrete listen-address defaults and Tailscale/network publication setup.
- Native-process versus container packaging and Synology service integration.
- Logging library, log schema, and Go package layout.
- Signal handling, process supervision, and other runtime details.
- Specifications, tests, and implementation.
