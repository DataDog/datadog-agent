# JMX checks (JMXFetch)

-----

JMX-based integrations (Kafka, Cassandra, Tomcat, ActiveMQ, ...) are not run inside the agent process at all. The agent launches [JMXFetch](https://github.com/DataDog/jmxfetch), a Java application, as a supervised JVM sidecar; JMXFetch connects to the monitored JVMs over JMX/RMI, fetches MBean attributes, and reports the resulting metrics *back into the agent* through [DogStatsD](../dogstatsd/internals.md). Configuration flows the other way over the agent's authenticated [IPC API](../processes/ipc.md). JMX instances therefore never become `check.Check` objects and bypass the entire [check collector](collector.md) pipeline.

Everything here is compiled under the `jmx` build tag; images and binaries built without it have no JMX subsystem.

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/jmxfetch/jmxfetch.go`](<<<SRC>>>/pkg/jmxfetch/jmxfetch.go) | JVM launch: command line, classpath, heap sizing, reporter selection, log piping |
| [`pkg/jmxfetch/scheduler.go`](<<<SRC>>>/pkg/jmxfetch/scheduler.go) | The Autodiscovery scheduler named `jmx` |
| [`pkg/jmxfetch/state.go`](<<<SRC>>>/pkg/jmxfetch/state.go) | Global cache of scheduled JMX configs, polled over IPC; lazy JVM startup |
| [`pkg/jmxfetch/runner.go`](<<<SRC>>>/pkg/jmxfetch/runner.go) | Runner state around the `JMXFetch` process handle |
| [`pkg/jmxfetch/limiter.go`](<<<SRC>>>/pkg/jmxfetch/limiter.go) | Restart rate limiting |
| [`pkg/jmxfetch/jmxfetch_nix.go`](<<<SRC>>>/pkg/jmxfetch/jmxfetch_nix.go) / [`jmxfetch_windows.go`](<<<SRC>>>/pkg/jmxfetch/jmxfetch_windows.go) | Platform-specific stop/kill semantics |
| [`pkg/collector/check/jmx.go`](<<<SRC>>>/pkg/collector/check/jmx.go) | `IsJMXConfig` / `IsJMXInstance` detection |
| [`pkg/config/setup/standard_names.go`](<<<SRC>>>/pkg/config/setup/standard_names.go) | Legacy `StandardJMXIntegrations` name list |
| [`comp/api/api/apiimpl/internal/agent/agent_jmx.go`](<<<SRC>>>/comp/api/api/apiimpl/internal/agent/agent_jmx.go) | `GET /agent/jmx/configs` and `POST /agent/jmx/status` IPC endpoints |
| [`comp/agent/jmxlogger`](<<<SRC>>>/comp/agent/jmxlogger) | Component writing JMXFetch output to `jmxfetch.log` |
| [`pkg/status/jmx`](<<<SRC>>>/pkg/status/jmx) | JMXFetch section of `agent status`, fed by the status POSTs |
| [`cmd/agent/subcommands/jmx/command.go`](<<<SRC>>>/cmd/agent/subcommands/jmx/command.go) | `agent jmx collect / list ...` troubleshooting commands |
| [`pkg/cli/standalone/jmx.go`](<<<SRC>>>/pkg/cli/standalone/jmx.go) | One-shot JMXFetch execution shared by the CLI commands |

## Detecting a JMX instance

[`check.IsJMXInstance`](<<<SRC>>>/pkg/collector/check/jmx.go) classifies an instance as JMX when any of these holds:

1. The integration name is in the legacy [`StandardJMXIntegrations`](<<<SRC>>>/pkg/config/setup/standard_names.go) list: `activemq`, `activemq_58`, `cassandra`, `jmx`, `kafka`, `presto`, `solr`, `tomcat`. This list is deprecated and frozen; it exists for old configs only.
1. The instance or `init_config` sets `is_jmx: true`.
1. The instance or `init_config` sets `loader: jmx`.

The [check scheduler](collector.md) skips such instances, and the dedicated `jmx` Autodiscovery scheduler picks them up instead. A single config file can mix JMX and non-JMX instances; they are split per instance.

## Scheduling: the "jmx" AD scheduler

[`jmxfetch.RegisterWith(ac)`](<<<SRC>>>/pkg/jmxfetch/scheduler.go) registers `JmxScheduler` with [Autodiscovery](autodiscovery.md) under the name `jmx` (done in the agent's run command, right next to the `check` scheduler). Its `Schedule`:

1. Skips non-check configs and configs filtered out for metrics by the workload filter.
1. Extracts each JMX instance into its own single-instance `integration.Config` with a derived ID (`<name>_<digest>`), so AD identifiers, service IDs, and log config are preserved per instance.
1. Applies `java_bin_path`, `java_options`, `tools_jar_path`, and `custom_jar_paths` from the config onto the runner (first writer wins — these are process-wide JVM settings).
1. Stores the config in the global [`jmxState`](<<<SRC>>>/pkg/jmxfetch/state.go) cache and, on the first config, lazily starts the JVM (with retries). No JMX configs scheduled means no JVM running.

`Unschedule` removes entries from the cache; JMXFetch notices on its next config poll. The cache tracks a modification timestamp used for the polling described below.

## Launching the JVM

[`JMXFetch.Start`](<<<SRC>>>/pkg/jmxfetch/jmxfetch.go) builds and spawns:

```text
java [java_options] -classpath [custom jars:]<dist>/jmx/jmxfetch.jar org.datadog.jmxfetch.App
     --ipc_host <ipc address> --ipc_port <cmd_port>
     --check_period ... --thread_pool_size ... --collection_timeout ...
     --reconnection_timeout ... --reconnection_thread_pool_size ...
     --log_level ... --reporter statsd:<endpoint> --statsd_queue_size ...
     collect
```

Notable mechanics:

1. **Heap sizing**: by default the JVM gets `-Xms50m -Xmx200m`. With `jmx_use_container_support: true` it instead gets `-XX:+UseContainerSupport` plus `-XX:MaxRAMPercentage=<jmx_max_ram_percentage>` (default 25); the legacy `jmx_use_cgroup_memory_limit` uses the older experimental cgroup flag. Explicit `-Xmx`/`-Xms` in `java_options` disable the automatic options; setting both container-support and cgroup options is an error.
1. **Auth**: the IPC auth token is passed in the child's environment as `SESSION_TOKEN`; JMXFetch presents it on every IPC call. `-Djdk.attach.allowAttachSelf=true` is always appended, and `java.io.tmpdir` is pointed at `<run_path>/jmxfetch`.
1. **Reporter**: for normal operation, `--reporter statsd:unix://<dogstatsd_socket>` when the UDS socket is configured and live, else `statsd:<host>:<dogstatsd_port>` over UDP (host defaults to `localhost`). The `console`/`json` reporters exist for the CLI commands.
1. **Logs**: stdout/stderr are scanned line-by-line and forwarded to the agent logger through [`comp/agent/jmxlogger`](<<<SRC>>>/comp/agent/jmxlogger), landing in `jmxfetch.log`.
1. `JAVA_TOOL_OPTIONS` can be injected via `jmx_java_tool_options`.

## Config transport: IPC polling

JMXFetch pulls its check configurations from the agent rather than reading files:

1. The JVM periodically calls `GET https://<ipc_host>:<cmd_port>/agent/jmx/configs?timestamp=<last-seen>` on the agent's IPC API ([`agent_jmx.go`](<<<SRC>>>/comp/api/api/apiimpl/internal/agent/agent_jmx.go)), authenticated with the session token. If nothing changed since `timestamp`, the agent answers `204 No Content`; otherwise it serializes the current [`jmxState`](<<<SRC>>>/pkg/jmxfetch/state.go) cache (init configs, instances, check names, source metadata) as JSON.
1. JMXFetch posts collection status to `POST /agent/jmx/status`; the payload is stored by [`pkg/status/jmx`](<<<SRC>>>/pkg/status/jmx) and rendered as the JMXFetch section of `agent status` and in the [inventorychecks](<<<SRC>>>/comp/metadata/inventorychecks) metadata. This is the *only* status signal — the collector's runner stats never see JMX checks.

`metrics.yaml` files inside `<check>.d` folders carry JMX metric/attribute definitions (which MBeans to collect) and are attached to the config as `MetricConfig`; they are not standalone check configs.

## Data return path

Metrics collected by JMXFetch re-enter the agent through the [DogStatsD server](../dogstatsd/internals.md) (UDS preferred, UDP fallback) and from there follow the ordinary [metrics pipeline](../pipelines/metrics/aggregation.md). Consequences: DogStatsD must be enabled for JMX metrics to work, JMX metrics are subject to DogStatsD tagging/origin behavior rather than check-sender behavior, and bean-heavy instances show up as DogStatsD traffic.

## Lifecycle and supervision

1. The runner starts the JVM with lifecycle management: a [`Monitor()`](<<<SRC>>>/pkg/jmxfetch/jmxfetch.go) goroutine waits on the process and restarts it if it exits with an error, rate-limited to `jmx_max_restarts` (default 3) within `jmx_restart_interval` seconds (default 5); past the limit, a startup error is recorded and surfaces in `agent status`.
1. A liveness health handle named `jmxfetch` beats every 500 ms while the monitor runs.
1. On Unix, `Stop()` sends SIGTERM and escalates to SIGKILL after a short grace period ([`jmxfetch_nix.go`](<<<SRC>>>/pkg/jmxfetch/jmxfetch_nix.go)); Windows termination differs ([`jmxfetch_windows.go`](<<<SRC>>>/pkg/jmxfetch/jmxfetch_windows.go)).
1. A clean JVM exit (no error) ends supervision without restart.

## Configuration

All keys live in the main `datadog.yaml`:

| Key | Purpose |
|---|---|
| `jmx_use_container_support`, `jmx_max_ram_percentage` | Container-aware JVM heap sizing (default 25% when enabled) |
| `jmx_use_cgroup_memory_limit` | Legacy cgroup heap flag (mutually exclusive with container support) |
| `jmx_custom_jars` | Extra jars prepended to the classpath |
| `jmx_java_tool_options` | `JAVA_TOOL_OPTIONS` for the child JVM |
| `jmx_max_restarts`, `jmx_restart_interval` | Crash-restart rate limiting |
| `jmx_check_period`, `jmx_thread_pool_size`, `jmx_collection_timeout` | Main collection loop tuning (ms / threads / seconds) |
| `jmx_reconnection_timeout`, `jmx_reconnection_thread_pool_size` | Reconnection behavior |
| `jmx_statsd_client_queue_size`, `jmx_statsd_telemetry_enabled`, `jmx_statsd_client_use_non_blocking`, `jmx_statsd_client_buffer_size`, `jmx_statsd_client_socket_timeout` | statsd client tuning inside JMXFetch |
| `jmx_telemetry_enabled` | JMXFetch self-telemetry |
| `jmx_log_file` / `log_format_rfc3339` | Logging |
| `cmd_port` | Agent IPC port the JVM polls (default 5001) |
| `dogstatsd_socket`, `dogstatsd_port` | Metrics return path |
| `run_path` | Parent of the JVM temp dir |

Per-check JVM options (`java_bin_path`, `java_options`, `tools_jar_path`, `custom_jar_paths`, `process_name_regex`) come from the integration's `init_config`/instance and are applied when the runner is configured.

## Deployment notes

1. `jmxfetch.jar` ships in the dist path of full agent packages and container images; slim images built without the `jmx` build tag drop the jar, the scheduler, and the CLI commands. There is no bundled JRE on all platforms — containers include one, and host installs use the system `java` unless `java_bin_path` points elsewhere.
1. In Kubernetes, `jmx_use_container_support: true` matters: without it the default 200 MB heap cap applies regardless of pod limits, and with generous pod limits the default 25% `MaxRAMPercentage` can be a large number.
1. JMX checks can be dispatched as [cluster checks](../containers/cluster-checks.md); the runner that receives the config launches its own JMXFetch.

## Troubleshooting

1. `agent status` → JMXFetch section shows initialized vs failed checks, as reported by the JVM itself.
1. `agent jmx list everything|matching|with-metrics|limited|collected|not-matching` and `agent jmx collect` ([`command.go`](<<<SRC>>>/cmd/agent/subcommands/jmx/command.go)) run a one-shot JMXFetch with console or JSON reporters against the currently scheduled configs — useful for verifying bean matching without touching the running sidecar.
1. `agent check <jmx-check>` does not run the check in-process; it degrades to the equivalent of `jmx list with-metrics`.
1. JMXFetch logs go to `jmxfetch.log` next to the agent logs (they are also in flares).

## Gotchas

1. JMX checks are invisible to the collector: no `check.Check`, no runner stats, no `datadog.agent.check_status` service check. If the JVM is down, JMX checks silently stop reporting until the restart limiter gives up and a startup error appears in status.
1. One JVM serves all JMX integrations; per-check `java_options` are first-come-first-served process-wide settings, not per-check isolation.
1. The legacy name list makes `is_jmx` unnecessary for the standard eight integrations — but any *new* JMX integration must set `is_jmx: true` or `loader: jmx` explicitly.
1. Metrics arrive via DogStatsD, so tags behave like DogStatsD tags (no check-level `empty_default_hostname` semantics, and `bean` tags are constructed by JMXFetch itself).
1. The config poll is timestamp-based: an agent restart resets the cache and the JVM re-fetches everything; a hung JVM keeps polling old configs until its next cycle.
1. `cmd_port` collisions or IPC auth-token issues break JMX configuration delivery even though the JVM process looks healthy.
