> **TL;DR:** `pkg/procmgr` contains a Rust-based process manager daemon (`dd-procmgrd`) that supervises Datadog sub-components declared via YAML configs, providing systemd-like lifecycle management with dependency ordering, restart policies, and gRPC control.

# pkg/procmgr

## Purpose

`pkg/procmgr` contains a Rust implementation of a **process manager daemon** (`dd-procmgrd`) that tracks and supervises a set of child processes declared via YAML configuration files. It is designed to be a lightweight, systemd-like supervisor for Datadog sub-components that need to run as separate OS processes alongside the main agent (for example, the DDOT OpenTelemetry collector).

A companion CLI binary (`dd-procmgr`) communicates with the daemon over gRPC.

The Go portion of the repository only holds the proto definition; all executable code is Rust, located in `pkg/procmgr/rust/`.

## Key elements

### Key types

#### Rust crate layout

| Path | Description |
|------|-------------|
| `src/bins/dd-procmgrd.rs` | Daemon entry point. Loads config, constructs `ProcessManager`, calls `mgr.run()`. |
| `src/bins/dd-procmgr.rs` | Client CLI entry point. |
| `src/manager.rs` | `ProcessManager` — the central coordinator. |
| `src/process.rs` | `ManagedProcess` — per-process state machine and lifecycle logic. |
| `src/config.rs` | Configuration loading (`YamlConfigLoader`) and `ProcessConfig` schema. |
| `src/ordering.rs` | Topological sort for startup order based on `after`/`before` dependency declarations. |
| `src/grpc/` | gRPC server (`tonic`) serving the `ProcessManager` proto service. |
| `src/shutdown.rs` | Ordered, graceful shutdown logic (fan-out SIGTERM then wait). |
| `src/state.rs` | `ProcessState` enum with valid transition rules. |
| `proto/process_manager.proto` | gRPC service definition (Go package: `pbgo/procmgr`). |

### Configuration and build flags

The crate is built with Cargo and also has a Bazel target. The config directory is controlled by the `DD_PM_CONFIG_DIR` environment variable (default `/etc/datadog-agent/processes.d/`). There are no `datadog.yaml` keys for the daemon itself.

#### Configuration

Process definitions are YAML files placed in `/etc/datadog-agent/processes.d/` (overridable via `DD_PM_CONFIG_DIR`). The filename stem (without extension) becomes the process name. Names must be ASCII alphanumeric, hyphens, underscores, or dots.

Key fields of `ProcessConfig`:

| Field | Default | Description |
|-------|---------|-------------|
| `command` | (required) | Executable path. |
| `args` | `[]` | Argument list. |
| `env` | `{}` | Extra environment variables (parent environment is NOT inherited). |
| `environment_file` | none | Path to a `KEY=VALUE` env file; prefix with `-` to make optional. |
| `auto_start` | `true` | Whether to start automatically on daemon startup or config reload. |
| `condition_path_exists` | none | Skip start if this path does not exist. |
| `restart` | `never` | Restart policy: `always`, `on-failure`, `on-success`, `never`. |
| `restart_sec` | `1.0` | Base restart delay in seconds (doubles on each restart up to `restart_max_delay_sec`). |
| `restart_max_delay_sec` | `60.0` | Upper bound for restart backoff. |
| `start_limit_burst` | `5` | Maximum restarts within `start_limit_interval_sec`. |
| `start_limit_interval_sec` | `10` | Burst limiting window. |
| `runtime_success_sec` | `1` | If a process runs longer than this, the backoff counter resets. |
| `stop_timeout` | `90` | Seconds to wait for graceful stop before SIGKILL. |
| `after` / `before` | `[]` | Startup-order dependency declarations. Cycles are detected and logged as warnings. |
| `stdout` / `stderr` | `inherit` | `inherit`, `null`, or a file path. |

### Key functions

#### `ProcessManager`

The central `ProcessManager` struct holds an `Arc<RwLock<Vec<ManagedProcess>>>` and a `startup_order` index vector. It runs a `tokio::select!` event loop handling:

- SIGTERM / SIGINT — triggers ordered shutdown
- Process exit events — applies restart policy and schedules delayed restarts
- gRPC commands — `Create`, `Start`, `Stop`, `ReloadConfig`

#### `ManagedProcess` state machine

```
Created -> Running -> Exited (exit 0 without stop request)
                   -> Failed (non-zero exit or spawn error)
                   -> Stopped (stop explicitly requested)
```

Transitions are validated; invalid transitions are logged (and panicked in debug builds).

Each process is placed in its own process group (`process_group(0)`) so that signals are delivered to the entire process tree.

### Key interfaces

#### gRPC service

The `ProcessManager` proto service (defined in `proto/process_manager.proto`) exposes:

| RPC | Description |
|-----|-------------|
| `List` | Lists all managed processes with summary state. |
| `Describe` | Returns full detail for one process (by name or UUID prefix). |
| `GetStatus` | Daemon health and aggregate counts per state. |
| `Create` | Registers a new process at runtime (not from config file). |
| `Start` / `Stop` | Manual lifecycle control by name or UUID prefix. |
| `ReloadConfig` | Re-reads config files; adds new, removes deleted, restarts modified. |
| `GetConfig` | Returns config source and location. |

Process IDs can be resolved by full name or by UUID prefix (minimum 8 hex characters). Ambiguous prefixes return `INVALID_ARGUMENT`.

## Usage

### Running the daemon

```bash
# Default config directory: /etc/datadog-agent/processes.d/
dd-procmgrd

# Override config directory
DD_PM_CONFIG_DIR=/tmp/my-procs dd-procmgrd
```

### Example configuration file

```yaml
# /etc/datadog-agent/processes.d/datadog-agent-ddot.yaml
description: Datadog Distribution of OpenTelemetry Collector
command: /opt/datadog-agent/ext/ddot/embedded/bin/otel-agent
args:
  - run
  - --config
  - /etc/datadog-agent/otel-config.yaml
auto_start: true
condition_path_exists: /opt/datadog-agent/ext/ddot/embedded/bin/otel-agent
restart: on-failure
stdout: inherit
stderr: inherit
```

### Using the CLI

```bash
dd-procmgr list
dd-procmgr start datadog-agent-ddot
dd-procmgr stop datadog-agent-ddot
dd-procmgr reload-config
```

### Runtime process creation (via gRPC)

Processes created at runtime via the `Create` RPC are tracked separately (`ProcessOrigin::Runtime`) and are not removed by `ReloadConfig`.

## Build

The crate is built with Cargo (`pkg/procmgr/rust/Cargo.toml`) and also has a Bazel target (`pkg/procmgr/rust/BUILD.bazel`). The proto file generates Go stubs into `pkg/proto/pbgo/procmgr`.

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/system` | [system.md](../util/system.md) | `pkg/util/system/socket` provides the `IsAvailable` and `GetSocketAddress` helpers used by Go-side callers to probe the `dd-procmgrd` gRPC socket before connecting. The daemon binds a Unix domain socket (or TCP address) whose reachability can be checked with `socket.IsAvailable(path, timeout)` before issuing gRPC calls. |
| `comp/core/config` | [config.md](../../comp/core/config.md) | The config directory override (`DD_PM_CONFIG_DIR`) is an environment variable rather than an agent config key. However, the DDOT process definition (`datadog-agent-ddot.yaml`) references binary paths that are determined by the agent installation layout, which is managed via `comp/core/config` key `otelcollector.enabled`. When the OTel collector feature is enabled in `datadog.yaml`, the corresponding process config is expected to be present in the `processes.d/` directory. |
