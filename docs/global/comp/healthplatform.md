# comp/healthplatform

**Team:** agent-health

## Purpose

`comp/healthplatform` collects and reports agent health issues to the Datadog
backend. It is the central hub for detecting, tracking, and forwarding
structured health problems detected by the agent or its integrations.

Concretely, the component:

- Runs periodic health checks registered by any part of the agent.
- Accepts ad-hoc issue reports from integrations via `ReportIssue`.
- Persists issue state to disk (`<run_path>/health-platform/issues.json`) so
  that issue continuity (new → ongoing → resolved) survives agent restarts.
- Forwards `HealthReport` payloads to the Datadog intake endpoint
  (`/api/v2/agenthealth`) every 15 minutes.
- Exposes a local HTTP endpoint (`GET /health-platform/issues`) on the agent
  API and includes issues in the flare archive.

When `health_platform.enabled` is `false` (default), a no-op implementation is
used and no I/O occurs.

## Key elements

### Component interface

```go
// comp/healthplatform/def/component.go

type HealthCheckFunc func() (*healthplatformpayload.IssueReport, error)

type Component interface {
    ReportIssue(checkID, checkName string, report *healthplatformpayload.IssueReport) error
    RegisterCheck(checkID, checkName string, checkFn HealthCheckFunc, interval time.Duration) error
    GetAllIssues() (int, map[string]*healthplatformpayload.Issue)
    GetIssueForCheck(checkID string) *healthplatformpayload.Issue
    ClearIssuesForCheck(checkID string)
    ClearAllIssues()
}
```

**`ReportIssue`** — the primary integration entry point. Pass a non-nil
`*IssueReport` to report an issue; pass `nil` to mark the issue as resolved.
The component looks up the issue template from the registry using
`report.IssueId`, fills in remediation metadata, and stores the result.

**`RegisterCheck`** — registers a `HealthCheckFunc` to be called periodically.
The function should return a non-nil `*IssueReport` when an issue is detected
and `nil` when healthy. A default interval of 15 minutes is used if `interval`
is 0 or negative.

**`GetAllIssues` / `GetIssueForCheck`** — thread-safe read access; return
proto-cloned copies to avoid external mutation.

**`ClearIssuesForCheck` / `ClearAllIssues`** — mark issues as resolved, update
on-disk state, and remove from in-memory map.

### Internal structure

```
healthPlatformImpl
├── issueRegistry   (*issues.Registry)   — issue templates + built-in checks
├── checkRunner     (*checkRunner)        — goroutine-per-check scheduler
├── forwarder       (*forwarder)          — HTTP client → Datadog intake
└── issues          map[string]*Issue    — current in-memory issue state
```

**`checkRunner`** starts one goroutine per registered check. Each goroutine
runs the check immediately on start, then ticks at its configured interval.
Checks are independent: a slow check does not delay others.

**`forwarder`** sends a `HealthReport` proto payload to
`https://event-platform-intake.<site>/api/v2/agenthealth` every 15 minutes.
The report includes hostname, agent version, and all current issues.

### Issue modules (`comp/healthplatform/impl/issues`)

Issue modules bundle detection logic with remediation metadata. Each module:

1. Implements the `Module` interface (`IssueID()`, `IssueTemplate()`,
   `BuiltInCheck()`).
2. Calls `issues.RegisterModuleFactory` from its `init()` function.
3. Is imported (blank import) in `health-platform.go` to trigger registration.

Built-in modules:

| Module | Issue ID | Detection |
|--------|----------|-----------|
| `issues/dockerpermissions` | `docker-permissions` | Checks Docker socket accessibility |
| `issues/checkfailure` | `check-failure` | Reported by the check runner on persistent failures |

To add a new issue type: create a new sub-package under
`comp/healthplatform/impl/issues/`, implement `Module`, and add a blank import
in `health-platform.go`.

### Issue state lifecycle

```
(first detected)  →  IssueStateNew
(still present)   →  IssueStateOngoing
(cleared)         →  IssueStateResolved  →  pruned after 24 h
```

State is persisted to `<run_path>/health-platform/issues.json` using an atomic
write (temp file + rename). On startup, active issues are reloaded and
re-enriched from the registry.

### fx wiring

```go
// comp/healthplatform/fx/fx.go
func Module() fxutil.Module {
    return fxutil.Component(fx.Provide(NewComponent))
}
```

Depends on: `config.Component`, `log.Component`, `telemetry.Component`,
`hostnameinterface.Component`, `compdef.Lifecycle`.

Provides: `healthplatformdef.Component`, `api.AgentEndpointProvider`
(GET `/health-platform/issues`), `flaretypes.Provider`.

## Usage

The component is wired into the main agent and cluster-agent. Other components
use it by injecting `healthplatform.Component`:

```go
// Report an issue from an integration
comp.ReportIssue("my-check-id", "My Check", &healthplatformpayload.IssueReport{
    IssueId: "docker-permissions",
    Context: map[string]string{"socket": "/var/run/docker.sock"},
})

// Clear the issue when resolved
comp.ReportIssue("my-check-id", "My Check", nil)
```

```go
// Register a periodic health check
comp.RegisterCheck("my-check-id", "My Check", func() (*IssueReport, error) {
    if healthy {
        return nil, nil
    }
    return &IssueReport{IssueId: "my-issue-id"}, nil
}, 10*time.Minute)
```

Enable in `datadog.yaml`:

```yaml
health_platform:
  enabled: true
```

## Related components

| Component / Package | Relationship |
|---|---|
| [`comp/core/status`](core/status.md) | `comp/healthplatform` does **not** register a `status.Provider`, so health-platform issues do not appear on the `datadog-agent status` page. The dedicated HTTP endpoint (`GET /health-platform/issues`) and the flare provider are the authoritative surfaces. If you want health-platform issues visible in `agent status`, implement `status.Provider` on the component and return a `status.InformationProvider` from the `Provides` struct. |
| [`comp/forwarder/defaultforwarder`](forwarder/defaultforwarder.md) | `comp/healthplatform` does **not** use the default metric forwarder. It maintains its own internal `forwarder` that posts `HealthReport` proto payloads directly to `https://event-platform-intake.<site>/api/v2/agenthealth` every 15 minutes, bypassing the retry queue and disk-serialization logic of `defaultforwarder`. |
