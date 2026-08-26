# Specification 0003: Workstation Status

## Status

Approved for pc-control v0.2.

## Purpose and scope

pc-control v0.2 provides one private, programmatic operation to observe the
immediate TCP reachability of the configured workstation SSH endpoint. The
service supports exactly one workstation. A client cannot select a target,
port, protocol, or probe option.

This specification defines observable status HTTP behavior, TCP-probe
semantics, and status-timeout configuration. It does not define deployment or
network controls that restrict access to the private network boundary.

## Terms

### Accepted status request

A request is an accepted status request only when pc-control has fully
received and validated an exact `GET /v1/status` request with no query
parameters and an empty request body. Requests that fail routing, method,
query-string, or request-body validation are not accepted status requests.

### TCP probe operation

Each accepted status request causes exactly one logical TCP probe operation to
the configured shutdown SSH host and port, under the configured probe timeout.
pc-control invokes one TCP dial operation and performs no retry loop, fallback
loop, repeated probe, or second dial after a failure.

Normal hostname resolution and address-selection behavior inside the Go
standard-library networking stack is an implementation detail and is not a
pc-control retry. pc-control does not introduce manual DNS resolution or
address-selection policy for status.

### Online and offline

The status result is online when the TCP dial operation succeeds. The result
is offline when that operation fails or reaches the configured probe timeout.

Online means only that the configured TCP endpoint accepted the connection.
Offline means only that the endpoint could not be reached by this probe.
Neither result proves the workstation's physical power state, boot completion,
successful SSH authentication, SSH protocol availability, or general health.

## HTTP interface

### Status endpoint

The only status endpoint is the exact path:

```text
/v1/status
```

The only supported method is `GET`. The operation accepts no identifier,
options, protocol input, or other input.

### Routing and method handling

- A path other than the exact `/v1/status` path, including `/v1/status/`,
  returns `404 Not Found`.
- On the exact `/v1/status` path, a method other than `GET` returns `405
  Method Not Allowed` and includes `Allow: GET`.
- Method handling takes precedence over query-string and request-body
  validation. For example, `POST /v1/status?x=1` returns `405 Method Not
  Allowed`.
- Routing or method failures must not cause a TCP probe operation.
- The existing `/v1/wake` and `/v1/shutdown` interfaces and their
  specifications are unchanged.

### Query string and request body handling

For `GET /v1/status`:

- Query parameters are not permitted. A request with query parameters returns
  `400 Bad Request` with error code `invalid_request`.
- The request body must be empty. A non-empty request body returns `400 Bad
  Request` with error code `invalid_request`, regardless of its contents or
  media type.
- A trailing `?` with no query content is accepted.
- `Content-Type` is irrelevant for an empty request and may be absent or
  present.
- pc-control does not return `415 Unsupported Media Type` for this operation.
- Query-string or request-body validation failures must not cause a TCP probe
  operation.

### Status responses

When the TCP probe operation succeeds, pc-control returns:

```http
200 OK
Content-Type: application/json
```

```json
{"status":"online"}
```

When the TCP dial operation fails or reaches the configured probe timeout,
pc-control returns:

```http
200 OK
Content-Type: application/json
```

```json
{"status":"offline"}
```

Offline is a normal status result rather than an HTTP operation failure.
pc-control does not expose a `status_failed` error for TCP dial failures or
timeouts, and does not disclose raw network errors, target details, or timeout
internals to clients.

### Error responses

Except for `HEAD` responses, every error response defined by this specification
has the following JSON envelope and includes `Content-Type: application/json`:

```json
{"error":{"code":"...","message":"..."}}
```

The stable error codes and status mappings are:

| Condition | HTTP status | error.code |
| --- | --- | --- |
| Invalid `GET` status input | `400 Bad Request` | `invalid_request` |
| Unsupported method on `/v1/status` | `405 Method Not Allowed` | `method_not_allowed` |
| Unknown path | `404 Not Found` | `not_found` |

The error-code meanings are part of the v0.2 contract. Exact message text is
not a compatibility contract. Messages must be concise, safe, and
human-readable. They must not disclose raw network errors, socket details,
target host details, credentials, key material, stack traces, or other
deployment internals.

### HEAD handling

Every `HEAD` response has no response body, regardless of path or status. It
retains the status and applicable routing or method headers of the corresponding
outcome.

In particular:

- `HEAD /v1/status` returns `405 Method Not Allowed` with `Allow: GET` and no
  body.
- `HEAD` to an unknown path returns `404 Not Found` and no body.

No `HEAD` request causes a TCP probe operation.

### Client disconnect and cancellation

Once pc-control has fully received and validated a status request, it performs
exactly one TCP probe operation even if the HTTP client disconnects or cancels
before a response is delivered.

Client cancellation after acceptance must not cancel the probe or create an
additional probe. A disconnected client has no guaranteed response.

If pc-control cannot fully receive and validate the request, the request is not
accepted and must not trigger a probe.

## Configuration

Runtime configuration is supplied through environment variables. Status reuses
the existing required shutdown SSH target configuration from Specification
0002; it does not introduce another host or port setting.

| Variable | Required | Meaning |
| --- | --- | --- |
| `PC_CONTROL_SHUTDOWN_SSH_HOST` | Yes | Existing configured SSH target hostname or IP literal, reused by status. |
| `PC_CONTROL_SHUTDOWN_SSH_PORT` | No | Existing SSH TCP destination port, reused by status; defaults to `22` when absent. |
| `PC_CONTROL_STATUS_PROBE_TIMEOUT` | No | TCP status-probe timeout; defaults to `1s` only when absent. |

The existing Wake-on-LAN and shutdown configuration remains required because
v0.2 exposes all three configured capabilities. Leading or trailing whitespace
is invalid for every status configuration value. Invalid or incomplete required
configuration prevents normal service startup and is reported through safe
operational diagnostics.

### Status probe timeout

`PC_CONTROL_STATUS_PROBE_TIMEOUT` defaults to `1s` only when absent. When
present, it must be a non-empty, positive Go duration such as `1s` or `1500ms`.
An empty, whitespace-padded, malformed, zero, or negative value prevents normal
startup.

The timeout bounds the TCP dial operation. It is independent of the
`PC_CONTROL_SHUTDOWN_TIMEOUT` end-to-end SSH operation timeout.

## TCP probe behavior

For every accepted status request, pc-control invokes one TCP dial operation
to `PC_CONTROL_SHUTDOWN_SSH_HOST` and `PC_CONTROL_SHUTDOWN_SSH_PORT`, or TCP
port `22` when that existing variable is absent. The operation has the
configured status probe timeout.

If the dial succeeds, pc-control closes the connection without sending SSH
authentication or protocol traffic, executing a remote command, or otherwise
communicating with the endpoint. If it fails or times out, pc-control reports
offline. pc-control performs no retry, fallback, repeated probe, second dial,
Wake-on-LAN operation, SSH authentication, SSH protocol traffic, remote
command, power-state check, boot-completion check, or health check beyond that
one TCP dial outcome.

## Required observable outcomes

- Every accepted status request causes one, and only one, logical TCP probe
  operation.
- Each accepted duplicate or concurrent status request causes its own
  independent single probe operation.
- A successful TCP dial returns `200 OK` with `{"status":"online"}`.
- A failed or timed-out TCP dial returns `200 OK` with
  `{"status":"offline"}`.
- Routing, method, query-string, and request-body failures cause zero probes.
- Invalid status timeout configuration prevents normal startup.
- Status reuses the configured shutdown SSH host and port and does not require
  SSH credentials or known-hosts data for the probe itself.
- Status performs no SSH authentication or protocol traffic, remote command,
  Wake-on-LAN, or pc-control retry behavior.
- Neither status result confirms physical power state, boot completion,
  successful SSH authentication, or general workstation health.
