> **TL;DR:** `agentcrashdetect` registers the `agentcrashdetect` check, which reads Windows BSOD crash records left by Datadog kernel drivers in the registry and reports them once via the Agent Telemetry pipeline.

# comp/checks/agentcrashdetect

**Package:** `github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect`
**Team:** windows-products
**Platform:** Windows only (build tag `windows`)

## Purpose

`agentcrashdetect` registers the `agentcrashdetect` agent check, which detects Windows Blue Screen of Death (BSOD) crashes caused by Datadog kernel-mode drivers and reports them to Datadog via the Agent Telemetry pipeline.

The Datadog agent ships several Windows kernel drivers (NPM/USM, CWS process monitor, APM injector). A bug in one of these drivers can cause a Windows kernel panic (BSOD). This check surfaces those events so they appear in Datadog telemetry under the `agentbsod` event type rather than going unnoticed.

The check only activates when at least one of the relevant system-probe modules is enabled (`network_config.enabled`, `service_monitoring_config.enabled`, `runtime_security_config.enabled`, or `windows_crash_detection.enabled`). When none are enabled, `Run()` returns immediately without doing anything.

## Key Elements

### Key interfaces

```go
// def/component.go
type Component interface{}
```

Marker interface; all behaviour is the side effect of registering the check factory during startup.

### Key types

**`AgentCrashDetect` check struct:**

| Field | Type | Purpose |
|-------|------|---------|
| `reporter` | `*crashreport.WinCrashReporter` | Reads crash data from Windows registry |
| `crashDetectionEnabled` | bool | Set during `Configure`; gates all `Run()` logic |
| `probeconfig` | `compsysconfig.Component` | System-probe config, used to check enabled flags |
| `atel` | `agenttelemetry.Component` | Sends the `agentbsod` telemetry event |

**`AgentBSOD` telemetry payload:**

```go
type AgentBSOD struct {
    Date         string                // Crash timestamp
    Offender     string                // Faulting module
    BugCheck     string                // BSOD stop code
    BugCheckArg1-4 string             // Stop code arguments
    Frames       []AgentBSODStackFrame // Call stack (instruction pointers)
    AgentVersion string
}
```

Sent as a JSON-marshalled byte slice via `agenttelemetry.Component.SendEvent("agentbsod", payload)`.

**Datadog driver list** — the check only reports crashes where one of these drivers appears in the call stack:

| Driver name | Purpose |
|-------------|---------|
| `ddnpm` | Network Performance Monitoring / USM |
| `ddprocmon` | CWS process monitoring |
| `ddinjector` | APM application tracing |
| `crashdriver` | Testing only |

### Key functions

**Crash detection mechanism:** Crash data is written to the Windows registry by the `wincrashdetect` system-probe module after the system recovers from a BSOD:

```
HKEY_LOCAL_MACHINE\SOFTWARE\Datadog\Datadog Agent\agent_crash_reporting
```

`WinCrashReporter.CheckForCrash()` reads and returns any unprocessed crash record. The `lastReported` registry key prevents the same crash from being reported more than once.

`NewComponent` calls `core.RegisterCheck` during `OnStart`.

### Configuration and build flags

Requires the following fx dependencies:

| Dependency | Purpose |
|------------|---------|
| `compsysconfig.Component` | Read system-probe enabled flags |
| `agenttelemetry.Component` | Send crash telemetry |
| `compdef.Lifecycle` | Register `OnStart` hook |

The check activates only when at least one of `network_config.enabled`, `service_monitoring_config.enabled`, `runtime_security_config.enabled`, or `windows_crash_detection.enabled` is `true`.

## Usage

The component is included in `comp/checks/bundle.go` (non-Windows stub) and `cmd/agent/subcommands/run/command_windows.go` for Windows:

```go
agentcrashdetectfx.Module()
// ...
fx.Invoke(func(_ agentcrashdetect.Component) {})
```

No additional `conf.yaml` is required. The check is enabled automatically when a supported system-probe module is active. The single config field (`enabled: true/false`) can be placed in the init config to explicitly disable the check if needed.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/windowsdriver`](../../pkg/windowsdriver.md) | The drivers whose names appear in the BSOD call stack (`ddnpm`, `ddprocmon`, `ddinjector`) are the same kernel-mode drivers managed by `pkg/windowsdriver`. `ddnpm` is the NPM/USM network driver; `ddprocmon` is the CWS process-monitoring driver used by `pkg/windowsdriver/procmon`; `ddinjector` is the APM injection driver exposed by `pkg/windowsdriver/ddinjector`. When a BSOD occurs, the Windows crash-recovery path writes the stack frames to the registry; this check reads those frames and checks for the driver names against the `ddDrivers` map. |
| [`pkg/util/winutil`](../../pkg/util/winutil.md) | The crash record is stored in the registry under `HKLM\SOFTWARE\Datadog\Datadog Agent\agent_crash_reporting`. `crashreport.WinCrashReporter` uses `golang.org/x/sys/windows/registry` to read and mark records as reported. This is a lower-level registry operation distinct from the SCM and Event Log utilities in `pkg/util/winutil`, but both packages are Windows-only and share the `//go:build windows` constraint. |

### Relationship to the `wincrashdetect` system-probe module

The data flow for BSOD detection spans two components:

```
BSOD occurs
    │
    ▼
Windows crash-dump recovery (system boots)
    │
    ▼
wincrashdetect system-probe module
  (pkg/collector/corechecks/system/wincrashdetect/probe)
    │  writes crash record to registry
    ▼
HKLM\...\agent_crash_reporting
    │
    ▼
agentcrashdetect check (this component)
  crashreport.WinCrashReporter.CheckForCrash()
    │  reads and marks record as reported
    ▼
agenttelemetry.Component.SendEvent("agentbsod", payload)
    │
    ▼
Datadog Agent Telemetry pipeline
```

The `wincrashdetect` module must be enabled (via `windows_crash_detection.enabled: true` in the system-probe config) for crash records to be written to the registry. Without it, `agentcrashdetect` runs every check interval but does nothing.

### Deduplication

`WinCrashReporter` uses the `lastReported` registry key to ensure each crash is sent to Datadog exactly once. Subsequent `Run()` calls return `nil` for the same crash without re-submitting telemetry.
