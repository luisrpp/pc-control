# ADR 0001: Use Go for pc-control v0.1

## Status

Accepted

## Context

pc-control v0.1 is a small, headless service with a programmatic HTTP/JSON
interface. For each received wake command, it must perform exactly one
Wake-on-LAN attempt for a single preconfigured workstation and immediately
report whether the local send operation succeeded or failed.

The architecture requires explicit boundaries between application behavior
and infrastructure, narrow dependencies, deterministic core testing, and
side effects at the HTTP and UDP boundaries. The service must be portable:
it is developed and tested locally without intrinsically depending on a
particular private-network service, hosting platform, or container runtime.

## Decision

Implement pc-control v0.1 in Go.

The implementation will use idiomatic Go while preserving the approved
architecture: a small application core, an HTTP/JSON inbound adapter, and a
narrow Wake-on-LAN sender port implemented by a native UDP adapter. Go's
standard library is the default for HTTP and UDP networking. Additional
frameworks and dependencies require a concrete v0.1 need.

## Consequences

- The service can be distributed as a small native binary and can also be
  packaged for a container if that deployment decision is later made.
- The standard library supports the required HTTP and UDP boundary adapters
  without requiring a web framework or an external Wake-on-LAN library.
- Interfaces are used only where they express a concrete boundary or enable
  substitution at that boundary; Go abstractions are not added merely to
  imitate architectural patterns from another ecosystem.
- Application behavior can be tested independently from HTTP, UDP, private-
  network services, deployment platforms, containers, and hardware by
  substituting the wake-sender port.
- The final Go package layout, module setup, dependency list, and test tooling
  remain deferred to implementation and delivery phases.
