# pc-control v0.2 — Product Requirements Document

## Problem statement

The operator needs a private, remote way to power on and gracefully shut down
one preconfigured workstation. Wake-on-LAN is configured for that workstation,
and access is provided through a deployment-specific private network boundary.

## Goals

- Allow the operator to remotely request that the preconfigured workstation
  be woken using Wake-on-LAN.
- Allow the operator to remotely request that the same preconfigured
  workstation begin a normal graceful shutdown.
- Provide a headless, programmatic interface suitable for direct use with an
  API client and future consumption by other applications.
- Keep the service private: it must not be publicly reachable from the
  Internet.
- Provide immediate feedback about whether pc-control sent the wake request.

## Users and actors

### Operator

The single person authorized to use pc-control in v0.2.

### Authorized client

A client permitted through the private network boundary that invokes
pc-control's programmatic remote interface on the operator's behalf.

### Preconfigured workstation

The single workstation that pc-control can request to wake, shut down, and
observe through its configured endpoints.

## Use cases

### Request workstation wake

1. An authorized client sends a wake request through pc-control's remote
   interface.
2. pc-control performs one Wake-on-LAN attempt for the preconfigured
   workstation.
3. pc-control immediately reports whether it sent the wake request or could
   not perform the operation.

The request may be made while the workstation is already running; pc-control
does not determine workstation status before sending the request.

### Request workstation shutdown

1. An authorized client sends a shutdown request through pc-control's remote
   interface.
2. pc-control performs one remote shutdown operation for the preconfigured
   workstation.
3. pc-control immediately reports whether it observed successful initiation
   of the shutdown capability or could not perform the operation.

pc-control does not check workstation availability or power state before the
operation, and a successful result does not confirm that the workstation is
offline or fully powered off.

## Functional requirements

1. pc-control shall support exactly one preconfigured workstation in v0.2.
2. pc-control shall expose a programmatic remote interface through which an
   authorized client can request that workstation be woken.
3. For each accepted wake request, pc-control shall perform exactly one
   Wake-on-LAN attempt.
4. pc-control shall provide an immediate result to the requesting client.
5. pc-control shall indicate success when it has successfully sent the wake
   request.
6. pc-control shall indicate immediate failure when it cannot perform the
   wake operation.
7. pc-control shall not determine whether the workstation is powered on or
   online before, during, or after a wake request.
8. pc-control shall not automatically retry a failed wake attempt.
9. pc-control shall expose a programmatic remote interface through which an
   authorized client can request a normal graceful shutdown of the
   preconfigured workstation.
10. For each accepted shutdown request, pc-control shall perform exactly one
    remote shutdown operation.
11. pc-control shall indicate shutdown success only when the remote shutdown
    capability was successfully initiated or accepted as observable by
    pc-control. It shall not treat this result as confirmation that the
    workstation is offline or powered off.
12. pc-control shall not determine workstation availability, online state, or
    power state before, during, or after a shutdown request.
13. pc-control shall not automatically retry a failed or indeterminate
    shutdown operation.
14. The credential used for remote shutdown shall be capable only of
    initiating the configured graceful shutdown and shall not provide
    general-purpose shell access or arbitrary command execution.

## Non-functional product requirements

1. pc-control shall be portable across suitable runtime environments.
2. pc-control shall be accessible only through a private network boundary and
   shall not be publicly exposed to the Internet.
3. The product shall be headless and shall not provide a dedicated graphical
   web interface in v0.2.
4. v0.2 shall not require an application-level login, user-management,
   roles, or permissions system.

## Constraints

- Deployment network controls are responsible for keeping the service private
  and limiting access to authorized clients.
- The selected deployment must provide the network reachability needed for
  Wake-on-LAN and the configured SSH endpoint.
- Deployment-specific values, including addresses, hostnames, ports,
  credentials, and hardware identifiers, are not fixed product requirements.

## Scope

### In scope

- One operator
- One preconfigured workstation
- A programmatic private remote wake request
- One Wake-on-LAN attempt per request
- Immediate sent-or-failed feedback

### Out of scope

- A graphical web interface
- Public Internet exposure
- Application-level login, multi-user management, roles, or permissions
- Selecting, registering, or managing multiple workstations
- Determining whether the workstation is online or already running
- Confirming boot completion or Wake-on-LAN delivery
- Confirming shutdown completion, offline state, or powered-off state
- Automatic retry behavior
- Forced power-off, reboot, hibernation, maintenance, system updates, health
  checks, monitoring, or arbitrary remote command execution
- User-visible request history, audit trail, or persistent notifications
- Second Brain functionality

## Success criteria

pc-control v0.2 is successful when the operator can, from a client within the
private network boundary, make programmatic requests that cause pc-control to
perform one Wake-on-LAN attempt or one remote graceful-shutdown operation for
the preconfigured workstation and receive an immediate result for each local
operation. A shutdown success indicates initiation or acceptance of the
shutdown capability, not confirmed power-off.

## Deferred product evolution

The following may be considered in future versions but are not v0.2
requirements: multi-user access, multiple workstations, workstation status,
shutdown completion or power-state confirmation, forced power controls,
maintenance capabilities, notifications, and integration with a larger
personal Second Brain system.

## Architectural and implementation decisions deferred from this PRD

This document intentionally does not decide the programmatic protocol,
endpoint or request/response design, mechanism for enforcing the private
network boundary, Wake-on-LAN implementation, shutdown adapter details,
configuration format, container design, language, dependencies, package
structure, logging implementation, or testing approach. These belong to later
architecture and delivery phases.
