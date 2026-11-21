# Logon Duration

This component measures how long a host took to boot and log a user in after a reboot, and sends
one Event Management event per boot. It runs on macOS and Windows when `logon_duration.enabled` is
true.

On start, the Agent compares the current boot time with the one stored in its persistent cache
(`logon_duration:last_boot_time`). If the host has rebooted, it collects the boot timeline, sends
one event and updates the cache.

- **macOS**: system-probe reads the timeline from the unified log. The Agent waits until a user
  owns `/dev/console` (checked every 10 s), then queries system-probe every 15 s for up to
  2 minutes until the login and desktop-ready timestamps are valid. A timestamp before boot,
  after now, or out of order (with a 5 s tolerance) makes the timeline invalid. If collection
  fails, the cache is not updated and the next Agent start tries again.
- **Windows**: the timeline comes from the boot autologger ETL trace, including Group Policy
  processing. A trace that fails to parse is not retried. The boot start, login UI start, session
  logon and desktop visible timestamps are checked with the same rules as on macOS.

# Format

The event is sent to the Event Management intake with `integration_id: system-notable-events` and
`system-notable-events.event_type: logon_duration`. The event timestamp is the boot time.

## Titles

- `Device booted up: Boot & login took <total> ms` - both the boot and the logon duration were
  measured.
- `Device booted up: Boot timeline incomplete` - at least one of them is missing.
- `Device booted up: Boot timeline invalid` - at least one timestamp is impossible, for example
  after a clock reset at boot. The event still carries the raw timeline and durations, so offsets
  and durations can be negative.

## Custom fields

The fields below are under `data.attributes.attributes.custom`.

- `agent_version` - **string**: the version of the Agent that sent the event. This was introduced
  in `7.87.0`; events without it come from an older Agent.
- `boot_timeline` - **list of milestones**, in timeline order. Only observed milestones are
  included. Each milestone has:
  - `id` - **string**: a stable identifier (see [Milestone IDs](#milestone-ids)).
  - `name` - **string**: the display name.
  - `offset_ms` - **number**: the position on the timeline, in ms from boot. Idle time at the
    login screen is removed so the timeline is contiguous. Can be negative (see
    [Negative values](#negative-values)).
  - `duration_ms` - **number**: the length of the milestone in ms, `0` for an instant.
  - `timestamp` - **string**: the wall-clock time in UTC, formatted as `2006-01-02T15:04:05.000Z`.
- `durations` - **dict of string to number**, all in ms:
  - `boot_duration_ms`: from boot to the login window (macOS) or the login UI (Windows).
  - `logon_duration_ms`: from the user's logon to the desktop being ready.
  - `total_boot_duration_ms`: `boot_duration_ms + logon_duration_ms`. Idle time at the login
    screen is not included.
  - One entry per other milestone with a non-zero duration, keyed by its `id`.
  - On macOS the three totals are always present, `0` when not measured. On Windows each total is
    only present when it was measured.
  - Can be negative (see [Negative values](#negative-values)).
- `filevault_enabled` - **bool**, **macOS only**: whether FileVault is on.
- `group_policy_details` - **dict**, **Windows only**, present when Group Policy ran during boot:
  - `computer`, `user` - **list**: one entry per Client-Side Extension (CSE) invocation, with:
    - `cse_id` - **string**: the CSE GUID, braced and uppercase.
    - `cse_name` - **string**: the CSE name, when known.
    - `offset_ms` - **number**: when the invocation started, measured like the milestones'
      `offset_ms`.
    - `duration_ms` - **number**: the invocation's wall-clock duration in ms.
    - `result` - **string**: `success`, `warning` or `error`.
    - `async` - **bool**: when true, `duration_ms` is the cost of dispatching a worker thread, not
      of the work itself.
    - `gpos` - **list**: the Group Policy objects applied, each with `id` (braced uppercase GUID)
      and `name` (when known).
    - `gpos_omitted` - **int**: GPO references left out to bound the payload size.
  - `computer_cses_omitted`, `user_cses_omitted` - **int**: invocations left out to bound the
    payload size.
- `invalid_milestones` - **list of string**, present when `timeline_status` is `invalid`: the
  timestamps that failed validation, in chain order (see [Timestamp IDs](#timestamp-ids)).
- `missing_milestones` - **list of string**, present when a timestamp was not found: the missing
  timestamps, in chain order (see [Timestamp IDs](#timestamp-ids)).
- `timeline_status` - **string**: `complete`, `incomplete` or `invalid`, matching the title.
  `invalid` wins when a timeline is both invalid and incomplete. Introduced in `7.87.0`.

## Milestone IDs

- macOS: `boot_duration`, `login_window_ready`, `logon_duration`.
- Windows: `boot_duration`, `login_ui_start`, `computer_group_policy`, `user_group_policy`,
  `logon_duration`, `profile_loaded`, `profile_created`, `execute_shell_commands`,
  `explorer_initializing`, `desktop_visible`, `desktop_startup_apps`.

## Negative values

Offsets and durations are computed from the raw timestamps and are never clamped:

- On a `complete` timeline each value can be slightly negative, by up to the 5 s tolerance.
- On an `invalid` timeline they can be off by any amount, up to decades after a clock reset.

## Timestamp IDs

`missing_milestones` and `invalid_milestones` name timestamps, not `boot_timeline` milestones:

- macOS: `login_window_ready`, `login`, `desktop_ready`.
- Windows: `boot_start`, `login_ui_start`, `session_logon`, `desktop_visible`.

## Changing the format

Any change that adds, removes, renames or redefines a field must update this README and note the
Agent release it shipped in, so that readers can tell from `agent_version` which layout an event
has.

# Example payload (macOS)

```json
{
  "data": {
    "type": "event",
    "attributes": {
      "host": "my-mac",
      "title": "Device booted up: Boot & login took 41000 ms",
      "category": "alert",
      "integration_id": "system-notable-events",
      "system-notable-events": {
        "event_type": "logon_duration"
      },
      "attributes": {
        "status": "ok",
        "priority": "3",
        "custom": {
          "agent_version": "7.87.0",
          "timeline_status": "complete",
          "filevault_enabled": true,
          "boot_timeline": [
            {
              "id": "boot_duration",
              "name": "Boot Duration",
              "offset_ms": 0,
              "duration_ms": 25000,
              "timestamp": "2026-11-19T08:00:00.000Z"
            },
            {
              "id": "login_window_ready",
              "name": "Login Window Ready",
              "offset_ms": 25000,
              "duration_ms": 0,
              "timestamp": "2026-11-19T08:00:25.000Z"
            },
            {
              "id": "logon_duration",
              "name": "Logon Duration",
              "offset_ms": 25000,
              "duration_ms": 16000,
              "timestamp": "2026-11-19T08:03:10.000Z"
            }
          ],
          "durations": {
            "boot_duration_ms": 25000,
            "logon_duration_ms": 16000,
            "total_boot_duration_ms": 41000
          }
        }
      },
      "message": "Total boot duration took 41000 ms.",
      "timestamp": "2026-11-19T08:00:00.000000Z"
    }
  }
}
```
