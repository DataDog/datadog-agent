> **TL;DR:** `pkg/logonduration` measures macOS user logon duration by querying OSLogStore for login window, credential entry, and desktop-ready timestamps, exposing the results via a system-probe HTTP module.

# pkg/logonduration

## Purpose

`pkg/logonduration` measures the macOS user logon duration by collecting timestamps from the system log store (OSLogStore). It records when the login window appeared, when the user entered credentials, and when the desktop became ready, allowing the agent to report how long a full macOS login sequence took. FileVault status is also captured because it affects when the login window appears and which log query to use.

**Platform**: macOS only. All functions have no-op stub implementations on other platforms (`timestamps_noop.go`).

**Privilege requirement**: accessing OSLogStore and running `fdesetup` both require root.

## Key elements

### Key types

#### `LoginTimestamps`

```go
type LoginTimestamps struct {
    LoginWindowTime  time.Time `json:"login_window_time"`
    LoginTime        time.Time `json:"login_time"`
    DesktopReadyTime time.Time `json:"desktop_ready_time"`
    FileVaultEnabled bool      `json:"filevault_enabled"`
}
```

The aggregate result type. Zero `time.Time` values indicate that the corresponding timestamp could not be collected (e.g., the relevant log entry was not found).

### Key functions

#### `GetLoginTimestamps() LoginTimestamps`

The main entry point. Calls all four sub-functions in order, logging warnings for any individual failure. Returns a `LoginTimestamps` with whatever timestamps were successfully collected. On non-macOS platforms it returns a zero-value struct.

#### Individual query functions (macOS only)

All are implemented via CGO calling into Objective-C code in `timestamps_darwin.m` (linked with the `Foundation` and `OSLog` frameworks):

| Function | Description |
|----------|-------------|
| `IsFileVaultEnabled() (bool, error)` | Runs `fdesetup status` to check FileVault state |
| `GetLoginWindowTime(fileVaultEnabled bool) (time.Time, error)` | Queries OSLogStore for when the login window appeared. The query differs depending on whether FileVault is enabled (different log messages are emitted in each case) |
| `GetLoginTime() (time.Time, error)` | Queries OSLogStore for `sessionDidLogin` — when the user successfully entered credentials |
| `GetDesktopReadyTime() (time.Time, error)` | Queries OSLogStore for the Dock checking in with `launchservicesd`, which signals that the desktop is ready for interaction |

### Configuration and build flags

macOS only. All functions have no-op stub implementations on other platforms (`timestamps_noop.go`, build tag `!darwin`). Requires root to access OSLogStore and run `fdesetup`.

#### CGO / Objective-C layer

- `timestamps_darwin.h` — declares C functions: `queryLoginWindowTimestamp`, `queryLoginTimestamp`, `queryDesktopReadyTimestamp`, `checkFileVaultEnabled`
- `timestamps_darwin.m` — Objective-C implementations using `OSLog` framework APIs
- CGO flags: `-x objective-c`, linked with `-framework Foundation -framework OSLog`
- Timestamps are returned as `C.double` (Unix seconds with sub-second precision) and converted to `time.Time` with nanosecond resolution by `unixFloatToTime`.

#### Noop stubs (`timestamps_noop.go`)

```go
//go:build !darwin
```

All functions return `errors.New("logonduration: not implemented on this platform")`. `GetLoginTimestamps` returns a zero-value `LoginTimestamps`.

## Usage

The package is exposed as a **system-probe HTTP module** on macOS (`cmd/system-probe/modules/logon_duration_darwin.go`). The module registers a `/check` endpoint that calls `GetLoginTimestamps()` and serializes the result as JSON. The endpoint is protected by a concurrency limit of 1.

A higher-level component (`comp/logonduration/`) wraps this module to provide the logon duration data to the rest of the agent.

```go
// Inside system-probe module handler:
timestamps := logonduration.GetLoginTimestamps()
// timestamps.LoginWindowTime, timestamps.LoginTime, timestamps.DesktopReadyTime
// timestamps.FileVaultEnabled
```

Because every call requires root and performs OSLogStore queries, callers should avoid calling `GetLoginTimestamps` in a tight loop. The system-probe endpoint enforces single-concurrency to prevent overlapping queries.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`comp/logonduration`](../../comp/logonduration.md) | The component (`comp/logonduration/impl`) is the **Windows** counterpart. It measures Windows boot/logon duration by parsing ETW AutoLogger trace files rather than OSLogStore, and submits the result as a Datadog Event via `eventplatform.Forwarder`. `pkg/logonduration` (this package) covers the **macOS** side and is surfaced as a system-probe HTTP module rather than a standalone fx component. The two share the same feature name and configuration concept but use entirely different data sources. |
| [`pkg/util/winutil`](../util/winutil.md) | On Windows, `comp/logonduration` depends on `pkg/util/winutil/etw.ProcessETLFile` and `etw.StopETWSession` to replay the boot-time ETL file. On macOS, `pkg/logonduration` uses the equivalent OS-native mechanism (CGO / Objective-C OSLog queries). The `etw/` sub-package of `winutil` is the Windows analogue of the CGO/OSLog layer in this package. |
| [`pkg/system-probe`](../system-probe.md) | This package is exposed as a system-probe module (`cmd/system-probe/modules/logon_duration_darwin.go`). The module registers a `GET /check` endpoint backed by `GetLoginTimestamps()` and is loadable when `system_probe_config.language_detection` or a dedicated config key is enabled. Agent-side checks call it via `pkg/system-probe/api/client.GetCheck[LoginTimestamps]`. |
