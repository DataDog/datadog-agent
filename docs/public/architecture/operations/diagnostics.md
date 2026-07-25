# Diagnostics and CLI tools

-----

Almost every `agent` subcommand other than `run` is a short-lived program that asks the running daemon a question and renders the answer. This page explains that pattern once — one-shot Fx apps talking HTTPS to the CMD API server — then tours the diagnose suites, the introspection commands (`check`, `stream-logs`, `tagger-list`, `workload-list`, `config`, `remote-config`, ...), runtime `log_level` changes, and the web GUI. The [status, health, and telemetry systems](introspection.md) these commands render, and the [flare](flare.md) they can trigger, have their own pages.

## Key packages and files

| Path | Purpose |
|---|---|
| [`cmd/agent/subcommands/subcommands.go`](<<<SRC>>>/cmd/agent/subcommands/subcommands.go) | Master list of `agent` CLI subcommands |
| [`comp/core/ipc/def/component.go`](<<<SRC>>>/comp/core/ipc/def/component.go) | Auth token and IPC certificate; the `HTTPClient` every CLI command uses |
| [`comp/api/api/apiimpl/server_cmd.go`](<<<SRC>>>/comp/api/api/apiimpl/server_cmd.go) | The CMD API server on `https://localhost:5001` that serves the CLI |
| [`comp/core/diagnose/def/component.go`](<<<SRC>>>/comp/core/diagnose/def/component.go) | `Diagnosis` type, status constants, the suite `Catalog`, suite name constants |
| [`comp/core/diagnose/impl/diagnose.go`](<<<SRC>>>/comp/core/diagnose/impl/diagnose.go) | Runs suites with include/exclude filters; `POST /agent/diagnose`; `diagnose.log` flare provider |
| [`comp/core/diagnose/local/local.go`](<<<SRC>>>/comp/core/diagnose/local/local.go) | Assembles the diagnose suites inside the CLI process for `--local` runs |
| [`pkg/diagnose/connectivity/core_endpoint.go`](<<<SRC>>>/pkg/diagnose/connectivity/core_endpoint.go) | Real HTTP checks against every configured intake endpoint |
| [`pkg/diagnose/ports/ports.go`](<<<SRC>>>/pkg/diagnose/ports/ports.go), [`pkg/diagnose/firewallscanner`](<<<SRC>>>/pkg/diagnose/firewallscanner) | Port-conflict detection; Windows firewall rule scan |
| [`cmd/agent/subcommands/diagnose/command.go`](<<<SRC>>>/cmd/agent/subcommands/diagnose/command.go) | `agent diagnose` and `agent diagnose show-metadata` |
| [`pkg/cli/subcommands/check/command.go`](<<<SRC>>>/pkg/cli/subcommands/check/command.go) | `agent check`: runs a check locally with a full AD/tagger/workloadmeta stack |
| [`pkg/cli/subcommands/config/command.go`](<<<SRC>>>/pkg/cli/subcommands/config/command.go) | Shared `config` subcommand (get/set/list-runtime), reused by several binaries |
| [`comp/core/settings/impl/settingsimpl.go`](<<<SRC>>>/comp/core/settings/impl/settingsimpl.go) | Runtime settings registry and its HTTP handlers |
| [`comp/core/gui/impl/gui.go`](<<<SRC>>>/comp/core/gui/impl/gui.go), [`agent.go`](<<<SRC>>>/comp/core/gui/impl/agent.go), [`checks.go`](<<<SRC>>>/comp/core/gui/impl/checks.go) | Web GUI server: intent-token auth, status/flare/log/config/check endpoints |

## How CLI commands work

Every one-shot subcommand is a miniature Fx application built with `fxutil.OneShot`, wiring only the components it needs — typically config, logging in a quiet mode, and the [`ipc` component](<<<SRC>>>/comp/core/ipc/def/component.go) in `ModuleReadOnly` mode (the artifacts must already exist; only the daemon may create them). The command then acts as an HTTPS client of the CMD API server on `https://localhost:5001` (`cmd_host`/`cmd_port`): `ipc.HTTPClient` injects the bearer auth token and validates the server against the IPC certificate. When the daemon is not running, commands fail fast with an "is the Agent running?" style error rather than hanging. The transport, token, and certificate mechanics are covered in [Inter-process communication](../processes/ipc.md).

This split matters when interpreting output: the command renders *the daemon's* state, fetched over the wire, except for the handful of commands that deliberately run in the CLI process (`agent check`, `agent diagnose --local`, local [flares](flare.md)). Those see the CLI's environment — user, env vars, secrets access — which may differ from the daemon's.

## `agent diagnose`

The diagnose system runs suites of targeted checks and reports each result as a `Diagnosis` with a status (`PASS`, `FAIL`, `WARNING`, `UNEXPECTED ERROR`), a name, a category, an explanation, and a remediation hint ([`def/component.go`](<<<SRC>>>/comp/core/diagnose/def/component.go)).

### The catalog and suites

Suites live in a process-global `Catalog` mapping fixed suite names to `func(Config) []Diagnosis`. The names are constants, and registering an unknown name panics at startup:

| Suite | What it checks |
|---|---|
| `check-datadog` | Every scheduled check instance, via `collector.Diagnose` on the live [collector](../checks/collector.md); Python checks contribute their own diagnoses |
| `connectivity-datadog-core-endpoints` | Real HTTP requests to every configured metrics/intake endpoint |
| `connectivity-datadog-autodiscovery` | Connectivity for autodiscovery-related endpoints |
| `connectivity-datadog-event-platform` | [Event platform](../pipelines/event-platform.md) endpoint connectivity |
| `port-conflict` | Processes already bound to ports the Agent needs |
| `firewall-scan` | Firewall rules that would block Agent traffic (effectively Windows-oriented) |
| `health-issues` | Health-platform issues; returns no diagnoses when `health_platform.enabled` is false |

The core agent registers all of these in [`run/command.go`](<<<SRC>>>/cmd/agent/subcommands/run/command.go); the [Cluster Agent](../containers/cluster-agent.md) registers only the connectivity suites ([`cmd/cluster-agent/subcommands/diagnose/command.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/diagnose/command.go)).

### Execution: in-agent by default, local on demand

[`impl/diagnose.go`](<<<SRC>>>/comp/core/diagnose/impl/diagnose.go) sorts suites by name, applies the `--include`/`--exclude` regex filters (matched against suite name, check name, and category), validates each diagnosis, and renders text or JSON. `agent diagnose` runs the suites **inside the daemon** via `POST /agent/diagnose` (the handler resets the connection deadline because suites can outlive `server_timeout`), so results reflect the daemon's environment. `--local` instead builds a self-contained autodiscovery/tagger/workloadmeta stack in the CLI process ([`local/local.go`](<<<SRC>>>/comp/core/diagnose/local/local.go)) — useful when the daemon is down or when you suspect a privilege difference. Local [flares](flare.md) use the same local assembly automatically. The component is also a flare provider, writing `diagnose.log` into every remote flare.

The connectivity suite ([`core_endpoint.go`](<<<SRC>>>/pkg/diagnose/connectivity/core_endpoint.go)) resolves every configured domain and API key with `utils.GetMultipleEndpoints` — the same multi-endpoint logic the [forwarder](../pipelines/forwarder.md) uses, so what it tests is what production traffic does — and fires real HTTP requests tagged with a `datadog-agent-diagnose` header. Logs endpoints are checked over HTTP or TCP depending on `logs_config.force_use_tcp`. Errors are scrubbed of API keys before display.

### `agent diagnose show-metadata`

`agent diagnose show-metadata <payload>` prints the exact JSON the Agent would send to a metadata intake: `v5`, `gohai`, `inventory-agent`, `inventory-host`, `inventory-checks`, `host-gpu`, `host-system-info`, `ha-agent`, `package-signing`, `system-probe`, `security-agent`, `agent-telemetry`, and `agent-full-telemetry` (plus `health-issues`, which prints health-platform issues rather than a metadata payload). Each is fetched from the corresponding `GET /agent/metadata/...` endpoint provided by the [`comp/metadata`](<<<SRC>>>/comp/metadata) inventory components — `show-metadata agent-telemetry` is the standard way to audit what [agent telemetry](introspection.md#agent-telemetry) ships.

/// warning
The diagnosis status constants are ABI-shared with Python: the numeric values must stay in sync with `rtloader/include/rtloader_types.h` and `diagnose.py` in integrations-core. Renumbering them breaks Python check diagnoses silently.
///

## CLI tour

The table lists the introspection-relevant subcommands of the core `agent` binary ([`subcommands.go`](<<<SRC>>>/cmd/agent/subcommands/subcommands.go)); "endpoint" is the CMD API route the command calls, and *(local)* marks commands that do their work in the CLI process.

| Command | Endpoint / behavior |
|---|---|
| `agent status [section] [--list] [-j] [-v]` | `GET /agent/status[...]`; rendered and scrubbed client-side |
| `agent check <name> [--check-rate] [--check-times N] [--json] [--profile-memory] [--breakpoint]` | *(local)* runs the check in the CLI process with a no-op forwarder and prints what *would* be sent ([`pkg/cli/subcommands/check`](<<<SRC>>>/pkg/cli/subcommands/check/command.go)) |
| `agent configcheck` | `GET /agent/config-check` — [autodiscovery](../checks/autodiscovery.md)-resolved check configs, scrubbed |
| `agent config` / `list-runtime` / `get\|set <setting>` | settings endpoints (see below) |
| `agent diagnose [--list] [--include RE] [--exclude RE] [--local] [--json]` | `POST /agent/diagnose`, or *(local)* with `--local` |
| `agent flare [caseID]` | `POST /agent/flare`, falling back to *(local)*; see [Flare](flare.md) |
| `agent health` | `GET /agent/status/health` — the readiness set, exit non-zero when unhealthy |
| `agent hostname` | resolved hostname from the daemon |
| `agent launch-gui` | `GET /agent/gui/intent`, then opens the browser (below) |
| `agent stream-logs [--name] [--type] [--source] [--service] [-o file] [-d dur]` | `POST /agent/stream-logs` — chunked live stream from the [logs agent's](../pipelines/logs.md) diagnostic receiver ([`comp/logs/agent/impl/agent.go`](<<<SRC>>>/comp/logs/agent/impl/agent.go)) |
| `agent stream-event-platform [--type]` | `POST /agent/stream-event-platform` — live [event platform](../pipelines/event-platform.md) payloads |
| `agent tagger-list` | `GET /agent/tagger-list` — every entity's tags per collector (see [Tagger](../containers/tagger.md)) |
| `agent workload-list [-v]` | `GET /agent/workload-list` — the [workloadmeta](../containers/workloadmeta.md) store dump |
| `agent workload-filter-list` | workload filtering rules as evaluated by the daemon |
| `agent remote-config [state]` | [remote configuration](../configuration/remote-config.md) client/repo state |
| `agent secret` | secret backend status and scrubbed secret usage (see [Secrets](../configuration/secrets.md)) |
| `agent dogstatsd-stats` / `dogstatsd-capture` / `dogstatsd-replay` | DogStatsD stats and traffic capture/replay (see [DogStatsD internals](../dogstatsd/internals.md)) |
| `agent jmx list\|collect ...` | *(local)* drives [JMXFetch](../checks/jmx.md) directly |
| `agent stop` | `POST /agent/stop` — graceful shutdown |
| `agent controlsvc start-service\|stop-service\|restart-service` | Windows service control ([`controlsvc`](<<<SRC>>>/cmd/agent/subcommands/controlsvc/command.go)) |

Other binaries mirror subsets of this surface against their own API ports: `datadog-cluster-agent {status,flare,diagnose,config,check,clusterchecks,metamap,...}` (port 5005), `process-agent {status,check,config,taggerlist,workloadlist}` (6162), `security-agent {status,flare,config,compliance,workloadlist}` (5010), `trace-agent {info,config}`. The shared implementations live under [`pkg/cli/subcommands`](<<<SRC>>>/pkg/cli/subcommands/config/command.go) so the behavior is identical across binaries. See [Binaries and flavors](../processes/binaries.md) for what each process is.

/// note
`agent check` and `agent diagnose` have **opposite defaults**: `check` always runs in the CLI process (results can differ from the daemon — permissions, env vars, secrets access), while `diagnose` runs in the daemon unless `--local` is passed.
///

## Runtime configuration changes

`agent config` talks to the [settings component](<<<SRC>>>/comp/core/settings/impl/settingsimpl.go), which exposes the daemon's effective configuration and a registry of named runtime-mutable settings over five CMD API endpoints (full scrubbed config, config without defaults, list, get, set). The command surface: `agent config` prints the running daemon's effective configuration (scrubbed; `-a` includes defaults), `agent config list-runtime` lists mutable settings, `agent config get|set <setting> [value]` reads or writes one, and `agent config get --source` shows which layer a value came from (default, file, env var, remote config, or CLI). The `RuntimeSetting` interface, the per-binary catalogs, and how to add a setting are covered in [Runtime settings](../configuration/runtime-settings.md).

The setting you will use most is `log_level` ([`runtime_setting_log_level.go`](<<<SRC>>>/pkg/config/settings/runtime_setting_log_level.go)): `agent config set log_level debug` flips the live seelog logger immediately, without restart. Two operational notes: runtime changes are **not persisted** — they apply to the running process with source `CLI` and revert on restart — and a [flare](flare.md) taken afterwards records the elevated level in its filename. `log_level` can also be set from the Datadog backend through remote config (source `remote-config`). Settings whose `Hidden()` method returns true print a warning when set, because they carry billing or side-effect risk.

## The web GUI

The Agent Manager GUI ([`comp/core/gui`](<<<SRC>>>/comp/core/gui/impl/gui.go)) is a local web app served on `http://localhost:<GUI_port>` — default port 5002 on Windows and macOS, `-1` (disabled) on Linux. It binds loopback only (`system.IsLocalAddress` is enforced) and is plain HTTP, so its security rests entirely on its token scheme:

1. `agent launch-gui` ([`launchgui`](<<<SRC>>>/cmd/agent/subcommands/launchgui/command.go)) calls the authenticated CMD endpoint `GET /agent/gui/intent`, which mints a single-use, 32-byte **intent token**.
1. The CLI opens `http://localhost:5002/auth?intent=<token>` in the browser. The GUI server deletes the intent token and issues an HMAC-signed **access token** cookie (`accessToken`, derived from the Agent auth token) whose lifetime is `GUI_session_expiration` (default 0 = no expiry).
1. All subsequent `/agent/*` and `/checks/*` GUI routes pass through `authMiddleware`, which checks the cookie plus an `Origin` check on non-GET requests.

You cannot simply browse to port 5002 and log in — the intent token is only obtainable through the authenticated API, so `agent launch-gui` (or the Windows tray icon, "Datadog Agent Manager") is the sole entry point. Capabilities ([`agent.go`](<<<SRC>>>/comp/core/gui/impl/agent.go), [`checks.go`](<<<SRC>>>/comp/core/gui/impl/checks.go)): render the HTML status page, tail and flip the ordering of `agent.log`, view and **edit** `datadog.yaml` and `conf.d` check config files (writes go to the real config directory), list/run checks once, build and submit a flare, and restart the Agent — the restart button is implemented on Windows ([`platform_windows.go`](<<<SRC>>>/comp/core/gui/impl/platform_windows.go)) and macOS via launchctl ([`platform_darwin.go`](<<<SRC>>>/comp/core/gui/impl/platform_darwin.go)), while Linux returns "not implemented" ([`platform_nix.go`](<<<SRC>>>/comp/core/gui/impl/platform_nix.go)).

## Configuration

| Key | Default | Effect |
|---|---|---|
| `cmd_host` / `cmd_port` | `localhost` / 5001 | CMD API server every CLI command targets |
| `server_timeout` | 30 s | CMD API connection deadline; flare and diagnose handlers reset it |
| `GUI_host` / `GUI_port` | `localhost` / 5002 (Windows/macOS), `-1` (Linux) | Web GUI bind; `-1` disables |
| `GUI_session_expiration` | 0 (never) | GUI access-token TTL |
| `health_platform.enabled` | `true` | Gates the `health-issues` diagnose suite |
| `logs_config.force_use_tcp` | `false` | Switches the logs connectivity diagnosis to TCP |

## Gotchas

1. `agent check` results are not gospel for daemon behavior: it runs in your shell's environment. A check that passes under `agent check` as root can still fail in the daemon running as `dd-agent`, and vice versa.
1. Registering a diagnose suite under a name missing from the fixed catalog list panics at startup — the suite names are a closed set by design.
1. Runtime settings changed with `agent config set` silently revert on restart; if a debug session ends with a restart, the log level is back to `info` even though nobody set it back.
1. `agent health` and the Kubernetes probes evaluate different catalogs (readiness union vs liveness) over different transports (authenticated CMD API vs the unauthenticated probe server) — one failing does not imply the other does. See [Health checks](introspection.md#health-checks-and-kubernetes-probes).
1. The GUI can edit and save `datadog.yaml`, but on Linux it cannot restart the Agent to apply the change — the edit quietly waits for the next manual restart.
1. Local diagnose runs in flares are capped at 60 s and deliberately leak the worker goroutine on timeout, because Python/CGo initialization in the CLI process can hang indefinitely.
