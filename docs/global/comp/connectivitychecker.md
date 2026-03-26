# comp/connectivitychecker — Connectivity Checker Component

**Import path:** `github.com/DataDog/datadog-agent/comp/connectivitychecker/def`
**Team:** fleet
**Importers:** `cmd/agent/subcommands/run` (core agent)

## Purpose

`comp/connectivitychecker` periodically tests the agent's ability to reach Datadog endpoints and publishes the results to the inventory agent. The results are sent to the Datadog backend as part of the `inventories` payload, where they power connectivity diagnostics in the Datadog UI.

It runs independently in the background, requiring no interaction from other components at runtime. If the feature is disabled (`inventories_diagnostics_enabled: false`) the background goroutine exits immediately after starting.

## Package layout

| Package | Role |
|---|---|
| `comp/connectivitychecker/def` | Component interface |
| `comp/connectivitychecker/impl` | Timer loop, orchestration, lifecycle |
| `comp/connectivitychecker/checker` | Core diagnostic logic (`Check` function) |
| `comp/connectivitychecker/fx` | fx `Module()` wiring `impl` |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/connectivitychecker/def
type Component interface{}
```

No methods are exported. The component operates entirely through its lifecycle hooks and an internal timer.

## Implementation details

**Timer loop:** After `OnStart`, the component waits an initial delay of 30 seconds before the first check (to avoid racing with agent startup), then repeats every 10 minutes. The loop runs in a goroutine and exits when the stop channel is closed or the component context is cancelled.

**Config-change reactivity:** The component registers a `config.OnUpdate` hook. When any configuration value changes, the timer is restarted immediately (delay = 0), so updated proxy or endpoint settings take effect at the next check cycle without waiting the full 10-minute interval.

**Diagnosis:** Each cycle calls `checker.Check`, which aggregates results from three sources:

1. `connectivity.DiagnoseInventory` — tests HTTP reachability of the configured Datadog intake endpoints (metrics, logs, APM, etc.), using the agent's configured proxy settings.
2. `eventplatformimpl.Diagnose` — tests event platform forwarder endpoints.
3. `connectivity.Diagnose` — runs the general connectivity diagnose suite (same checks as `agent diagnose`).

Each individual check produces a `DiagnosisPayload`:

```go
type DiagnosisPayload struct {
    Status      string            // "success" or "failure"
    Description string            // human-readable check name
    Error       string            // truncated to 200 chars on failure
    Metadata    map[string]string // optional key-value details, may include "remediation"
}
```

Results are grouped under the key `"connectivity"` and stored via `inventoryAgent.Set("diagnostics", diagnoses)`.

## fx wiring

```go
import connectivitycheckerfx "github.com/DataDog/datadog-agent/comp/connectivitychecker/fx"

// In your fx app:
connectivitycheckerfx.Module()
```

The module requires `log.Component`, `config.Component`, `compdef.Lifecycle`, and `inventoryagent.Component`.

## Usage

The component is entirely self-contained. Include the fx module and the component manages itself:

```go
// In the agent run command, the component is injected to bind its lifecycle:
func run(...) error {
    // connectivitycheckerfx.Module() included in the fx.Options above
    // No direct method calls needed
}
```

**Checking results** (indirect, via inventory agent):

Connectivity results are accessible through the `datadog_agent` inventory metadata payload that [`comp/metadata/inventoryagent`](metadata/inventoryagent.md) periodically sends to the Datadog backend. They appear under the `diagnostics` key as a map with a `"connectivity"` entry containing an array of `DiagnosisPayload` objects.

The component stores results with:
```go
c.inventoryAgent.Set("diagnostics", diagnoses)
```

This call triggers `inventoryagent.Refresh()` if the value changed, so new check results reach the backend as soon as the minimum interval (`inventories_min_interval`, default 60 s) allows — not only at the `inventories_max_interval` boundary.

**Relationship to `agent diagnose`:**

The `connectivity.DiagnoseInventory` and `connectivity.Diagnose` calls in `checker.Check` run the same checks as `datadog-agent diagnose --include connectivity-datadog-core-endpoints`. The connectivity checker automates this on a timer so the results are always fresh in the UI without manual intervention. See [`pkg/diagnose`](../pkg/diagnose/diagnose.md) for the full suite catalog and how to register additional suites.

## Configuration

| Key | Description |
|---|---|
| `inventories_diagnostics_enabled` | Enable/disable the connectivity check loop (checked on each timer tick and on config update) |

When `inventories_diagnostics_enabled` is `false`, the timer goroutine starts but immediately returns without running any checks.

## Notes

- The initial 30-second delay prevents the checker from running before the agent has fully initialized its forwarders and proxy configuration.
- The `Error` field in a failed diagnosis is truncated at 200 characters to keep inventory payloads compact.
- On config updates, the component closes the existing timer channel and creates a new one, then calls `startTimer(0)` to trigger an immediate re-check. The previous `collectCtx` is cancelled first to abort any in-flight check.
- The `checker` sub-package is kept separate from the `impl` package to make the diagnostic logic independently testable.

## Related components

| Component / Package | Relationship |
|---|---|
| [`comp/metadata/inventoryagent`](metadata/inventoryagent.md) | The connectivity checker writes results by calling `inventoryAgent.Set("diagnostics", diagnoses)`. `Set` triggers a `Refresh()` on the inventory agent if the value changed, so updated diagnostics are picked up in the next scheduled `datadog_agent` payload without waiting the full `inventories_max_interval`. The results are visible in the Datadog Infrastructure UI under the **Agent** inventory tab. |
| [`pkg/diagnose`](../pkg/diagnose/diagnose.md) | The `checker.Check` function calls `connectivity.DiagnoseInventory` and `connectivity.Diagnose` from this package to test HTTP reachability of configured Datadog intake endpoints. These are the same underlying checks that power `datadog-agent diagnose` — `connectivitychecker` re-runs them on a timer and archives the results via `inventoryagent` rather than printing to stdout. |
| [`comp/core/config`](core/config.md) | The component registers a `config.OnUpdate` hook to detect proxy or endpoint changes at runtime. When any configuration key changes, the ongoing check is cancelled and a new one starts immediately (with zero initial delay), ensuring that updated `dd_url`, `proxy.*`, or `logs_config.*` values take effect without an agent restart. |
