# Test Design: Specification 0001 — Wake-on-LAN

## Status

Approved test design for pc-control v0.1.

## Specification under test

This test design verifies the observable behavior defined in
[Specification 0001: Wake-on-LAN](0001-wake-on-lan.md). It derives from the
approved product requirements and architecture, including the separation of
the HTTP adapter, wake application use case, and native UDP Wake-on-LAN
adapter.

This document defines test responsibilities and observable outcomes. It does
not prescribe Go package layout, production function names, concrete
interfaces, mock libraries, or implementation structure.

## Test-suite principles

- Tests observe specified behavior rather than internal implementation
  details.
- Tests are deterministic, focused, and fast enough for routine development
  and CI execution.
- Substitution is used only at real architectural boundaries.
- Native HTTP and UDP behavior is tested at those boundaries with local
  sockets.
- No test requires a physical workstation, Wake-on-LAN hardware, Synology,
  Tailscale, Docker, Internet access, or a LAN broadcast domain.
- A successful local UDP send is never treated as evidence of packet
  delivery, workstation receipt, workstation state, or boot completion.

## Test levels and responsibilities

| Level | Responsibility | Boundary exercised | External resources |
| --- | --- | --- | --- |
| Unit | Wake application behavior and configuration rules | Wake-sender boundary or controlled configuration input | None |
| HTTP integration | HTTP contract and request lifecycle | HTTP adapter with a controlled wake sender | In-process HTTP or local loopback TCP |
| UDP integration | Native UDP emission and Magic Packet bytes | Native UDP adapter | Local loopback UDP socket |
| Startup integration | Configuration loading and service composition failure behavior | Startup/composition path | Controlled environment and diagnostic capture |
| Acceptance | Thin full-composition happy path | Real local composition: environment, HTTP, use case, and UDP | Local loopback TCP and UDP |

Behaviors are tested at their most meaningful boundary. The same condition is
not duplicated across every level merely for coverage.

## Unit test design

Unit tests run without network sockets, processes, or process environment
variables. They use a deterministic recording, blocking, or failing wake
sender at the architecture's existing wake-sender boundary.

### Wake application behavior

| Behavior verified | Test design | Controlled boundary | Determinism |
| --- | --- | --- | --- |
| One accepted command causes one attempt | Invoke the wake behavior once and assert one sender invocation. | Recording wake sender | Deterministic |
| Immediate success | Make the sender report local send success and assert the immediate successful application outcome. | Successful sender | Deterministic |
| Immediate failure | Make the sender fail and assert the immediate failed application outcome. | Failing sender | Deterministic |
| No automatic retry | Make the sender fail and assert it was invoked once only. | Failing recording sender | Deterministic |
| Duplicate commands | Invoke the accepted command repeatedly and assert one independent sender invocation per command. | Recording sender | Deterministic |
| Concurrent commands | Start multiple accepted commands with a concurrency-safe recording or blocking sender; assert one invocation per command. | Blocking/recording sender | Deterministic coordination, not sleeps |
| No state checks or receipt checks | Exercise the use case solely through the sender boundary. No workstation-status, receipt, or boot-completion boundary is introduced. | Wake sender | Deterministic |

The application-level exactly-one assertions establish the no-retry and
independent-duplicate requirements. UDP packet capture is not used to prove
application concurrency semantics.

### Configuration rules

Configuration parsing and validation are tested with a controlled source of
configuration values. The tests cover the following contract.

| Configuration area | Accepted cases | Rejected cases |
| --- | --- | --- |
| Required values | Present, non-empty values without leading or trailing whitespace | Missing, empty, or whitespace-padded required values |
| HTTP listen address | IPv4 literal with explicit port, bracketed IPv6 literal with explicit port, and empty-host wildcard with explicit port | Hostnames, missing or malformed port, unsupported address form, port 0, and ports above 65535 |
| WOL destination | Conventional dotted-decimal IPv4 literal with four octets from 0 through 255 | CIDR suffixes, hostnames, IPv6, wrong octet count, out-of-range octets, and leading-zero octets |
| WOL port | Absent value defaults to 9; base-10 values from 1 through 65535, including `09` for port 9 | Present but empty, malformed, zero, negative, or out-of-range values |
| WOL MAC | Six-byte colon-separated, hyphen-separated, and dotted forms, case-insensitively | Other forms, mixed/invalid separators, non-hex input, and decoded lengths other than six bytes |

## HTTP integration test design

HTTP integration tests exercise the actual HTTP adapter and use the existing
wake-sender architectural boundary to control send success, failure, and
attempt observation. Normal contract cases may run with an in-process HTTP
server. Request-incompletion and disconnect cases use a real loopback TCP
connection because they depend on HTTP request lifecycle behavior.

### Routing, validation, and success

| Request or condition | Expected observable outcome | WOL attempts |
| --- | --- | --- |
| Empty `POST /v1/wake` | `200 OK`; JSON media type; `{"result":"sent"}` after a successful sender result | 1 |
| Empty command with absent or arbitrary `Content-Type` | Same success outcome; never `415 Unsupported Media Type` | 1 |
| `POST /v1/wake?` | Treated as having no query content and accepted | 1 |
| `POST /v1/wake` with non-empty raw query, including `?x=1` and `?=` | `400 Bad Request`; `invalid_request` JSON error | 0 |
| `POST /v1/wake` with any non-empty body, regardless of content type | `400 Bad Request`; `invalid_request` JSON error | 0 |
| Exact path with unsupported method, including a query or body | `405 Method Not Allowed`; `Allow: POST`; `method_not_allowed` JSON error | 0 |
| Unknown path, including `/v1/wake/` | `404 Not Found`; `not_found` JSON error | 0 |

The unsupported-method cases explicitly verify that method handling precedes
query-string and request-body validation.

### Send failure and error safety

For a valid command, a controlled failed sender must produce `503 Service
Unavailable` with the `wake_failed` JSON error code when a response can be
delivered. All non-HEAD errors must use the specified JSON envelope and the
`application/json` media type; media-type parameters such as `charset=utf-8`
are acceptable.

Tests verify stable status and error code, a non-empty human-readable error
message, and non-disclosure of a deliberately injected internal-error
sentinel. They do not assert exact message wording, logging schema, or
implementation error types.

### HEAD and request lifecycle

| Request or condition | Expected observable outcome | WOL attempts |
| --- | --- | --- |
| `HEAD /v1/wake` | `405 Method Not Allowed`, `Allow: POST`, no body | 0 |
| `HEAD` to an unknown path | `404 Not Found`, no body | 0 |
| Any HEAD request with query/body complications | Corresponding routing or method outcome, no body | 0 |
| Incomplete valid-looking request body followed by client disconnect | The command is not accepted | 0 |
| Fully received and validated command followed by client disconnect | One send attempt still occurs; no additional attempt; no client response is required | 1 |

The cancellation test sends a complete valid request over local TCP, confirms
that the controlled sender has begun, then disconnects the client. A blocking
sender and synchronization signals, rather than sleeps, establish the event
ordering.

## UDP integration test design

UDP integration tests exercise the native UDP Wake-on-LAN adapter with a real
UDP receiver bound on loopback. They do not contact a broadcast network or
physical device.

For a configured destination and port, one send operation must emit a
datagram observed by the local receiver at that destination and port. The
captured datagram must be exactly 102 bytes:

1. Bytes 0 through 5 are each `0xFF`.
2. Bytes 6 through 101 are sixteen consecutive repetitions of the configured
   six-byte MAC address.

Local UDP send success is tested as the boundary operation that leads to the
specified success outcome. The test does not and cannot establish physical
delivery or a workstation power-state change.

Portable, deterministic reproduction of a kernel UDP-send failure is not a
requirement for this layer. Failure propagation is verified at the existing
wake-sender boundary in the unit and HTTP integration tests. The production
design must not acquire a general-purpose socket abstraction solely to force
that OS failure mode.

## Startup integration test design

Startup integration tests exercise the actual configuration-loading and
composition path with controlled environment values.

- Invalid or incomplete configuration prevents the service from entering its
  normal serving state.
- Startup exposes failure to the invoking caller, process, or supervisor
  path.
- A captured operational diagnostic indicates configuration failure.
- Tests do not depend on exact message text, logging destination, structured
  fields, or logging implementation.
- A diagnostic must not expose deliberately injected sensitive configuration
  value sentinels.

The exhaustive validation matrix belongs to unit tests. Startup integration
tests verify the resulting composition behavior without duplicating every
invalid parsing case.

Environment-mutating tests are non-parallel, unless an isolated child process
is used. The exact choice is an implementation concern; both approaches keep
the test local and deterministic.

## Default-suite acceptance test

The default test suite includes one thin, hardware-free full-composition
acceptance test:

1. Provide real valid environment values, including an explicit local
   loopback HTTP port and a loopback UDP destination/port.
2. Start the composed service.
3. Send an empty `POST /v1/wake` through its HTTP interface.
4. Assert `200 OK` and the defined JSON sent result.
5. Capture the emitted local UDP datagram and verify the configured
   destination/port and exact Magic Packet layout.

This test covers configuration loading, composition, HTTP serving, wake
application behavior, and native UDP emission together. It runs routinely;
it is not preemptively separated or tagged as deployment-specific.

## Determinism and isolation controls

- Use loopback TCP and UDP only; never depend on broadcasts, routes, or
  external hosts.
- Coordinate concurrent and cancellation tests with barriers or channels, not
  time-based sleeps.
- Apply short bounded timeouts only to genuinely asynchronous local socket
  operations.
- Keep environment changes isolated and clean them up after each test.
- Configure an explicit valid test port rather than port `0`, which the
  specification forbids. The test harness owns port allocation and cleanup.
- Do not use real hardware or attempt to infer WOL delivery from UDP send
  success.

## Explicit non-goals of this suite

The suite does not test workstation reachability, receipt of a Magic Packet,
boot completion, Tailscale authorization, public-exposure policy, Synology,
container deployment, Docker, persistence, retry scheduling, or hardware
Wake-on-LAN behavior. These are outside Specification 0001 or deliberately
outside pc-control v0.1.
