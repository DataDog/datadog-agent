# Developing AVD session collection

The Windows `vdi` check supports `provider: azure_virtual_desktop` on an
explicitly configured AVD session host. System-probe enumerates local Windows
sessions through WTS and the check submits metrics for RDP desktop sessions.
WTS is shared by AVD and other RDP hosts; this setting does not detect Azure
registration or query the Azure control plane.

## First development slice

| Metric | Scope | Meaning |
| --- | --- | --- |
| `vdi.session.count` | Host | Number of discovered RDP desktop sessions, including disconnected sessions |
| `vdi.session.active` | Host | Number of sessions in the WTS active state |
| `vdi.session.disconnected` | Host | Number of disconnected sessions |
| `vdi.session.present` | Session | Value `1` while the session is present in a fresh inventory |
| `vdi.session.duration` | Session | Seconds since the Windows session logon time, when available |
| `vdi.session.idle_time` | Session | Seconds since the last input reported by WTS, when available |

All metrics carry `vdi_provider:azure_virtual_desktop` and `vdi_protocol:rdp`.
Session metrics also carry `vdi_session_id` and `vdi_session_state`, and
`vdi_session_user` when user tags are enabled and a user is available.
The regular Agent hostname and instance tags apply.

Disconnected sessions remain in inventory. Listener, service, and console
sessions are excluded. WTS session identifiers are not used as connection
identifiers, so this slice does not emit `vdi.connection.*` for AVD.

WTS can clear the client protocol and station name after disconnect. On an
explicitly configured AVD host, a disconnected desktop with a user or logon
timestamp remains eligible even when its current client protocol is unavailable
or reports console. Named console sessions are excluded; WTS cannot establish
the historical protocol of an unnamed disconnected desktop from this snapshot.

The `vdi.session_enrichment.health` service check reports inventory failures.
Missing, failed, or stale inventory suppresses session metrics and counts;
a successful empty inventory emits zero counts. Partial inventory can emit
known session metrics but suppresses aggregate counts. No old user mapping is
reused following a failed refresh. Snapshots older than one minute are rejected.

`collect_user_tags: false` removes usernames from metric tags.
`collect_session_tags: false` disables individual session metrics while retaining
aggregate counts. These options control emitted tags; system-probe still needs
the local inventory to collect sessions.

## Enable on a development host

Build both the Agent and system-probe from this worktree. The system-probe
endpoint and AVD check must be deployed together.

In `C:\ProgramData\Datadog\datadog.yaml`:

```yaml
process_manager:
  enabled: false
vdi:
  enabled: true
```

When overlaying only the Agent and system-probe binaries onto an existing MSI,
use Windows SCM services by disabling process-manager supervision. The baseline
MSI's process manager and definition predate `vdi.enabled`; they can suppress the
SCM service without starting their own child. Process-manager supervision needs
both the matching process-manager binary and Windows definition from this branch.

Create `C:\ProgramData\Datadog\conf.d\vdi.d\conf.yaml`:

```yaml
init_config:
instances:
  - provider: azure_virtual_desktop
    collect_user_tags: true
    collect_session_tags: true
    inventory_stale_ttl: 300
```

Restart the development Agent services after installing the matching binaries.
Run the check as the normal Agent service identity as well as inspecting its
scheduled output; an elevated interactive run alone does not test the privilege
boundary. System-probe performs WTS collection under its existing service account.

## Validate on the AVD host

1. Compare `query session` output with the reported session IDs, usernames, and
   counts. Test with two different users on a multi-session host when available.
2. Disconnect one client without signing out: the session remains present,
   moves to disconnected state, and the disconnected count increases.
3. Reconnect and confirm that session tags and state match WTS. Sign out and
   confirm that the session is absent from the next inventory.
4. Check that duration and idle time are nonnegative. Keep timestamp-derived
   metrics absent when Windows does not provide the corresponding timestamp.
5. Stop system-probe temporarily: the check reports unhealthy inventory and
   emits no false zero session count or stale user-tagged measurements.
6. Disable user tags and then session tags and verify the documented behavior.
7. Confirm the AVD check does not require DCV to be installed and does not emit
   DCV metrics or connection identities.

## Follow-up work

Enumerate the counters installed on the actual AVD image before adding PDH
collection for input delay, RemoteFX network, and RemoteFX graphics. Validate
each counter-to-WTS join across disconnect/reconnect and multiple users.
Input-delay counters have sampling-interval requirements that must be handled
before scheduling them at the Agent check interval. Connection IDs, traffic
counter reset handling, Azure resource metadata, and process/session enrichment
remain separate development slices.

Microsoft references:

- [WTS session information](https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/ne-wtsapi32-wts_info_class)
- [Remote Desktop input-delay counters](https://learn.microsoft.com/en-us/windows-server/remote/remote-desktop-services/rds-rdsh-performance-counters)
- [Remote Desktop graphics counters](https://learn.microsoft.com/en-us/azure/virtual-desktop/remotefx-graphics-performance-counters)

To inspect installed counter instances from PowerShell on the session host:

```powershell
Get-Counter -ListSet 'RemoteFX*', 'User Input Delay*', 'Terminal Services*' |
    Select-Object CounterSetName, PathsWithInstances
```
