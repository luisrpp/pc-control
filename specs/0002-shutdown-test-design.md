# Test Design: Specification 0002 — Graceful Shutdown

## Status

Approved test design for pc-control v0.2.

## Specification under test

This test design verifies the observable behavior defined in
[Specification 0002: Graceful Shutdown](0002-shutdown.md). It derives from
the approved product requirements, architecture, and ADR 0002, including the
separation of the HTTP adapter, shutdown application use case, narrow shutdown
port, and Go-native SSH adapter.

This document defines test responsibilities and observable outcomes. It does
not prescribe Go package layout, production function names, concrete
interfaces, mock libraries, or implementation structure.

## Test-suite principles

- Tests observe specified behavior rather than internal implementation details.
- Tests are deterministic, focused, and fast enough for routine development
  and CI execution.
- Substitution is used only at real architectural boundaries.
- Native HTTP and SSH behavior is tested at those boundaries with local
  loopback resources.
- No test requires a physical workstation, a real `systemctl` invocation,
  Tailscale, Synology, Docker, Internet access, or an actual target SSH host.
- A successful SSH operation is never treated as evidence that the workstation
  powered off, is offline, or literally executed the client-side command.
- Test fixtures use generated ephemeral host keys and temporary dedicated
  client keys; no real secret, target identity material, or production
  known-hosts data is committed.

## Test levels and responsibilities

| Level | Responsibility | Boundary exercised | External resources |
| --- | --- | --- | --- |
| Unit | Shutdown application behavior and shutdown configuration rules | Shutdown port or controlled configuration input | None |
| HTTP integration | Shutdown HTTP contract and request lifecycle | HTTP adapter with a controlled shutdown port | In-process HTTP or local loopback TCP |
| SSH integration | Native SSH authentication, host verification, timeout, and fixed client-side operation | Go-native SSH adapter | Local loopback fake SSH server and temporary key files |
| Startup integration | Shutdown configuration loading and service composition failure behavior | Startup/composition path | Controlled environment, temporary files, diagnostic capture |
| Acceptance | Thin full-composition shutdown happy path | Real local composition: environment, HTTP, use case, and fake SSH server | Local loopback TCP and temporary key files |

Behaviors are tested at their most meaningful boundary. The same condition is
not duplicated across every level merely for coverage.

## Unit test design

Unit tests run without network sockets, processes, key files, or process
environment variables. They use a deterministic recording, blocking, or
failing shutdown port at the architecture's shutdown boundary.

### Shutdown application behavior

| Behavior verified | Test design | Controlled boundary | Determinism |
| --- | --- | --- | --- |
| One accepted command causes one operation | Invoke shutdown once and assert one port invocation. | Recording shutdown port | Deterministic |
| Immediate initiation success | Make the port report success and assert the immediate successful application outcome. | Successful shutdown port | Deterministic |
| Immediate failure | Make the port fail and assert the immediate failed application outcome. | Failing shutdown port | Deterministic |
| No automatic retry | Make the port fail and assert it was invoked once only. | Failing recording port | Deterministic |
| Duplicate commands | Invoke accepted shutdown repeatedly and assert one independent port invocation per command. | Recording shutdown port | Deterministic |
| Concurrent commands | Start multiple accepted commands with a concurrency-safe recording or blocking port; assert one invocation per command. | Blocking/recording shutdown port | Deterministic coordination, not sleeps |
| No SSH or state behavior in core | Exercise the use case solely through the shutdown port. No SSH, availability, power-state, or remote-command boundary is introduced. | Shutdown port | Deterministic |

### Shutdown configuration rules

Configuration parsing and validation are tested with controlled configuration
values and temporary files where composition validates file content.

| Configuration area | Accepted cases | Rejected cases |
| --- | --- | --- |
| Required SSH values | Present, non-empty values without leading or trailing whitespace | Missing, empty, or whitespace-padded required values |
| SSH host | Deployment-selected DNS hostname and IPv4/IPv6 literal without scheme or port | Empty value, surrounding whitespace, URL scheme, or embedded port |
| SSH port | Absent value defaults to `22`; base-10 values from `1` through `65535`, including `022` for port 22 | Present but empty, malformed, zero, negative, or out-of-range values |
| SSH user | Non-empty restricted-account name without outer whitespace | Missing, empty, or whitespace-padded value |
| Private-key path and data | Readable path to one valid unencrypted temporary private key | Missing, empty, whitespace-padded, unreadable, malformed, or encrypted-key data |
| Known-hosts path and data | Readable path to syntactically valid temporary known-hosts data | Missing, empty, whitespace-padded, unreadable, or malformed data |
| Shutdown timeout | Absent value defaults to `10s`; positive Go durations | Present but empty, malformed, zero, or negative values |

The test suite does not require a known-hosts file to contain a reachable
production target. Matching a host key is verified at the SSH boundary.
Read-only mounting of the private-key file is a deployment responsibility:
tests verify the observable file and configuration contract but do not add a
runtime write check or production abstraction solely to test mount semantics.

## HTTP integration test design

HTTP integration tests exercise the actual HTTP adapter and use the shutdown
port architectural boundary to control initiation success, failure, and
operation observation. Normal contract cases may run with an in-process HTTP
server. Request-incompletion and disconnect cases use a real loopback TCP
connection because they depend on HTTP request lifecycle behavior.

### Routing, validation, and success

| Request or condition | Expected observable outcome | Shutdown operations |
| --- | --- | --- |
| Empty `POST /v1/shutdown` | `202 Accepted`; JSON media type; `{"result":"initiated"}` after a successful port result | 1 |
| Empty command with absent or arbitrary `Content-Type` | Same success outcome; never `415 Unsupported Media Type` | 1 |
| `POST /v1/shutdown?` | Treated as having no query content and accepted | 1 |
| `POST /v1/shutdown` with non-empty raw query, including `?x=1` and `?=` | `400 Bad Request`; `invalid_request` JSON error | 0 |
| `POST /v1/shutdown` with any non-empty body, regardless of content type | `400 Bad Request`; `invalid_request` JSON error | 0 |
| Exact path with unsupported method, including a query or body | `405 Method Not Allowed`; `Allow: POST`; `method_not_allowed` JSON error | 0 |
| Unknown path, including `/v1/shutdown/` | `404 Not Found`; `not_found` JSON error | 0 |

The unsupported-method cases explicitly verify that method handling precedes
query-string and request-body validation. Regression tests keep the existing
`/v1/wake` behavior unchanged while both endpoints are present.

### Shutdown failure and error safety

For a valid command, a controlled failed shutdown port must produce
`503 Service Unavailable` with the `shutdown_failed` JSON error code when a
response can be delivered. All non-HEAD errors must use the specified JSON
envelope and the `application/json` media type; media-type parameters such as
`charset=utf-8` are acceptable.

Tests verify stable status and error code, a non-empty human-readable error
message, and non-disclosure of deliberately injected internal-error sentinels.
They do not assert exact message wording, logging schema, or implementation
error types.

### HEAD and request lifecycle

| Request or condition | Expected observable outcome | Shutdown operations |
| --- | --- | --- |
| `HEAD /v1/shutdown` | `405 Method Not Allowed`, `Allow: POST`, no body | 0 |
| `HEAD` to an unknown path | `404 Not Found`, no body | 0 |
| Any HEAD request with query/body complications | Corresponding routing or method outcome, no body | 0 |
| Incomplete valid-looking request body followed by client disconnect | The command is not accepted | 0 |
| Fully received and validated command followed by client disconnect | One shutdown operation still occurs; no additional operation; no client response is required | 1 |

The cancellation test sends a complete valid request over local TCP, confirms
that a controlled blocking shutdown port has begun, then disconnects the
client. A blocking port and synchronization signals, rather than sleeps,
establish the event ordering.

## SSH integration test design

SSH integration tests exercise the native SSH shutdown adapter against a local
fake SSH server on loopback. The fake server uses an ephemeral host key and
accepts only a generated temporary dedicated client key. The test writes a
matching known-hosts fixture and private-key fixture to temporary files.

| Behavior verified | Test design | Expected result |
| --- | --- | --- |
| Successful shutdown-capability initiation | Fake server accepts the dedicated key, verifies the configured host identity, accepts the fixed client-side shutdown operation, and returns successful session completion. | Adapter reports success. |
| Fixed client-side operation | Fake server records the SSH exec request. | It is exactly `systemctl poweroff`; no caller-controlled command input exists. |
| Host-key pinning | Use a mismatched fake-server host key or non-matching known-hosts fixture. | Adapter fails before remote operation acceptance; no trust-on-first-use fallback. |
| Dedicated-key authentication | Reject the configured key or offer a different key. | Adapter reports failure and no operation success. |
| Remote operation failure | Fake server returns a non-successful session completion. | Adapter reports failure. |
| Indeterminate post-initiation disconnect | Fake server accepts the shutdown operation, then closes the connection without successful session completion. | Adapter reports failure and makes no retry; the test does not infer target state. |
| End-to-end timeout | Fake server deterministically blocks connection, authentication, or session completion beyond a short configured timeout. | Adapter reports failure within the configured bound. |
| One adapter call, one operation | Fake server records sessions/exec requests for one adapter invocation. | One SSH shutdown operation only; no automatic retry or second session. |

The successful fake-server response represents remote acceptance of the
shutdown capability only. It is not a test that a real operating system ran
`systemctl poweroff`, and it does not establish shutdown completion or offline
state. The fixed-command test checks the initial adapter's outbound request,
not the external success definition: a real forced-command policy may ignore
or replace that request.

No test adds a generic command-execution abstraction solely to control SSH
failure modes. The local fake SSH server is the actual external-protocol
boundary and supports deterministic error and timeout behavior.

## Startup integration test design

Startup integration tests exercise actual configuration loading and service
composition with controlled environment values and temporary SSH fixture files.

- Valid WOL and shutdown configuration, including parsable temporary key and
  known-hosts files, composes the service successfully.
- Invalid or incomplete shutdown configuration prevents the service from
  entering normal serving state.
- Unreadable, malformed, or encrypted private-key files and unreadable or
  malformed known-hosts files prevent normal startup.
- Startup exposes failure to the invoking caller, process, or supervisor path.
- A captured operational diagnostic indicates configuration failure without
  exposing deliberately injected private-key, credential, or host-identity
  sentinels.
- The exhaustive parsing matrix belongs to unit tests. Startup integration
  verifies composition behavior without duplicating every invalid case.

Environment-mutating tests are non-parallel unless an isolated child process
is used. The exact choice is an implementation concern; both approaches keep
the test local and deterministic.

## Default-suite acceptance test

The default test suite includes one thin, hardware-free full-composition
shutdown acceptance test:

1. Generate a temporary dedicated client key, fake-server host key, and
   matching known-hosts file.
2. Start a local fake SSH server that accepts the configured capability and
   reports successful session completion.
3. Provide real valid environment values for existing WOL settings, the SSH
   target/port/user/key/known-hosts settings, and a local loopback HTTP port.
4. Start the composed service.
5. Send an empty `POST /v1/shutdown` through its HTTP interface.
6. Assert `202 Accepted` and `{"result":"initiated"}`.
7. Assert that the fake SSH server observed exactly one authenticated shutdown
   operation and no additional connection or retry.

This test covers configuration loading, composition, HTTP serving, shutdown
application behavior, host-key verification, key authentication, and native
SSH operation. It never contacts a real workstation or performs an operating
system shutdown.

## Determinism and isolation controls

- Use loopback TCP only; never depend on external hosts, DNS, routes, or a
  real SSH daemon.
- Generate ephemeral test keys and write them only to test-owned temporary
  files; clean them up after each test.
- Coordinate concurrent, timeout, and cancellation tests with barriers,
  channels, and socket deadlines, not time-based sleeps.
- Apply short bounded timeouts only to genuinely asynchronous local socket
  operations and timeout-contract tests.
- Keep environment changes isolated and clean them up after each test.
- Ensure tests never invoke `systemctl`, a shell, a forced command, sudo, or
  a real target-side authorization policy.

## Explicit non-goals of this suite

The suite does not test actual workstation reachability, target-side
`authorized_keys` or sudo provisioning, whether a forced command replaces the
client-side command, actual execution of `systemctl poweroff`, shutdown
completion, offline state, Tailscale authorization, public-exposure policy,
Synology, container deployment, Docker, persistence, retry scheduling, or
hardware behavior. These are outside Specification 0002 or deliberately
outside pc-control v0.2.
