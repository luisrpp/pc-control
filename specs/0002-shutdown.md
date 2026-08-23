# Specification 0002: Graceful Shutdown

## Status

Approved for pc-control v0.2.

## Purpose and scope

pc-control v0.2 provides one private, programmatic command to request that the
one preconfigured workstation begin a normal graceful operating-system
shutdown. A client cannot select a workstation, command, option, or shutdown
mode.

This specification defines observable shutdown HTTP behavior, SSH-shutdown
operation semantics, configuration, and safety requirements. It does not
define deployment or network controls that restrict access to the private
network boundary.

## Terms

### Accepted shutdown command

A request is an accepted shutdown command only when pc-control has fully
received and validated an exact `POST /v1/shutdown` request with no query
parameters and an empty request body. Requests that fail routing, method,
query-string, or request-body validation are not accepted shutdown commands.

### SSH shutdown operation

Each accepted shutdown command causes exactly one SSH shutdown operation. The
operation uses the configured target, port, user, dedicated private key, and
known-hosts data, under the configured end-to-end timeout.

The initial pc-control implementation issues the fixed client-side SSH command
`systemctl poweroff`. pc-control exposes no command-selection or arbitrary
remote-command capability. The target SSH server may use a forced command or
another deployment restriction that ignores or replaces this client-side
command.

### Shutdown initiation success

Shutdown initiation succeeds when pc-control observes successful completion of
the configured remote shutdown capability within the configured timeout. This
means the target operating system accepted or initiated normal graceful
shutdown as observable through that capability.

It does not prove that the literal client-side `systemctl poweroff` string was
executed remotely, that the workstation has completed shutdown, or that it is
offline or powered off.

## HTTP interface

### Shutdown command endpoint

The only shutdown command endpoint is the exact path:

```text
/v1/shutdown
```

The only supported method is `POST`. The command accepts no identifier,
options, command text, or other input.

### Routing and method handling

- A path other than the exact `/v1/shutdown` path, including
  `/v1/shutdown/`, returns `404 Not Found`.
- On the exact `/v1/shutdown` path, a method other than `POST` returns
  `405 Method Not Allowed` and includes `Allow: POST`.
- Method handling takes precedence over query-string and request-body
  validation. For example, `GET /v1/shutdown?x=1` returns `405 Method Not
  Allowed`.
- Routing or method failures must not cause an SSH shutdown operation.
- The existing `/v1/wake` interface and its Specification 0001 behavior are
  unchanged.

### Query string and request body handling

For `POST /v1/shutdown`:

- Query parameters are not permitted. A request with query parameters returns
  `400 Bad Request` with error code `invalid_request`.
- The request body must be empty. A non-empty request body returns
  `400 Bad Request` with error code `invalid_request`, regardless of content
  or media type.
- A trailing `?` with no query content is accepted.
- `Content-Type` is irrelevant for an empty request and may be absent or
  present.
- pc-control does not return `415 Unsupported Media Type` for this command.
- Query-string or request-body validation failures must not cause an SSH
  shutdown operation.

### Success response

If the SSH shutdown operation completes with shutdown initiation success,
pc-control returns:

```http
202 Accepted
Content-Type: application/json
```

```json
{"result":"initiated"}
```

`"initiated"` means only that pc-control observed successful initiation or
acceptance of the configured graceful-shutdown capability. It must not be
described as confirmation that the literal client-side command executed, that
the workstation shut down, or that it is offline.

### Error responses

Except for `HEAD` responses, every error response defined by this specification
has the following JSON envelope and includes `Content-Type: application/json`:

```json
{"error":{"code":"...","message":"..."}}
```

The stable error codes and status mappings are:

| Condition | HTTP status | error.code |
| --- | --- | --- |
| Invalid `POST` command input | `400 Bad Request` | `invalid_request` |
| Unsupported method on `/v1/shutdown` | `405 Method Not Allowed` | `method_not_allowed` |
| Unknown path | `404 Not Found` | `not_found` |
| Valid command but failed SSH shutdown operation | `503 Service Unavailable` | `shutdown_failed` |

The error-code meanings are part of the v0.2 contract. Exact message text is
not a compatibility contract. Messages must be concise, safe, and
human-readable. They must not disclose SSH internals, credentials, key
material, known-hosts data, target host details, raw system errors, stack
traces, or remote command output.

An SSH connection, timeout, authentication, host-key-verification, or remote
operation failure all use `shutdown_failed`. In particular, a connection can
fail after remote shutdown has begun; the actual workstation state is then
indeterminate and pc-control neither reports success nor retries.

### HEAD handling

Every `HEAD` response has no response body, regardless of path or status. It
retains the status and applicable routing or method headers of the corresponding
outcome.

In particular:

- `HEAD /v1/shutdown` returns `405 Method Not Allowed` with `Allow: POST` and
  no body.
- `HEAD` to an unknown path returns `404 Not Found` and no body.

No `HEAD` request causes an SSH shutdown operation.

### Client disconnect and cancellation

Once pc-control has fully received and validated a shutdown command, it
performs exactly one SSH shutdown operation even if the HTTP client disconnects
or cancels before a response is delivered.

Client cancellation after acceptance must not cancel the operation or create an
additional operation. A disconnected client has no guaranteed response.

If pc-control cannot fully receive and validate the request, the request is
not accepted and must not trigger SSH shutdown.

## Configuration

Runtime configuration is supplied through environment variables. Secrets and
host identity material are supplied through deployment-mounted files, not
environment-variable contents or source code.

The existing required Wake-on-LAN configuration remains required because the
v0.2 service exposes both configured capabilities. The shutdown configuration
is in addition to the variables defined by Specification 0001.

| Variable | Required | Meaning |
| --- | --- | --- |
| `PC_CONTROL_SHUTDOWN_SSH_HOST` | Yes | SSH target hostname or IP literal, without a port. |
| `PC_CONTROL_SHUTDOWN_SSH_PORT` | No | SSH TCP destination port; defaults to `22` only when absent. |
| `PC_CONTROL_SHUTDOWN_SSH_USER` | Yes | Dedicated restricted target account. |
| `PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH` | Yes | Path to the read-only mounted private-key file for the dedicated key. |
| `PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH` | Yes | Path to known-hosts data used for mandatory host-key verification. |
| `PC_CONTROL_SHUTDOWN_TIMEOUT` | No | End-to-end SSH shutdown-operation timeout; defaults to `10s` only when absent. |

Leading or trailing whitespace is invalid for every configuration value.
Required variables must be present and non-empty. Invalid or incomplete
required configuration prevents normal service startup and is reported through
safe operational diagnostics.

### SSH target, port, and user

`PC_CONTROL_SHUTDOWN_SSH_HOST` and `PC_CONTROL_SHUTDOWN_SSH_USER` are required
and have no defaults. The host may be a deployment-selected DNS hostname or IP
literal, but it must not include a URL scheme or port. The user is the
dedicated restricted target account. The deployment operator must ensure the
known-hosts data identifies the configured host and port according to the
known-hosts format.

`PC_CONTROL_SHUTDOWN_SSH_PORT` defaults to `22` only when absent. When
present, it must be non-empty and a base-10 integer from `1` through `65535`.
A present but empty, malformed, or out-of-range value prevents normal startup.

### Private key and known-hosts files

`PC_CONTROL_SHUTDOWN_SSH_PRIVATE_KEY_PATH` must identify a private-key file
mounted read-only such that it is not writable from inside the pc-control
runtime. `PC_CONTROL_SHUTDOWN_SSH_KNOWN_HOSTS_PATH` must identify a readable
mounted known-hosts file; a read-only mount is appropriate where the deployment
supports it, but is not a separate v0.2 runtime requirement. Their contents
must not be logged or returned to clients.

The private-key file must contain one valid unencrypted private key accepted
by the selected Go-native SSH implementation. Encrypted-key and SSH-agent
support are not part of v0.2. The known-hosts file must be syntactically valid
for the selected implementation. Unreadable files or malformed contents
prevent normal startup. A missing or non-matching host key prevents the SSH
shutdown operation; trust on first use is forbidden.

### Shutdown timeout

`PC_CONTROL_SHUTDOWN_TIMEOUT` defaults to `10s` only when absent. When
present, it must be a non-empty, positive Go duration such as `10s` or
`1500ms`. It covers connection, authentication, the remote operation, and
completion observable by pc-control. An empty, malformed, zero, or negative
value prevents normal startup.

## SSH shutdown behavior and security boundary

For every accepted shutdown command, pc-control performs exactly one SSH
shutdown operation using the configured target, port, user, dedicated key,
known-hosts data, and timeout. The initial client-side operation is the fixed
`systemctl poweroff` command. pc-control performs no additional SSH operations,
automatic retries, availability probes, online-state checks, offline checks,
or arbitrary remote commands.

The configured credential is a shutdown capability: it must be provisioned to
initiate only the configured normal graceful shutdown. It must not grant
general-purpose interactive shell access or arbitrary command execution. The
target-side mechanism is deployment behavior; a forced command in
`authorized_keys` is preferred, but another mechanism is acceptable only if it
enforces the same capability boundary and least privilege for required sudo or
systemd authorization.

The adapter must verify the target host key against configured known-hosts data
before accepting the SSH connection. It must never accept an unknown host key
automatically.

If the operation completes with shutdown initiation success, pc-control has
the success response defined above. If it fails at any point, pc-control has
the `503 Service Unavailable` and `shutdown_failed` outcome when an HTTP
response can still be delivered. No outcome confirms fully powered-off state.

## Required observable outcomes

- Every accepted shutdown command causes one, and only one, SSH shutdown
  operation.
- Each accepted duplicate or concurrent shutdown command causes its own
  independent single operation.
- A successful remote shutdown capability initiation returns `202 Accepted`
  with the defined JSON result.
- A failed SSH shutdown operation returns `503 Service Unavailable` with
  `shutdown_failed` JSON when deliverable to the client.
- Routing, method, query-string, and request-body failures cause zero SSH
  shutdown operations.
- Invalid or incomplete required configuration prevents normal startup.
- pc-control neither proves nor reports that the literal client-side command
  executed, that the workstation is offline, or that shutdown completed.
- v0.2 performs no state checks, retries, target selection, persistence,
  application-level authorization, public-exposure policy enforcement, forced
  power-off, or arbitrary remote command execution.
