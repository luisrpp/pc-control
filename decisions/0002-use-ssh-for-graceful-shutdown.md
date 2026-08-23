# ADR 0002: Use SSH with least privilege for graceful shutdown

## Status

Accepted

## Context

pc-control v0.2 adds a private command to request graceful shutdown of a
single configured workstation with a compatible systemd and OpenSSH setup. The
application must make one remote shutdown operation per accepted command,
report only its immediate observable outcome, avoid retries and state checks,
and never expose arbitrary remote command execution.

The target can initiate normal graceful shutdown through `systemctl poweroff`.
The pc-control deployment needs a remote-management mechanism that works from
the service host, remains independent of the application core, and gives a
compromised pc-control credential the smallest practical capability.

## Decision

Use a Go-native SSH shutdown adapter behind a narrow semantic shutdown port.
The adapter must not invoke an external `ssh` binary and must not provide a
general command-execution API.

The initial client-side operation issued by the adapter is the fixed command
`systemctl poweroff`. The capability contract is more important than proving
that literal command ran: a successful result means the remote graceful
shutdown capability was initiated or accepted as observable by pc-control.
The target SSH server may use a forced command or another restriction mechanism
that ignores or replaces the requested client command.

SSH authentication and target provisioning must use all of the following:

- A dedicated SSH key for pc-control, supplied through a mounted read-only
  private-key file rather than source code or environment-variable contents.
- Mandatory host-key verification using configured pinned known-hosts data;
  trust on first use is not permitted.
- A dedicated non-interactive target account.
- A target-side policy that grants this credential only the capability to
  initiate the configured graceful shutdown, with least privilege for any
  required sudo or systemd authorization.
- No general-purpose interactive shell or arbitrary command-execution
  capability for that credential.

A forced command in `authorized_keys` is the preferred target-side policy, but
it is deployment/provisioning behavior rather than an application dependency.

## Consequences

- The application core remains independent of SSH, credentials, host keys, and
  command strings; it depends only on the narrow shutdown port.
- The deployment gains a focused SSH dependency and must securely provision
  the target account, dedicated key, private-key file, known-hosts data, and
  any narrowly scoped elevation policy.
- The service remains compatible with a minimal runtime image because it does
  not require an external SSH executable.
- Host identity is verified before credentials or remote operations are
  accepted, preventing trust-on-first-use connections.
- Compromise of the pc-control credential is constrained to the configured
  shutdown capability rather than interactive access to the workstation.
- A successful SSH operation does not confirm workstation power-off. A
  transport failure after shutdown initiation may leave workstation state
  indeterminate; pc-control does not retry automatically.
