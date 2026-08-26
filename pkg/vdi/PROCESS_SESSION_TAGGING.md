# VDI process and session identity research

Status: research and design notes, not yet implemented

Last updated: 2026-08-17

## Goal

Allow processes and other process-associated telemetry from a Windows VDI host
to be filtered using the same authenticated user identity as the VDI session.

The initial provider is Amazon WorkSpaces using Amazon DCV. The design should
keep the Windows session primitives and data model reusable for other providers,
including Azure Virtual Desktop.

## Terminology

### WTS

WTS stands for Windows Terminal Services, the former name behind the Windows
Remote Desktop Services APIs. The API names still use the WTS prefix, for
example `WTSEnumerateSessions` and `WTSQuerySessionInformation`.

A WTS session ID is a numeric, host-local Windows identifier for a desktop or
service session. Windows associates every process with one of these sessions.
WTS session 0 normally contains services and system processes. Interactive
users generally run in nonzero sessions.

### Distinct user identities

Several identities can exist at the same time and must not be conflated:

- **Process user:** the Windows account in the process token, such as
  `DOMAIN\\Administrator`.
- **Windows session user:** the account logged into a WTS desktop session.
- **DCV session owner:** the DCV user that owns or created a DCV protocol
  session.
- **DCV authenticated user:** the remote identity authenticated for one DCV
  client connection, such as `alice@example.com`.

A process started with `RunAs`, a scheduled task, or an elevated token can have
a process user different from the interactive Windows session user. A DCV
permissions file can also permit an authenticated user other than the DCV
session owner to connect.

## Current implementation

The VDI branch already contains most of the primitives needed for process
enrichment:

- `pkg/vdi/model.WindowsSession` contains the Windows session ID, OS user,
  domain, state, logon time, and last-input time.
- `pkg/vdi/session/windows.SessionIDForProcess` wraps the Windows
  `ProcessIdToSessionId` API.
- `pkg/vdi/model.Session` has a provider-scoped ID and an optional internal
  `WindowsSessionID` field.
- DCV inventory contains the protocol session ID, session owner, connection ID,
  authenticated user, transport, client mode, and client timestamps.
- The VDI check attaches `vdi_connection_user` only to connection-scoped
  metrics. Session metrics do not inherit an authenticated connection user.
- The `DCV Server Processes` performance counter set includes a `Process
  Identifier` counter for DCV's `session_agent`, `system_agent`, and
  `user_agent` processes.
- Windows process payloads already contain the process token user in the
  `User` field and support arbitrary process tags.
- The Windows connections check can already retrieve PID-scoped tags from
  system-probe.

The pieces are not joined yet:

- `SessionIDForProcess` is not used.
- `Session.WindowsSessionID` is not populated for DCV.
- Reusable WTS primitives exist but are not invoked by the DCV collector.
- The process model does not retain a WTS session ID.
- Workloadmeta/tagger currently extracts service and GPU tags from process
  entities but has no generic VDI session context.
- Process Discovery payloads have a process user but no process tag list.

## Confirmed Windows behavior

Microsoft documents `ProcessIdToSessionId` as returning the Remote Desktop
Services session under which a process is running. Microsoft also documents
using WTS process enumeration and session filtering to select the processes in
a particular session.

This makes the WTS session ID the strongest local join key for grouping live
processes belonging to one Windows desktop session.

Important properties and limitations:

- The WTS session association is independent of the process token user.
- Services and machine-level background processes in session 0 must not inherit
  an interactive VDI user.
- WTS session IDs are only unique within one host and can eventually be reused.
  They are appropriate for a live snapshot but are not globally unique
  historical identifiers.
- A disconnected desktop can remain alive, along with its processes, and later
  be reconnected.
- A process belongs to a Windows desktop session. It does not inherently belong
  to one remote-client connection.
- Resolving some protected processes can require privileges unavailable to the
  restricted process-agent account. System-probe's LocalSystem boundary is the
  safer place to perform comprehensive resolution.
- PID reuse must be considered when passing enrichment snapshots between
  components. PID plus process creation time should identify a process instance.

Microsoft references:

- [ProcessIdToSessionId](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-processidtosessionid)
- [Process Enumeration](https://learn.microsoft.com/en-us/windows/win32/procthread/process-enumeration)
- [Remote Desktop Sessions](https://learn.microsoft.com/en-us/windows/win32/termserv/terminal-services-sessions)
- [TOKEN_INFORMATION_CLASS](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ne-winnt-token_information_class)
- [WTSEnumerateProcessesEx](https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/nf-wtsapi32-wtsenumerateprocessesexw)

## The unresolved DCV-to-Windows join

The DCV protocol session ID and the Windows WTS session ID are different
identifiers in different namespaces:

```text
DCV connection 17
  authenticated user: alice@example.com
            |
            v
DCV protocol session "console"
            |
            | mapping not yet proven
            v
Windows WTS session 3
            |
            +-- explorer.exe
            +-- chrome.exe
            +-- application.exe
```

AWS documents that Windows DCV uses a console session, only one console session
can exist at a time, and virtual sessions are Linux-only. However, `console` is
a DCV protocol session ID, not a numeric Windows WTS session ID. The available
AWS documentation does not expose a direct DCV session ID to WTS session ID
field.

Therefore, the implementation must not assume that the DCV session named
`console` is equivalent to the session returned by
`WTSGetActiveConsoleSessionId`. This relationship needs validation on actual
WorkSpaces hosts.

DCV references:

- [Starting Amazon DCV sessions](https://docs.aws.amazon.com/dcv/latest/adminguide/managing-sessions-start.html)
- [Viewing Amazon DCV sessions](https://docs.aws.amazon.com/dcv/latest/adminguide/managing-sessions-lifecycle-view.html)
- [DCV server sessions](https://docs.aws.amazon.com/dcv/latest/adminguide/dcv-server-sessions.html)
- [DCV server connections](https://docs.aws.amazon.com/dcv/latest/adminguide/dcv-server-connections.html)
- [DCV server processes](https://docs.aws.amazon.com/dcv/latest/adminguide/dcv-server-processes.html)

## Proposed identity and tagging model

Use each identifier for the scope it actually represents:

| Identity | Scope | Proposed use |
| --- | --- | --- |
| PID plus creation time | One process instance | Internal lookup and PID-reuse protection |
| WTS session ID | One live Windows desktop session | Primary process-to-session join |
| Windows user SID | One Windows account | Internal identity validation; optional telemetry |
| Process token user | Account executing a process | Keep the existing Process `User` field |
| Provider session ID | One provider session | Emit as `vdi_session_id` after an exact process-to-session join |
| Provider connection ID | One remote client connection | Emit as `vdi_connection_id` on connection telemetry, not ordinary processes |
| `vdi_connection_user` | Authenticated remote identity | Keep on connection telemetry, not sessions or ordinary processes |

Candidate process tags are:

```text
vdi_provider:aws_workspaces
vdi_protocol:dcv
vdi_session_id:console
```

`vdi_session_id` should only be attached after the DCV session's mapping to the
process's WTS session has been proven. `vdi_connection_id` should not normally
be attached to a process because several client connections can view or control
the same desktop and its processes. The WTS session ID remains an internal join
field rather than a second emitted session-ID tag.

An authenticated connection user is not the same as the process user. For
example, the following identities can coexist:

```text
process User: DOMAIN\\Administrator
vdi_connection_user:alice@example.com
```

`vdi_connection_user` is intentionally not a candidate process tag. A process
belongs to a Windows desktop session, while several independently authenticated
connections can view or control that session. Future user enrichment for
process telemetry requires a separate product contract rather than copying a
connection identity onto the shared session. Session identifiers should be
treated as high-cardinality tags.

## Proposed component boundary

Perform privileged correlation in the VDI system-probe module:

1. Collect DCV sessions and connections.
2. Collect Windows WTS sessions.
3. Enumerate current processes and resolve each PID's WTS session.
4. Establish the DCV protocol session to WTS session mapping.
5. Apply the mapping and freshness rules.
6. Expose a bounded PID enrichment snapshot containing enough process identity
   to prevent PID-reuse mistakes.

A conceptual response is:

```text
PID -> {
    process_creation_time,
    windows_session_id, // internal join field
    vdi_session_id,
    provider,
    protocol
}
```

The process-agent can fetch the snapshot once per process-check run and append
the returned tags to full process payloads. The Windows connections check can
merge the same PID-scoped context into network connection tags.

This approach:

- keeps privileged Windows and DCV access in system-probe;
- avoids putting provider-specific collection in the general process probe;
- supports both process and network telemetry with one identity decision;
- fails open when enrichment is unavailable; and
- avoids requiring workloadmeta protocol changes for the first implementation.

Extending workloadmeta and the tagger could be considered later if additional
consumers need the same process-session context. That path is broader because
process workloadmeta entities and their remote serialization currently have no
arbitrary VDI tag or session fields.

## Candidate DCV-to-WTS mapping

The strongest candidate is to use the DCV performance-counter PID:

```text
DCV session_agent counter instance
        -> Process Identifier counter
        -> ProcessIdToSessionId(PID)
        -> Windows WTS session ID
```

AWS confirms that `DCV Server Processes` exposes a PID and agent type. It does
not clearly document how a `session_agent` counter instance identifies its DCV
protocol session. We need to determine whether the instance name, process
command line, parent process, environment, or another DCV API provides that
last part of the join.

For WorkSpaces Personal, a fallback based on there being exactly one DCV
console session and exactly one active interactive WTS session may work. It
should be treated as a deliberately limited fallback, not as a general mapping
algorithm, and must never be used when the inventory is ambiguous.

## WorkSpaces validation plan

Capture all relevant namespaces at the same time:

```powershell
dcv list-sessions
dcv describe-session console --json
dcv list-connections console --json
Get-Counter '\DCV Server Processes(*)\Process Identifier'
Get-Process -IncludeUserName |
    Select-Object Id, ProcessName, SessionId, UserName
quser
```

Run the capture in these states:

1. Before a DCV user connects.
2. During one DCV connection.
3. After disconnecting while leaving the Windows session alive.
4. After reconnecting.
5. With an ordinary process launched from the interactive desktop.
6. With a process launched using `RunAs`.
7. With a service or scheduled task running as the same Windows account.
8. With two DCV identities authorized to connect to the same DCV session.
9. On WorkSpaces Applications with two simultaneous Windows sessions, if the
   target product supports that arrangement.
10. Across logoff and a later login to observe WTS session ID reuse behavior.

For every state, record:

- DCV protocol session ID and owner;
- DCV connection ID and authenticated user;
- DCV agent type, performance-counter instance, and PID;
- DCV agent PID's WTS session ID;
- interactive WTS session ID, state, user, domain, and logon time;
- representative desktop-process PIDs, creation times, session IDs, and token
  users; and
- the behavior of session and process IDs after disconnect, reconnect, and
  logoff.

## Open questions

1. How does a Windows DCV `session_agent` instance identify its DCV protocol
   session?
2. Does the DCV session agent run in the WTS session whose desktop is being
   streamed on WorkSpaces Personal?
3. How does this behave for WorkSpaces Applications and multiple simultaneous
   interactive users?
4. Can `dcv describe-session --json` on current Windows/DCV versions expose
   additional undocumented but stable mapping data?
5. Should process telemetry ever receive an authenticated remote-user identity,
   and how should that behave when several users connect to one session?
6. Should `windows_session_id` be emitted to customers or remain an internal
   join field?
7. Do backend Process and Network products accept and expose these proposed
   tags consistently?
8. Should usernames be normalized, hashed, or mapped to a stable provider ID?
   Simple string matching between `DOMAIN\\user`, UPN, and DCV authentication
   names is not reliable.
9. Which telemetry beyond full Process and Network Connections needs this
   context: Process Discovery, service discovery, USM, logs, APM, GPU metrics,
   or security events?

## Suggested next step

Run the validation matrix on one WorkSpaces Personal host and capture the raw
outputs without implementing a mapping first. If the DCV session-agent PID
provides a stable DCV-to-WTS bridge, document the invariant and build the
system-probe enrichment snapshot around it. If it does not, define a narrow,
explicitly guarded single-session fallback and continue investigating a
provider-supported mapping for multi-session products.
