# Test Design: Specification 0003 — Workstation Status

## Status

Approved test design for pc-control v0.2.

## Specification under test

This test design verifies the observable behavior defined in
[Specification 0003: Workstation Status](0003-status.md). It derives from the
approved product requirements and architecture, including the separation of
the HTTP adapter, status application use case, narrow probe port, and
Go-standard-library TCP probe adapter.

This document defines test responsibilities and observable outcomes. It does
not prescribe Go package layout, production function names, concrete
interfaces, mock libraries, or implementation structure.

## Test-suite principles

- Tests observe specified behavior rather than internal implementation
  details.
- Tests are deterministic, focused, and fast enough for routine development
  and CI execution.
- Substitution is used only at real architectural boundaries.
- Native HTTP and TCP behavior is tested at those boundaries with local
  loopback resources.
- No test contacts the real workstation, NAS, Tailscale network, Internet, or
  another external host.
- Tests do not infer physical power state, boot completion, SSH
  authentication, SSH protocol availability, or general health from an online
  result.

## Test levels and responsibilities

| Level | Responsibility | Boundary exercised | External resources |
| --- | --- | --- | --- |
| Unit | Status application behavior and status-timeout configuration rules | Probe port or controlled configuration input | None |
| HTTP integration | Status HTTP contract and request lifecycle | HTTP adapter with a controlled probe port | In-process HTTP or local loopback TCP |
| TCP integration | Native TCP dial success, failure, and no protocol traffic | Go-standard-library TCP probe adapter | Local loopback TCP listener only |
| Startup integration | Status configuration loading and service composition failure behavior | Startup/composition path | Controlled environment and temporary files where existing shutdown configuration requires them |
| Acceptance | Thin full-composition status happy path | Real local composition: environment, HTTP, use case, and TCP listener | Local loopback TCP only |

Behaviors are tested at their most meaningful boundary. The same condition is
not duplicated across every level merely for coverage.

## Unit test design

Unit tests run without network sockets, processes, key files, or process
environment variables. They use deterministic recording, blocking, or failing
probe ports at the architecture's status boundary.

### Status application behavior

| Behavior verified | Test design | Controlled boundary | Determinism |
| --- | --- | --- | --- |
| One accepted request causes one probe | Invoke status once and assert one port invocation. | Recording probe port | Deterministic |
| Online result | Make the port report dial success and assert online. | Successful probe port | Deterministic |
| Offline result | Make the port report dial failure and assert offline. | Failing probe port | Deterministic |
| No pc-control retry | Make the port fail and assert it was invoked once only. | Failing recording port | Deterministic |
| Duplicate requests | Invoke status repeatedly and assert one independent port invocation per request. | Recording probe port | Deterministic |
| Concurrent requests | Start multiple accepted requests with a concurrency-safe recording or blocking port; assert one invocation per request. | Blocking/recording port | Deterministic coordination, not sleeps |
| No TCP, SSH, or WOL behavior in core | Exercise the use case only through the probe port. No socket, SSH, remote-command, WOL, DNS, or generic networking boundary is introduced. | Probe port | Deterministic |

### Status configuration rules

Configuration parsing and validation are tested with controlled configuration
values. Existing Specification 0002 tests remain responsible for shutdown SSH
host and port validation; status tests verify only the new timeout setting and
its composition with the existing target configuration.

| Configuration area | Accepted cases | Rejected cases |
| --- | --- | --- |
| Status probe timeout | Absent value defaults to `1s`; positive Go durations such as `1s` and `1500ms` | Present but empty, whitespace-padded, malformed, zero, or negative values |
| Status target | Existing valid shutdown SSH hostname or IP literal and port are reused without a status-specific target setting | No status-specific host or port setting is part of this specification |

## HTTP integration test design

HTTP integration tests exercise the actual HTTP adapter and use the status
probe-port architectural boundary to control online/offline outcomes and probe
observation. Normal contract cases may run with an in-process HTTP server.
Request-incompletion and disconnect cases use a real loopback TCP connection
because they depend on HTTP request lifecycle behavior.

### Routing, validation, and status outcomes

| Request or condition | Expected observable outcome | Probe operations |
| --- | --- | --- |
| Empty `GET /v1/status` with a successful controlled probe | `200 OK`; JSON media type; `{"status":"online"}` | 1 |
| Empty `GET /v1/status` with a failed controlled probe | `200 OK`; JSON media type; `{"status":"offline"}` | 1 |
| Empty request with absent or arbitrary `Content-Type` | Defined online/offline result; never `415 Unsupported Media Type` | 1 |
| `GET /v1/status?` | Treated as having no query content and accepted | 1 |
| `GET /v1/status` with non-empty raw query, including `?x=1` and `?=` | `400 Bad Request`; `invalid_request` JSON error | 0 |
| `GET /v1/status` with any non-empty body, regardless of content type | `400 Bad Request`; `invalid_request` JSON error | 0 |
| Exact path with unsupported method, including a query or body | `405 Method Not Allowed`; `Allow: GET`; `method_not_allowed` JSON error | 0 |
| Unknown path, including `/v1/status/` | `404 Not Found`; `not_found` JSON error | 0 |

The unsupported-method cases explicitly verify that method handling precedes
query-string and request-body validation. Regression tests retain the
existing `/v1/wake` and `/v1/shutdown` behavior while all three endpoints are
present.

### Error safety, HEAD, and request lifecycle

- A failed controlled probe is an offline `200` result, not an error envelope
  or `503`; deliberately injected internal error sentinels must not be
  disclosed in the response.
- Non-HEAD validation, routing, and method errors use the specified JSON
  envelope, media type, stable code, and a non-empty safe message. Tests do
  not assert exact message wording.
- `HEAD /v1/status` returns `405 Method Not Allowed`, `Allow: GET`, and no
  body; it performs zero probes. `HEAD` to an unknown path returns `404` with
  no body and zero probes. Query/body complications do not alter the
  corresponding method or routing outcome.
- An incomplete valid-looking status request body followed by client
  disconnect is not accepted and performs zero probes.
- A fully received and validated status request followed by client disconnect
  begins exactly one probe and does not cause an additional probe. A blocking
  probe port and synchronization signals, rather than sleeps, establish the
  event ordering. No response is required for the disconnected client.

## TCP integration test design

TCP integration tests exercise the native status probe adapter with
loopback-only sockets. They do not use a fake SSH server because the status
contract deliberately performs no SSH protocol traffic.

- A listener bound to `127.0.0.1` on an ephemeral port records exactly one
  accepted connection for one probe and observes no application or SSH bytes
  before the adapter closes it. The probe result is online.
- A formerly-listening loopback port, closed before the probe, produces an
  offline adapter result without contacting any non-loopback address.
- A local listener that accepts but does not complete any higher-level
  protocol still produces online: TCP connection acceptance is sufficient and
  no SSH handshake is attempted.

Portable, deterministic reproduction of a kernel TCP-dial timeout is not a
requirement for this layer. Timeout parsing is verified through configuration
tests, and offline mapping through the controlled probe-port boundary. The
production adapter must apply the configured timeout, but the suite must not
use an external or routed address merely to induce a timeout.

The application's no-retry behavior is verified at the probe-port boundary in
unit and HTTP tests. The successful adapter test's single accepted loopback
connection verifies the adapter performs one dial operation for that request.
The suite does not impose a DNS or address-selection policy, and does not
interpret Go standard-library hostname-resolution behavior as a pc-control
retry.

## Startup integration test design

Startup integration tests exercise the actual configuration-loading and
composition path with controlled environment values and the existing temporary
shutdown material required by v0.2 composition.

- Absent `PC_CONTROL_STATUS_PROBE_TIMEOUT` composes with its `1s` default.
- Invalid status timeout configuration prevents the service from entering its
  normal serving state.
- Startup exposes failure to the invoking caller, process, or supervisor path.
- A captured operational diagnostic indicates configuration failure without
  disclosing deliberately injected sensitive configuration sentinels.
- The exhaustive timeout-validation matrix belongs to unit tests. Startup
  integration verifies resulting composition behavior without duplicating it.

Environment-mutating tests are non-parallel, unless an isolated child process
is used. The exact choice is an implementation concern; both approaches keep
the tests local and deterministic.

## Default-suite acceptance test

The default test suite includes one thin, hardware-free full-composition
status acceptance test:

1. Provide real valid environment values, including an explicit local loopback
   HTTP port, existing valid Wake-on-LAN configuration, valid temporary
   shutdown credential material, the loopback shutdown SSH host and port, and
   an optional valid status timeout.
2. Bind a simple loopback TCP listener at that configured shutdown port. It
   records connections and any bytes received; it does not implement SSH.
3. Start the composed service and send `GET /v1/status` through its HTTP
   interface.
4. Assert `200 OK` and `{"status":"online"}`.
5. Assert exactly one loopback TCP connection, no received protocol bytes,
   and no Wake-on-LAN operation. No received protocol bytes establishes that
   the probe did not begin an SSH protocol exchange.

This test covers configuration loading, composition, HTTP serving, status
application behavior, and native TCP probing together. It runs routinely; it
is not preemptively separated or tagged as deployment-specific.

## Determinism and isolation controls

- Use `127.0.0.1` loopback TCP listeners only for network behavior; never
  depend on broadcasts, routes, the real workstation, NAS, Tailscale, or an
  external host.
- Coordinate concurrent and cancellation tests with barriers or channels, not
  time-based sleeps.
- Apply short bounded timeouts only to genuinely asynchronous local socket
  operations.
- Keep environment changes isolated and clean them up after each test.
- Configure explicit valid test ports where configuration requires them; the
  test harness owns port allocation and cleanup.

## Explicit non-goals of this suite

The suite does not test physical workstation power state, boot completion, SSH
authentication or command execution, SSH host-key verification, general host
health, DNS/address-selection policy, Tailscale authorization, public-exposure
policy, Synology, container deployment, Docker, retry scheduling, or external
network reachability. These are outside Specification 0003 or deliberately
outside pc-control v0.2.
