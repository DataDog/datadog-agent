# Runtime settings

-----

Runtime settings are the small set of configuration keys that can be read and changed on a *running* process without a restart — `agent config set log_level debug` being the canonical example. They are implemented by the `comp/core/settings` component: a per-process registry of `RuntimeSetting` objects, exposed over each process's command API as `/config/...` HTTP endpoints and through the `agent config` family of CLI commands. Writes go through the normal [layered config](overview.md) with an explicit source (`cli` or `remote-config`), so precedence is handled by the same merge rules as everything else.

## Key packages

| Path | Purpose |
| --- | --- |
| [`comp/core/settings/def/component.go`](<<<SRC>>>/comp/core/settings/def/component.go) | The `RuntimeSetting` interface, `Params`, and the component API |
| [`comp/core/settings/impl/settingsimpl.go`](<<<SRC>>>/comp/core/settings/impl/settingsimpl.go) | The registry and every HTTP handler (`GetFullConfig`, `GetValue`, `SetValue`, ...) |
| [`pkg/config/settings`](<<<SRC>>>/pkg/config/settings) | Shared `RuntimeSetting` implementations: `log_level`, profiling knobs, `log_payloads`, ... |
| [`cmd/agent/subcommands/run/internal/settings`](<<<SRC>>>/cmd/agent/subcommands/run/internal/settings) | Core-agent-only implementations: `dogstatsd_stats`, `dogstatsd_capture_duration`, MRF failover flags |
| [`pkg/config/settings/http/client.go`](<<<SRC>>>/pkg/config/settings/http/client.go) | The HTTP client used by the CLI commands |
| [`pkg/cli/subcommands/config/command.go`](<<<SRC>>>/pkg/cli/subcommands/config/command.go) | The shared `config` Cobra command (`get`, `set`, `list-runtime`, schema printing) |
| [`pkg/config/fetcher/from_processes.go`](<<<SRC>>>/pkg/config/fetcher/from_processes.go) | Helpers pulling full runtime config from other agent processes |

## The registry

A `RuntimeSetting` is a tiny interface ([`comp/core/settings/def/component.go`](<<<SRC>>>/comp/core/settings/def/component.go)):

```go
type RuntimeSetting interface {
    Get(config config.Component) (interface{}, error)
    Set(config config.Component, v interface{}, source model.Source) error
    Description() string
    Hidden() bool
}
```

Each binary declares its own registry by providing `settings.Params{Settings: map[string]RuntimeSetting{...}, Config: ...}` in its run command — see the core agent's map in [`cmd/agent/subcommands/run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go). Because the registry is per-process, the same setting name (for example `log_level`) is registered independently by the core agent, process-agent, security-agent, system-probe, and cluster-agent — and changing it on one process does **not** change it on the others. The trace-agent does not use the component at all: its debug server exposes a bespoke `POST /config/set?log_level=...` handler ([`comp/trace/agent/impl/run.go`](<<<SRC>>>/comp/trace/agent/impl/run.go)), and the standalone dogstatsd binary has no runtime-settings registry.

`Set` implementations receive the `model.Source` and must pass it through to `config.Set()`; that is what makes the layered precedence work. Most implementations are thin wrappers that validate or coerce the value (helpers `GetBool`/`GetInt` in [`pkg/config/settings`](<<<SRC>>>/pkg/config/settings) accept string forms like `"true"`) and then write the mapped config key. Some have side effects beyond the config write — `log_level` reconfigures the logger, the profiling settings start and stop the internal profiler.

Commonly registered settings:

| Setting | Scope | Effect |
| --- | --- | --- |
| `log_level` | all processes | Changes the logger level and the `log_level` config key |
| `internal_profiling` (+ `internal_profiling_goroutines`, `internal_profiling_period`) | all | Starts/stops continuous internal profiling |
| `runtime_mutex_profile_fraction`, `runtime_block_profile_rate` | core agent, cluster-agent, process-agent, system-probe | Go runtime profiling rates |
| `log_payloads` | core agent | Log every payload sent to the intake (very verbose) |
| `dogstatsd_stats`, `dogstatsd_capture_duration` | core agent | DogStatsD debug stats and traffic capture |
| `multi_region_failover.failover_metrics` / `failover_logs` | core agent | MRF failover flags, driven by remote config |

## HTTP endpoints

The component returns `api.AgentEndpointProvider` values, which the API server mounts under `/agent` on the core agent's command API (`cmd_host:cmd_port`, default `localhost:5001`). Other binaries mount the same handlers on their own command servers — the security-agent under `/agent`; the cluster-agent, process-agent, and system-probe at the root. The trace-agent instead serves its own `/config` and `/config/set` handlers on its debug server.

| Endpoint | Method | Returns |
| --- | --- | --- |
| `/config` | GET | Full runtime config as scrubbed YAML (`AllSettings`) |
| `/config/without-defaults` | GET | Only non-default settings, scrubbed |
| `/config/list-runtime` | GET | JSON map of registered settings with description and hidden flag |
| `/config/{setting}` | GET | `{"value": ...}`; add `?sources=true` for the value in every config layer |
| `/config/{setting}` | POST | Sets the value; the form field `value` carries the new value |
| `/config/by-source` | GET | Per-layer JSON dump (registered separately by [cluster-agent](<<<SRC>>>/cmd/cluster-agent/api/agent/agent.go), [security-agent](<<<SRC>>>/cmd/security-agent/api/agent/agent.go), and [system-probe](<<<SRC>>>/cmd/system-probe/api/config.go)) |

All full-config responses pass through [`pkg/util/scrubber`](<<<SRC>>>/pkg/util/scrubber) before leaving the process, and the by-source dump scrubs each layer separately.

Per-process addresses used by the CLI and the [flare](../operations/flare.md) fetchers ([`pkg/config/fetcher/from_processes.go`](<<<SRC>>>/pkg/config/fetcher/from_processes.go)):

| Process | Address | Path |
| --- | --- | --- |
| core agent | `https://cmd_host:cmd_port` (localhost:5001) | `/agent/config...` |
| security-agent | `https://localhost:{security_agent.cmd_port}` (5010) | `/agent/config...` |
| trace-agent | `https://127.0.0.1:{apm_config.debug.port}` (5012) | `/config` |
| process-agent | `https://{ipc_address}:{process_config.cmd_port}` (6162) | `/config`, `/config/all` |
| system-probe | unix socket `{system_probe_config.sysprobe_socket}` / named pipe `\\.\pipe\dd_system_probe` | `/config...` |

Every HTTP channel except system-probe's is authenticated with the IPC auth token and TLS; transport details are in [Inter-process communication](../processes/ipc.md).

## CLI

The shared Cobra command in [`pkg/cli/subcommands/config/command.go`](<<<SRC>>>/pkg/cli/subcommands/config/command.go) is embedded in each binary:

1. `agent config` — print the running process's non-default runtime configuration (`-a`/`--all` includes defaults).
1. `agent config list-runtime` — list the settings that can be changed at runtime, with descriptions.
1. `agent config get <setting>` — read a value; `-s`/`--source` prints the value in every layer.
1. `agent config set <setting> <value>` — change a value on the running process.
1. `agent config print-agent-schema` / `print-system-probe-schema` — dump the embedded YAML schemas (see [the overview](overview.md#declaration-and-schema-sealing)).

Equivalent subcommands exist on the other binaries (`datadog-cluster-agent config ...`, `security-agent config ...`, `system-probe config ...`), each talking to its own process's endpoints.

## Source priorities: CLI vs remote config

`SetValue` over HTTP — and therefore `agent config set` — always writes at `SourceCLI` ([`settingsimpl.go`](<<<SRC>>>/comp/core/settings/impl/settingsimpl.go)), the highest-priority source (11). [Remote configuration](remote-config.md) writes through the same component at `SourceRC` (10) via its `AGENT_CONFIG` product callback ([`comp/remote-config/rcclient/impl/rcclient.go`](<<<SRC>>>/comp/remote-config/rcclient/impl/rcclient.go)). The interplay is deliberate:

1. If `log_level`'s current source is `cli`, the remote-config callback logs `Remote config could not change the log level due to CLI override` and refuses — a human at the box beats the backend.
1. When a remote-config override is withdrawn (empty RC payload), the callback calls `UnsetForSource("log_level", SourceRC)`, and the config falls back to the value from the next-lower layer (env var, file, or default) automatically.
1. There is no "unset" for the CLI layer over the API — a CLI-set value sticks until the process restarts.

Runtime settings are process-local and ephemeral: nothing is persisted to disk, and every value set at `SourceCLI` or `SourceRC` disappears on restart, when the config is rebuilt from defaults, file, env, and fleet policies.

## Adding a runtime setting

The pattern, end to end (the [`create-runtime-setting` skill](<<<SRC>>>/.claude/skills/create-runtime-setting/SKILL.md) automates it):

1. The config key must already be declared (`BindEnvAndSetDefault`) — see [the config how-to](https://datadoghq.dev/datadog-agent/how-to/go/config/).
1. Create `runtime_setting_<feature>.go` implementing the four-method interface, in [`pkg/config/settings`](<<<SRC>>>/pkg/config/settings) if shared across binaries or in [`cmd/agent/subcommands/run/internal/settings`](<<<SRC>>>/cmd/agent/subcommands/run/internal/settings) if core-agent-only. In `Set`, validate/coerce the input and call `config.Set(key, value, source)`, passing the source through unchanged.
1. Register it in the `settings.Params` map of every binary that should expose it (each binary's run `command.go`).
1. Test `Get`, `Set` with typed and string inputs, and the error path; run `dda inv test --targets=<package>`.

## Gotchas

1. **Per-process, not fleet-wide.** `agent config set log_level debug` changes only the core agent; the trace-agent keeps its own level. Tooling that needs "everything" must call each process (which is what `agent flare` does via the fetcher helpers).
1. **Hidden settings still work.** `Hidden()` only removes a setting from `list-runtime` output; `agent config set` prints a warning that changing a hidden option "may incur billing charges or have other unexpected side-effects" but proceeds.
1. **Values arrive as strings.** The HTTP `SetValue` handler passes the form value as a string; implementations must coerce (`GetBool`/`GetInt`) or reject. The underlying `config.Set` additionally converts to the declared default's type.
1. **A CLI write blocks RC forever (until restart).** Because `SourceCLI` outranks `SourceRC` and cannot be unset remotely, a debugging session that ends with `agent config set log_level debug` leaves the fleet's remote-config log-level management inert on that host until the Agent restarts.
1. **`/config` on the settings API is the full config, not just runtime settings.** The GET endpoints dump the entire (scrubbed) runtime configuration; only `/config/list-runtime` and the `{setting}` endpoints are scoped to registered runtime settings.
