# pkg/jmxfetch

### Purpose

Manages the lifecycle of the JMXFetch subprocess — a Java process that collects JMX metrics from services like Cassandra, Kafka, Hazelcast, or any custom JMX integration. The package starts and monitors the `jmxfetch.jar` process, passes check configurations to it via the Agent's IPC channel, and forwards the collected metrics to DogStatsD.

### Build tag

All non-stub code is gated by the `jmx` build tag. When built without this tag, the package exposes no-op stubs for all public functions so the rest of the codebase compiles cleanly. Always check for `//go:build jmx` or `//go:build !jmx` at the top of source files.

### Key elements

**`JMXFetch`** (`jmxfetch.go`, build tag `jmx`)

The central struct. It wraps the `*exec.Cmd` that runs `jmxfetch.jar` and holds all runtime configuration.

Key fields:

| Field | Description |
|---|---|
| `JavaBinPath` | Path to `java` binary (default: `"java"`) |
| `JavaOptions` | JVM flags appended to the command line |
| `JavaCustomJarPaths` | Additional JARs on the classpath |
| `JavaToolsJarPath` | Path to `tools.jar` (needed for attach-based collection) |
| `LogLevel` | Log level forwarded to JMXFetch (mapped via `jmxLogLevelMap`) |
| `Command` | JMXFetch command (default: `"collect"`) |
| `Reporter` | One of `ReporterStatsd`, `ReporterConsole`, `ReporterJSON` |
| `Checks` | Check names to pass on startup |
| `DSD` | DogStatsD server component (used to resolve the reporter endpoint) |
| `IPCHost` / `IPCPort` | Override the Agent IPC address passed to JMXFetch |

**`JMXReporter`**

String type with three constants: `ReporterStatsd` (default, sends metrics to DogStatsD), `ReporterConsole`, `ReporterJSON`.

**`JMXFetch.Start(manage bool) error`**

Builds the `java` command line and starts the subprocess. The classpath is assembled as:

```
[java_custom_jars] : [jmx_custom_jars config] : [dist/jmx/jmxfetch.jar]
```

Key JVM options applied automatically:
- `-Djdk.attach.allowAttachSelf=true` (always)
- Memory limits: `-Xmx200m -Xms50m` (unless overridden, or cgroup/container support is enabled)
- Container support: `-XX:+UseContainerSupport` with `-XX:MaxRAMPercentage` (when `jmx_use_container_support: true`)
- Cgroup memory limit: `-XX:+UseCGroupMemoryLimitForHeap` (when `jmx_use_cgroup_memory_limit: true`)
- A dedicated temp dir under `run_path/jmxfetch`

`SESSION_TOKEN` is injected into the subprocess environment via the IPC component's auth token.

If `manage` is `true`, `Start` launches a `Monitor` goroutine that handles automatic restarts.

**`JMXFetch.Stop() error`** (platform-specific)

Sends `SIGTERM` to the subprocess and waits 500 ms before sending `SIGKILL`. On Windows, the implementation differs.

**`JMXFetch.Monitor()`**

Runs in a goroutine when `manage=true`. Waits for the subprocess to exit and restarts it if the `restartLimiter` allows it. Restart limits are controlled by `jmx_max_restarts` and `jmx_restart_interval` config keys. A liveness health check (`health.RegisterLiveness("jmxfetch")`) is maintained via a 500 ms heartbeat ticker.

**`JMXFetch.ConfigureFromInitConfig` / `ConfigureFromInstance`**

Populate `JavaBinPath`, `JavaOptions`, `JavaToolsJarPath`, and `JavaCustomJarPaths` from check YAML `init_config` and `instances` sections respectively. Only set if the field is not already set, so CLI overrides take precedence.

**`restartLimiter`** (`limiter.go`)

Sliding-window rate limiter for subprocess restarts. Tracks the last `maxRestarts` stop times in a circular buffer and returns `false` from `canRestart` if they all fall within `interval` seconds.

**`JmxScheduler`** (`scheduler.go`, build tag `jmx`)

Implements `autodiscovery.Scheduler`. Registered with AutoDiscovery by calling `RegisterWith(ac)`. On each `Schedule` call it filters for JMX check instances (`check.IsJMXInstance`), configures the runner, and adds the config to the global `state`. On `Unschedule`, it removes configs from state.

**Package-level state** (`state.go`, build tag `jmx`)

A singleton `jmxState` holds the `runner` (wraps a `JMXFetch` instance), a `BasicCache` of scheduled configs, and a mutex. Key exported functions:

- `InitRunner(server, logger, ipc)` — initializes the runner; call once at agent startup.
- `RegisterWith(ac)` — registers the JMX scheduler with AutoDiscovery.
- `AddScheduledConfig(c)` — adds a config to the state (JMXFetch polls this list via IPC).
- `GetScheduledConfigs()` — returns the current config map.
- `GetScheduledConfigsModificationTimestamp()` — used by JMXFetch's IPC polling to detect config changes.
- `StopJmxfetch()` — stops the subprocess.
- `GetIntegrations()` — returns a JSON-serializable map of scheduled configs (used by the Agent's internal API).

### Usage

At agent startup (`cmd/agent/subcommands/run/command.go`), `jmxfetch.InitRunner` is called to wire in the DogStatsD server and IPC components, then `jmxfetch.RegisterWith(ac)` connects the scheduler to AutoDiscovery.

When AutoDiscovery finds a JMX-annotated service (e.g. a Kafka container), `JmxScheduler.Schedule` receives the config, calls `InitRunner` to configure the `JMXFetch` struct from `init_config` / instance YAML, and starts JMXFetch if not already running. JMXFetch then polls the Agent's IPC endpoint to retrieve the scheduled config list and begins collecting metrics, reporting them to DogStatsD.

The `comp/api` package exposes `/agent/jmx/configs` and `/agent/jmx/integrations` endpoints backed by `GetScheduledConfigs` and `GetIntegrations`.

For the standalone `jmx` CLI subcommand (`pkg/cli/standalone/jmx.go`), a `JMXFetch` instance is created directly with `ReporterConsole` or `ReporterJSON` reporter and `manage=false`.

### Configuration keys

| Key | Default | Description |
|---|---|---|
| `jmx_check_period` | 1500 | Main loop period for JMXFetch (ms) |
| `jmx_thread_pool_size` | 3 | Thread pool size |
| `jmx_collection_timeout` | 60 | Metric collection timeout (s) |
| `jmx_reconnection_timeout` | 10 | Reconnection timeout (s) |
| `jmx_max_restarts` | 3 | Max subprocess restarts within interval |
| `jmx_restart_interval` | 5 | Restart rate-limit window (s) |
| `jmx_use_container_support` | false | Use JVM container support (`-XX:+UseContainerSupport`) |
| `jmx_use_cgroup_memory_limit` | false | Use cgroup memory limit |
| `jmx_max_ram_percentage` | 25.0 | Heap as % of RAM when container support is on |
| `jmx_custom_jars` | `[]` | Additional JAR paths added to classpath |
| `jmx_java_tool_options` | `""` | Value injected as `JAVA_TOOL_OPTIONS` env var |

---

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`comp/agent/jmxlogger`](../../comp/agent/jmxlogger.md) | `jmxlogger.Component` is injected into `JMXFetch` via `NewJMXFetch(logger, ipc)`. JMXFetch pipes its subprocess stdout to `logger.JMXInfo` and stderr to `logger.JMXError`. The logger is backed by `pkg/util/log/setup.BuildJMXLogger`, which always runs at `InfoLvl` because JMXFetch performs its own level filtering. In CLI mode (`agent jmx collect`) the logger is directed to a timestamped file in the flare directory via `jmxloggerimpl.NewCliParams`; in normal agent operation it respects `jmx_log_file` from `datadog.yaml`. |
| [`pkg/collector`](collector/collector.md) | `JmxScheduler` implements `autodiscovery.Scheduler` and is registered alongside the standard `CheckScheduler`. When AutoDiscovery resolves a JMX-annotated service, both the `CheckScheduler` (which handles check-lifecycle) and the `JmxScheduler` (which manages the JMXFetch subprocess config list) receive the config. The JMXFetch subprocess does not run through the standard `runner.Worker` pool; instead, collected metrics flow from the subprocess to DogStatsD and are received by the agent's DogStatsD listener as ordinary metrics, bypassing the check runner entirely. |
| [`pkg/util/log`](util/log.md) | `pkg/jmxfetch` uses `pkg/util/log` directly (`pkglog.Infof`, `pkglog.Errorf`) for its own Go-level log messages (subprocess start/stop, restart events, config loading). The JMXFetch subprocess output is routed through the dedicated `jmxlogger` component rather than `pkg/util/log`. The `JMXLoggerName = "JMXFETCH"` constant defined in `pkg/util/log/setup` identifies the JMX log stream in log output. |

### Subprocess vs. in-process metrics collection

JMXFetch metrics travel a different path than metrics collected by Go checks:

```
JMX service (e.g. Kafka)
    |
    v
jmxfetch.jar (subprocess)
    ├─> IPC poll → GET /agent/jmx/configs   (reads AddScheduledConfig list)
    └─> StatsD socket → DogStatsD server
            |
            v
        aggregator/forwarder → Datadog intake
```

This means JMX metrics do not pass through `pkg/collector/runner.Worker` and are not subject to `check_runners` scaling. The `jmx_check_period` config key controls the JMXFetch polling cadence independently.

### Liveness monitoring

`JMXFetch.Monitor()` maintains a `health.RegisterLiveness("jmxfetch")` heartbeat updated every 500 ms. If the subprocess exits abnormally, the heartbeat stops. The `restartLimiter` prevents restart storms: if `jmx_max_restarts` (default 3) restarts all occur within `jmx_restart_interval` (default 5 s), further restarts are suppressed and the liveness check will fail, triggering an agent-level health alert.
