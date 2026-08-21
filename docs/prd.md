# pc-control v0.1 — Product Requirements Document

## Problem statement

The operator needs a private, remote way to power on their Arch Linux
workstation when it is powered off. The workstation is connected through
Ethernet, and Wake-on-LAN has already been configured and verified on the
local network. The operator, NAS, and personal devices are connected through
Tailscale.

## Goals

- Allow the operator to remotely request that the preconfigured workstation
  be woken using Wake-on-LAN.
- Provide a headless, programmatic interface suitable for direct use with an
  API client and future consumption by other applications.
- Keep the service private: it must not be publicly reachable from the
  Internet.
- Provide immediate feedback about whether pc-control sent the wake request.

## Users and actors

### Operator

The single person authorized to use pc-control in v0.1.

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

## Functional requirements

1. pc-control shall support exactly one preconfigured workstation in v0.1.
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

## Non-functional product requirements

1. pc-control shall be portable across suitable runtime environments.
2. pc-control shall be accessible only through a private network boundary and
   shall not be publicly exposed to the Internet.
3. The product shall be headless and shall not provide a dedicated graphical
   web interface in v0.1.
4. v0.1 shall not require an application-level login, user-management,
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
- Automatic retry behavior
- User-visible request history, audit trail, or persistent notifications
- Shutdown, hibernation, reboot, maintenance, system updates, health checks,
  monitoring, or other workstation-management capabilities
- Second Brain functionality

## Success criteria

pc-control v0.1 is successful when the operator can, from a client within the
private network boundary, make a programmatic request that causes pc-control
to send one Wake-on-LAN request to the preconfigured workstation and receive
an immediate result indicating whether that send operation succeeded or
failed.

## Deferred product evolution

The following may be considered in future versions but are not v0.1
requirements: multi-user access, multiple workstations, workstation status,
shutdown and power-state controls, maintenance capabilities, notifications,
and integration with a larger personal Second Brain system.

## Architectural and implementation decisions deferred from this PRD

This document intentionally does not decide the programmatic protocol,
endpoint or request/response design, transport, mechanism for enforcing the
private network boundary, Wake-on-LAN implementation, configuration format,
container design, language, dependencies, package structure, logging
implementation, or testing approach. These belong to later architecture and
delivery phases.
