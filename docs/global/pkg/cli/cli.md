# pkg/cli

## Purpose

`pkg/cli` contains reusable building blocks for the Datadog Agent's CLI binaries. It is split into two sub-packages:

- **`pkg/cli/standalone`** — utilities for CLI commands that boot a full in-process Agent runtime (e.g. `agent check`, `agent jmx`), as opposed to commands that simply talk to an already-running agent over IPC.
- **`pkg/cli/subcommands/`** — a collection of ready-made Cobra subcommands (one package per command) that multiple agent binaries can share.

## Key elements

### `pkg/cli/standalone`

| Symbol | Description |
|--------|-------------|
| `PrintWindowsUserWarning(op string)` | Prints a console warning that the command is running in a different user context than the Windows service — relevant when a command result depends on permissions. |
| `ExecJMXCommandConsole(...)` | *(build tag `jmx`)* Runs a JMX command and reports results via `log.Info` (console reporter). |
| `ExecJmxListWithMetricsJSON(...)` | *(build tag `jmx`)* Runs `list_with_metrics` against JMXFetch and prints the result as JSON. Used by `agent check <jmx-check>`. |
| `ExecJmxListWithRateMetricsJSON(...)` | *(build tag `jmx`)* Variant of the above for rate metrics (`--check-rate`). |

All three JMX helpers require that AutoConfig has already been initialised before they are called. They create a `jmxfetch.JMXFetch` runner internally, load the relevant integration configs, and block until JMXFetch exits.

The no-JMX stub (`jmx_nojmx.go`) provides empty implementations when the `jmx` build tag is absent so callers never need to guard the call site.

### `pkg/cli/subcommands/`

Each sub-package exposes a single `MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command` factory (or a variant of that signature). The agent binary passes a `globalParamsGetter` closure that returns the config-file paths and logger name known at the time Cobra executes the command — not at the time `MakeCommand` is called.

| Package | Command | What it does |
|---------|---------|--------------|
| `check` | `check <name>` | Runs a check in-process with a full agent stack (AutoDiscovery, demultiplexer, etc.). Supports `--check-rate`, `--check-times`, `--json`, `--table`, memory profiling, breakpoints, flare output. JMX checks are automatically delegated to `standalone.ExecJmx*`. |
| `config` | `config` | Queries / sets configuration on a running agent via IPC. |
| `health` | `health` | Calls the running agent's `/agent/status/health` endpoint and renders component health. |
| `version` | `version` | Prints the agent version, commit, payload version, and Go version. |
| `taggerlist` | `tagger-list` | Fetches and prints the tagger contents of a running agent. |
| `workloadlist` | `workloadlist` | Prints the workload-metadata store contents of a running agent. |
| `workloadfilter` | `workload-filter` | Validates workload-filter configuration (CEL rules). Two build-tag variants: `verify_config_cel.go` (with CEL support) and `verify_config_nocel.go`. |
| `clusterchecks` | `clusterchecks` | Lists cluster checks from the cluster agent. |
| `dcaflare` | `flare` (DCA) | Generates a flare from the cluster agent. |
| `dcaconfigcheck` | `configcheck` (DCA) | Runs a config check against the cluster agent. |
| `autoscalerlist` | `autoscaler-list` | Lists autoscalers from the cluster agent. |
| `processchecks` | `check` (process-agent) | Runs process-agent checks. |

#### `GlobalParams` (defined per-package)

Every subcommand package defines its own `GlobalParams` struct. The fields are a subset of:

```go
type GlobalParams struct {
    ConfFilePath         string
    ExtraConfFilePaths   []string
    SysProbeConfFilePath string
    FleetPoliciesDirPath string
    ConfigName           string
    LoggerName           string
}
```

The struct is populated by the parent command before Cobra dispatches to the subcommand's `RunE`.

## Usage

### Adding a shared subcommand to an agent binary

```go
import "github.com/DataDog/datadog-agent/pkg/cli/subcommands/health"

rootCmd.AddCommand(health.MakeCommand(func() health.GlobalParams {
    return health.GlobalParams{
        ConfFilePath: globalParams.ConfFilePath,
        ConfigName:   "datadog",
        LoggerName:   "agent",
    }
}))
```

### Running a JMX check from a CLI command

```go
import "github.com/DataDog/datadog-agent/pkg/cli/standalone"

// (AutoConfig must already be initialised)
err := standalone.ExecJmxListWithMetricsJSON(
    selectedChecks, logLevel, allConfigs, agentAPI, jmxLogger, ipcComp,
)
```

### In-process `check` command behaviour

`subcommands/check` spins up a minimal agent stack via `fxutil.OneShot`. It disables `cmd_port` (sets `DD_CMD_PORT=0`) so it does not collide with a running agent, boots AutoDiscovery with a configurable timeout, runs each matched check instance the requested number of times, and prints the aggregator output. The `--flare` flag scrubs and writes the output to the check-flare directory.

### Where subcommands are consumed

The subcommand factories are used by multiple binaries:

- `cmd/agent/` — main agent binary (check, health, config, tagger-list, workload-list, workload-filter, version, …)
- `cmd/cluster-agent/` — cluster agent (dca-flare, dca-configcheck, autoscaler-list, …)
- `cmd/process-agent/` — process-agent (check, version, …)
- `cmd/trace-agent/`, `cmd/system-probe/`, `cmd/otel-agent/`, `cmd/host-profiler/` — version subcommand

---

## Related packages and components

- **`pkg/collector`** — `subcommands/check` calls `pkgcollector.GetChecksByNameForConfigs` to resolve check instances and then calls `check.Run()` directly (without going through the collector component). The `collector.Component` is injected as `option.Option[collector.Component]` into the `check` run function — when present it is used for lifecycle management, otherwise checks are run fully in-process. See [pkg/collector docs](../collector/collector.md).
- **`comp/core/config`** — every subcommand `GlobalParams` struct carries a `ConfFilePath` / `ExtraConfFilePaths` that is passed to `config.NewAgentParams` inside the `fxutil.OneShot` app. Config is a mandatory dependency of every subcommand. See [comp/core/config docs](../../comp/core/config.md).
- **`pkg/util/fxutil`** — all subcommands are built with `fxutil.OneShot`, which starts the full fx app, calls the subcommand function, then shuts down cleanly. The `check` subcommand sets `DD_CMD_PORT=0` before calling `OneShot` so it does not collide with a running agent. Tests use `fxutil.TestOneShotSubcommand` to verify the command's dependency graph without executing the action. See [pkg/util/fxutil docs](../util/fxutil.md).
