# Specification 0001: Wake-on-LAN

## Status

Approved for pc-control v0.1.

## Purpose and scope

pc-control v0.1 provides one private, programmatic command to request that
one preconfigured workstation be woken through Wake-on-LAN (WOL).

The service supports exactly one workstation. A client cannot select a
workstation or supply wake options.

This specification defines pc-control's observable HTTP, configuration, and
WOL behavior. Deployment and network controls, including restricting access
to the private network boundary, are outside the application.

## Terms

### Accepted wake command

A request is an accepted wake command only when pc-control has fully received
and validated an exact `POST /v1/wake` request with no query parameters and an
empty request body.

Requests that fail routing, method, query-string, or request-body validation
are not accepted wake commands.

### Wake-on-LAN send attempt

Each accepted wake command causes exactly one WOL send attempt. The attempt is
to send one UDP datagram with the Magic Packet defined in this specification to
the configured UDP destination and port.

### Local UDP send success

The local UDP send succeeds when pc-control successfully completes the local
UDP send operation. This is the only success condition for a wake command.

Local UDP send success does not confirm packet delivery, packet receipt by the
workstation, workstation power-on, online state, or boot completion.

## HTTP interface

### Wake command endpoint

The only wake command endpoint is the exact path:

```text
/v1/wake
```

The only supported method is `POST`.

The command accepts no identifier, options, or other command input.

### Routing and method handling

- A path other than the exact `/v1/wake` path, including `/v1/wake/`, returns
  `404 Not Found`.
- On the exact `/v1/wake` path, a method other than `POST` returns
  `405 Method Not Allowed` and includes `Allow: POST`.
- Method handling takes precedence over query-string and request-body
  validation. For example, `GET /v1/wake?x=1` returns `405 Method Not Allowed`.
- Routing or method failures must not cause a WOL send attempt.

### Query string and request body handling

For `POST /v1/wake`:

- Query parameters are not permitted. A request with query parameters returns
  `400 Bad Request` with error code `invalid_request`.
- The request body must be empty. A non-empty request body returns
  `400 Bad Request` with error code `invalid_request`, regardless of its
  contents or media type.
- `Content-Type` is irrelevant for an empty request and may be absent or
  present.
- pc-control does not return `415 Unsupported Media Type` in v0.1.
- Query-string or request-body validation failures must not cause a WOL send
  attempt.

### Success response

If the WOL send attempt completes as a successful local UDP send,
pc-control returns:

```http
200 OK
Content-Type: application/json
```

```json
{"result":"sent"}
```

`"sent"` means only that the local UDP send operation succeeded. It must not
be described as confirmation that the workstation received the packet or
changed power state.

### Error responses

Except for `HEAD` responses, every error response defined by this
specification has the following JSON envelope and includes
`Content-Type: application/json`:

```json
{"error":{"code":"...","message":"..."}}
```

The stable error codes and status mappings are:

| Condition | HTTP status | error.code |
| --- | --- | --- |
| Invalid `POST` command input | `400 Bad Request` | `invalid_request` |
| Unsupported method on `/v1/wake` | `405 Method Not Allowed` | `method_not_allowed` |
| Unknown path | `404 Not Found` | `not_found` |
| Valid command but failed local WOL send | `503 Service Unavailable` | `wake_failed` |

The error-code meanings are part of the v0.1 contract. Exact message text is
not a compatibility contract. Messages must be concise, safe,
human-readable descriptions and must not disclose raw system errors, stack
traces, socket details, configuration values, hardware identifiers, or other
internal implementation details.

### HEAD handling

Every `HEAD` response has no response body, regardless of path or status. It
retains the status and applicable routing or method headers of the
corresponding outcome.

In particular:

- `HEAD /v1/wake` returns `405 Method Not Allowed` with `Allow: POST` and no
  body.
- `HEAD` to an unknown path returns `404 Not Found` and no body.

No `HEAD` request causes a WOL send attempt.

### Client disconnect and cancellation

Once pc-control has fully received and validated a wake command, it performs
exactly one WOL send attempt even if the HTTP client disconnects or cancels
before a response is delivered.

Client cancellation after acceptance must not cancel the attempt or create an
additional attempt. A disconnected client has no guaranteed response.

If pc-control cannot fully receive and validate the request, the request is
not accepted and must not trigger WOL.

## Configuration

Runtime configuration is supplied through environment variables.

| Variable | Required | Meaning |
| --- | --- | --- |
| `PC_CONTROL_HTTP_LISTEN_ADDR` | Yes | HTTP supported bind address with an explicit numeric TCP port |
| `PC_CONTROL_WOL_MAC` | Yes | MAC address of the configured workstation |
| `PC_CONTROL_WOL_DESTINATION` | Yes | IPv4 UDP destination for the Magic Packet |
| `PC_CONTROL_WOL_PORT` | No | UDP destination port; defaults to `9` only when absent |

Leading or trailing whitespace is invalid for every configuration value.
Required variables must be present and non-empty.

Invalid or incomplete required configuration prevents normal service startup
and is reported through operational logs.

### HTTP listen address

`PC_CONTROL_HTTP_LISTEN_ADDR` is required and has no default. It must contain
a supported bind address with an explicit numeric TCP port from `1` through
`65535`.

Supported bind-address forms are:

- IPv4 literal, for example `127.0.0.1:8080` or `0.0.0.0:8080`
- Bracketed IPv6 literal, for example `[::1]:8080`
- Empty-host wildcard, for example `:8080`

Hostnames are not supported.

The selected bind address has no security semantics in this specification.
Deployment and surrounding network controls are responsible for preventing
public exposure and limiting access to authorized clients.

### WOL destination

`PC_CONTROL_WOL_DESTINATION` is required. It must be a conventional
dotted-decimal IPv4 literal with exactly four decimal octets, each from `0`
through `255`.

It must not contain a CIDR suffix, hostname, IPv6 form, leading-zero octet,
or leading or trailing whitespace. For example, `192.168.001.255` is invalid.

pc-control accepts any syntactically valid IPv4 literal and must not determine
whether that address is an appropriate limited broadcast, directed broadcast,
or other destination for the deployment. Selecting a correct destination is
the deployment operator's responsibility.

### WOL port

`PC_CONTROL_WOL_PORT` defaults to `9` only when the variable is absent.

When present, it must be non-empty and a base-10 integer from `1` through
`65535`. A present but empty, malformed, or out-of-range value prevents normal
service startup.

### Workstation MAC address

`PC_CONTROL_WOL_MAC` is required. It accepts only these case-insensitive
six-byte textual representations:

- Colon-separated: `aa:bb:cc:dd:ee:ff`
- Hyphen-separated: `aa-bb-cc-dd-ee-ff`
- Dotted: `aabb.ccdd.eeff`

Any other representation is invalid. The decoded MAC address must contain
exactly six bytes; otherwise normal service startup is prevented.

## Wake-on-LAN behavior

For every accepted wake command, pc-control performs exactly one attempt to
send one UDP datagram to `PC_CONTROL_WOL_DESTINATION` and to
`PC_CONTROL_WOL_PORT`, or UDP port `9` when that variable is absent.

The datagram is a 102-byte Wake-on-LAN Magic Packet for the configured
six-byte MAC address. Its bytes are, in order:

1. Six bytes with value `0xFF`.
2. Sixteen consecutive repetitions of the configured six-byte MAC address.

pc-control performs no additional datagram sends, automatic retries,
workstation probes, online-state checks, receipt checks, or boot-completion
checks for that command.

If the local UDP send succeeds, the command has the success response defined
above. If the local UDP send fails, the command has the `503 Service
Unavailable` and `wake_failed` outcome when an HTTP response can still be
delivered.

## Required observable outcomes

- Every accepted wake command causes one, and only one, WOL send attempt.
- Each accepted duplicate or concurrent wake command causes its own
  independent single attempt.
- The attempted WOL datagram has the exact 102-byte Magic Packet layout
  specified above.
- A successful local UDP send returns `200 OK` with the defined JSON result.
- A failed local UDP send returns `503 Service Unavailable` with
  `wake_failed` JSON when deliverable to the client.
- Routing, method, query-string, and request-body failures cause zero WOL
  send attempts.
- Invalid or incomplete required configuration prevents normal startup.
- A successful local UDP send is never reported as confirmation of packet
  delivery, workstation receipt, workstation online state, or power-state
  change.
- v0.1 performs no workstation state checks, retries, target selection,
  persistence, application-level authorization, or public-exposure policy
  enforcement.
