# pkg/util/flavor

## Purpose

`pkg/util/flavor` identifies which agent binary is running. The Datadog Agent ships as many
separate binaries (core agent, cluster agent, process agent, trace agent, etc.). Some
shared packages need to adjust their behaviour based on which binary they are running inside
— for example, disabling metrics that make no sense for a given binary or enabling
IoT-specific configuration.

The package exposes a single global string variable (the *flavor*) that each binary's `main`
sets at startup with `SetFlavor`, and that shared packages read at runtime with `GetFlavor`.

The package has **no build constraints** and is safe to import on all platforms. It is a
separate Go module (`go.mod` present) to keep its dependency footprint minimal.

---

## Key elements

### Flavor constants

```go
const (
    DefaultAgent        = "agent"
    IotAgent            = "iot_agent"
    ClusterAgent        = "cluster_agent"
    Dogstatsd           = "dogstatsd"
    SecurityAgent       = "security_agent"
    ServerlessAgent     = "serverless_agent"
    HerokuAgent         = "heroku_agent"
    ProcessAgent        = "process_agent"
    TraceAgent          = "trace_agent"
    OTelAgent           = "otel_agent"
    SystemProbe         = "system_probe"
    HostProfiler        = "host_profiler"
    PrivateActionRunner = "private_action_runner"
)
```

These string values are stable identifiers. They appear in log lines, status pages, and
metric tags.

### Functions

```go
// SetFlavor sets the agent flavor. Call once in main, before any goroutine reads GetFlavor.
// Setting IotAgent also sets the iot_host config key to true.
func SetFlavor(flavor string)

// GetFlavor returns the current flavor string.
// Must NOT be called from init() functions or package-level variable initialisers
// because it reads a package-level variable that main has not yet set.
func GetFlavor() string

// GetHumanReadableFlavor returns a display name (e.g. "Cluster Agent", "IoT Agent").
// Returns "Unknown Agent" for unrecognised flavor strings.
func GetHumanReadableFlavor() string
```

**Side effect of `SetFlavor(IotAgent)`:** sets the `iot_host` config key to `true` via
`pkgconfigsetup.Datadog().Set(...)`. This activates IoT-specific logic in packages that
check that key.

### Test helpers (`flavor_test_util.go`)

`SetFlavor` is also exported for use in tests. `flavor_test_util.go` provides nothing
beyond the main file for now, but the file exists as an extension point for test utilities
that set/restore the flavor without leaking state across tests.

---

## Usage

### Setting the flavor at binary startup

Each agent binary calls `SetFlavor` once at the top of `main`, before the component system
initialises:

```go
// cmd/cluster-agent/main.go
flavor.SetFlavor(flavor.ClusterAgent)

// cmd/process-agent/main.go
flavor.SetFlavor(flavor.ProcessAgent)

// cmd/dogstatsd/main_nix.go
flavor.SetFlavor(flavor.Dogstatsd)
```

The default value (before any `SetFlavor` call) is `DefaultAgent`, so the core agent binary
does not need to call `SetFlavor` explicitly, though some entry points do for clarity.

### Reading the flavor in shared packages

Packages that need flavor-conditional logic call `GetFlavor()` at runtime, **never** at
init time or in package-level variable declarations:

```go
// pkg/collector/corechecks/system/disk/diskv2/disk.go
// Skip loading check if the binary is not the standard agent:
if flavor.GetFlavor() == flavor.DefaultAgent && !cfg.GetBool("use_diskv2_check") {
    return
}

// pkg/collector/corechecks/cluster/ksm/kubernetes_state.go
// KSM check runs on node agents and CLC runners, not on the cluster agent:
isRunningOnNodeAgent: flavor.GetFlavor() != flavor.ClusterAgent && !pkgconfigsetup.IsCLCRunner(cfg),

// pkg/logs/metrics/metrics.go
// Tag metrics with the binary name for observability:
tags = append(tags, "agent_flavor:"+flavor.GetHumanReadableFlavor())
```

### Testing flavor-dependent code

Tests that exercise flavor-conditional branches should call `SetFlavor` at the start and
restore `DefaultAgent` afterwards (or use `t.Cleanup`):

```go
func TestClusterAgentBehaviour(t *testing.T) {
    flavor.SetFlavor(flavor.ClusterAgent)
    t.Cleanup(func() { flavor.SetFlavor(flavor.DefaultAgent) })
    // ...
}
```

---

## Important caveats

- **Do not call `GetFlavor` in `init()` or in package-level `var` initialisers.** The flavor
  variable is set in `main`, which runs after all `init()` functions. Calling `GetFlavor`
  before `main` always returns `DefaultAgent`, which may silently produce wrong behaviour.
- `SetFlavor` is **not goroutine-safe** after startup. It is intended to be called exactly
  once at startup, before any concurrent code reads the flavor.
- Packages that gate expensive initialization on the flavor should do so lazily (e.g. inside
  the first call to a public function), not at import time.

---

## Related packages

| Package / component | Relationship |
|---------------------|--------------|
| [`pkg/config/setup`](../../pkg/config/setup.md) | `SetFlavor(IotAgent)` calls `pkgconfigsetup.Datadog().Set("iot_host", true)` as a side effect, activating IoT-specific config paths. The setup package also reads the flavor-driven `iot_host` key when computing default collection intervals and enabling IoT-specific checks. See the [setup docs](../../pkg/config/setup.md) for the full configuration loading sequence. |
| [`comp/core/log`](../../comp/core/log.md) | The agent status header provider (`comp/core/status/statusimpl/common_header_provider.go`) calls `GetHumanReadableFlavor()` to construct the display name shown on the status page (e.g. `"Cluster Agent (v7.x.y)"`). Remote-agent helpers (`comp/core/remoteagent/impl-*/`) also pass both `GetFlavor()` and `GetHumanReadableFlavor()` when registering themselves with the agent registry. |
| [`pkg/util/fxutil`](../util/fxutil.md) | Each agent binary wires its fx application via `fxutil.Run` or `fxutil.OneShot`. `SetFlavor` must be called **before** `fxutil.Run` so that components resolved during fx graph construction (e.g. the log component's `Params`) can read the correct flavor. The call order is: `SetFlavor` → `fxutil.Run` → fx lifecycle hooks. |
| `cmd/*/main.go` | Each binary calls `SetFlavor` exactly once at process start, before the fx application is constructed. The `DefaultAgent` flavor is pre-set, so the core agent binary does not need an explicit call, but most binaries call it anyway for clarity. |
