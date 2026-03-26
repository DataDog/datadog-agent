> **TL;DR:** `pkg/discovery` is a Linux-only system-probe module that identifies running services by determining their name, programming language, APM instrumentation state, and open ports from `/proc`, delivering the results via a `GET /discovery/services` REST endpoint.

# pkg/discovery — Service and Application Discovery

## Purpose

`pkg/discovery` implements **service discovery** for Universal Service Monitoring (USM) and
APM auto-instrumentation. It runs as a module inside system-probe and exposes a REST API that
other agent components poll to get the list of services currently running on the host.

For each listening process, the module determines:

- The **service name** — derived from the process command line, application framework
  configuration files, or Unified Service Tagging (UST) environment variables.
- The **programming language** — detected from the binary name, runtime markers, or
  `/proc/<pid>/maps`.
- The **APM instrumentation state** — whether the process already carries a Datadog tracer
  (via `ddtrace`, `dd-java-agent.jar`, `Datadog.Trace.dll`, `CORECLR_ENABLE_PROFILING`, or
  tracer metadata in `/proc/<pid>/fd`).
- The **open TCP/UDP ports** — collected from the network namespace.

The module is Linux-only (build constraint on the implementation files). The REST endpoint is
`GET /discovery/services` served by system-probe and consumed by the trace-agent and USM
components.

## Key elements

### Key types

#### `model/`

Defines the wire types for the `/discovery/services` endpoint.

| Type | Description |
|------|-------------|
| `Service` | Represents one listening process: `PID`, `GeneratedName`, `GeneratedNameSource`, `AdditionalGeneratedNames`, `Language`, `APMInstrumentation`, `TCPPorts`, `UDPPorts`, `UST`, `TracerMetadata`, `LogFiles`. |
| `UST` | Unified Service Tagging fields extracted from env vars: `Service`, `Env`, `Version`. |
| `ServicesResponse` | Top-level response body: `Services []Service`, `InjectedPIDs []int`, `GPUPIDs []int`. |

`GPUPIDs` is populated by the Rust core implementation (`module/rust/src/services.rs`) and lists PIDs that have GPU resources (e.g. processes that have opened CUDA libraries). This field is consumed by the GPU monitoring system to correlate process-level GPU activity.

`InjectedPIDs` lists PIDs that have already received APM library injection from the Admission Controller, allowing consumers to skip re-injection.

#### `language/`

Defines the `Language` string type and its constants:

```go
const (
    Unknown  Language = "UNKNOWN"
    Java     Language = "jvm"
    Node     Language = "nodejs"
    Python   Language = "python"
    Ruby     Language = "ruby"
    DotNet   Language = "dotnet"
    Go       Language = "go"
    CPlusPlus Language = "cpp"
    PHP      Language = "php"
)
```

Language detection is delegated to `pkg/languagedetection/privileged.LanguageDetector` (reads
ELF headers and interpreter paths). The `language/` sub-package only provides the shared
type definitions.

> **Cross-reference:** `pkg/languagedetection` is the two-tier detection library that
> powers this step. Its privileged tier (`pkg/languagedetection/privileged`) reads ELF
> headers and interpreter paths from `/proc/<pid>/exe` and is the actual implementation
> called by the discovery module. The unprivileged tier (process-agent / node-agent) runs
> command-line classification only. See [`languagedetection.md`](../languagedetection.md)
> for the full API including `LanguageName` constants, the `LanguageDetector` LRU cache,
> and the `TracerDetector` / `InjectorDetector` sub-detectors.

### Key interfaces

#### `tracermetadata/`

Parses tracer-generated metadata written by running APM tracers to an in-memory file descriptor (memfd). This sub-package (no build constraint) defines:

| Symbol | Description |
|--------|-------------|
| `TracerMetadata` | Struct mirroring the libdatadog schema: `SchemaVersion`, `RuntimeID`, `TracerLanguage`, `TracerVersion`, `Hostname`, `ServiceName`, `ServiceEnv`, `ServiceVersion`, `ProcessTags`, `ContainerID`, `LogsCollected`. |
| `GetTracerMetadataFromPath(fdPath)` | Reads and decodes a `TracerMetadata` from a `/proc/<pid>/fd/<n>` path pointing to a memfd. |
| `ShouldSkipServiceTagKV(tagKey, tagValue, ustService, ustEnv, ustVersion)` | Returns `true` if the tag duplicates a UST field, preventing double-reporting. |
| `TracerMetadata.Tags()` | Iterator over all non-empty tag key-value pairs. |
| `TracerMetadata.GetTags()` | Returns a `[]string` of all non-empty `key:value` pairs. |
| `tracermetadata/language.GetLanguage(meta)` | Converts `TracerMetadata.TracerLanguage` to a `languagemodels.Language`. |

The discovery module uses `tracermetadata.GetTracerMetadataFromPath` for each `/proc/<pid>/fd` entry that refers to a tracer memfd to extract rich service and tag information directly from the running tracer, without environment variable polling.

### Key functions

#### `apm/`

Detects whether a process already carries APM instrumentation (build constraint: `linux`).

| Symbol | Description |
|--------|-------------|
| `Instrumentation` | String type: `None` or `Provided`. |
| `Detect(lang, ctx, tracerMetadata) Instrumentation` | Entry point. If `tracerMetadata != nil` (tracer has written its metadata file) returns `Provided`. Otherwise delegates to a per-language detector. |

Per-language detection strategies:

| Language | Strategy |
|----------|----------|
| Python | Scans `/proc/<pid>/maps` for paths containing `/ddtrace/`. |
| Java | Checks command line and `JAVA_TOOL_OPTIONS` / `_JAVA_OPTIONS` / `JDK_JAVA_OPTIONS` for `-javaagent:` pointing to a Datadog JAR. |
| .NET | Checks `CORECLR_ENABLE_PROFILING=1` env var, then scans `/proc/<pid>/maps` for `Datadog.Trace.dll`. |

#### `usm/`

Infers the best service name for a process from its command line and application-level
configuration files. No build constraint — used on all platforms.

| Type | Description |
|------|-------------|
| `ServiceMetadata` | Holds `Name string`, `Source ServiceNameSource`, `AdditionalNames []string`. |
| `ServiceNameSource` | String enum: `CommandLine`, `Python`, `Nodejs`, `Gunicorn`, `Rails`, `Laravel`, `Spring`, `JBoss`, `Tomcat`, `WebLogic`, `WebSphere`. |
| `DetectionContext` | Input to all detectors: `Pid`, `Args []string`, `Envs envs.Variables`, `fs fs.SubFS`, `ContextMap DetectorContextMap`. |

Framework-specific detectors (one file each):

`java.go`, `spring.go`, `jboss.go`, `jee.go`, `tomcat.go`, `weblogic.go`, `websphere.go`,
`python.go`, `nodejs.go`, `ruby.go`, `php.go`, `laravel.go`, `yaml.go`, `erlang.go`.

Each implements the internal `detector` interface:
```go
type detector interface {
    detect(remainingArgs []string) (ServiceMetadata, bool)
}
```

A `simpleDetector` handles generic command-line heuristics; `dotnetDetector` handles .NET
assemblies. Framework detectors read application config files (e.g. `spring.application.name`
in `application.properties`, `package.json` for Node.js, `setup.py` / `setup.cfg` for Python).

The `resolveWorkingDirRelativePath` method resolves relative file paths by checking both
`/proc/<pid>/cwd` and the `PWD` environment variable.

#### `core/`

Lightweight Linux-only package (`//go:build linux`) holding the `Discovery` struct and `PidSet` helper used internally by the module implementation.

| Symbol | Description |
|--------|-------------|
| `Discovery` | Thin wrapper holding a `*DiscoveryConfig`. Provides a `Close()` hook for future resource cleanup. |
| `PidSet` | `map[int32]struct{}` with `Has`, `Add`, `Remove` helpers. Used by the module to track sets of known/GPU/injected PIDs efficiently. |
| `DiscoveryConfig` | Config struct loaded from the `discovery` section of `system-probe.yaml`. |

### Configuration and build flags

The module and its implementation files are Linux-only (build constraint `linux`). It has no eBPF programs. Configuration is read from the `discovery` section of `system-probe.yaml` into `DiscoveryConfig`. There are no top-level `datadog.yaml` keys.

#### `module/`

System-probe module implementation (build constraint: `linux`). Implements
`pkg/system-probe/api/module.Module`.

| Symbol | Description |
|--------|-------------|
| `NewDiscoveryModule(cfg, deps) (module.Module, error)` | Factory function registered in `cmd/system-probe/modules/discovery_linux.go`. |
| `discovery` struct | Holds a `core.Discovery` instance, a `sync.RWMutex`, and a `privileged.LanguageDetector`. |
| `Register(httpMux)` | Registers three HTTP handlers: `/status`, `/state`, `/discovery/services`. |
| `handleServices` | Main handler. Enumerates processes, filters ignored commands (`ignoreComms`, `ignoreFamily`), calls USM detection and language detection, assembles `ServicesResponse`. |

**Process filtering** — the following processes are always excluded from service discovery:

- Exact names: `chronyd`, `cilium-agent`, `containerd`, `dhclient`, `dockerd`, `kubelet`,
  `livenessprobe`, `local-volume-pr`, `sshd`, `systemd`.
- Prefix families (name up to the first hyphen): `systemd-*`, `datadog-*`,
  `containerd-*`, `docker-*`.

**Concurrency** — `handleServices` is wrapped with `utils.WithConcurrencyLimit` to cap
parallel requests to `utils.DefaultMaxConcurrentRequests`.

#### `envs/`

Provides `Variables`, a lazy accessor for process environment variables read from
`/proc/<pid>/environ`. Accessed via `envs.Get(name)`.

## Usage

### System-probe module registration

`cmd/system-probe/modules/discovery_linux.go` registers the module at init time:

```go
var DiscoveryModule = &module.Factory{
    Name:         config.DiscoveryModule,
    Fn:           discoverymodule.NewDiscoveryModule,
    NeedsEBPF:    func() bool { return false },
    OptionalEBPF: true,
}
```

No eBPF programs are required; the module relies entirely on `/proc` and network namespace
socket enumeration.

### Consuming the API

The trace-agent and USM consumers poll `GET /discovery/services` via system-probe's Unix
socket. The response is a JSON-serialized `ServicesResponse`.

**USM integration:** `pkg/network/usm` (Universal Service Monitoring) is a primary
consumer of the discovery service. USM uses process name and port information to correlate
eBPF-captured traffic to service names. It also drives the `ProcessMonitor` for
exec/exit events needed for uprobe attachment — the same process lifecycle events that
keep the discovery module's internal state current. See
[`usm.md`](../network/usm.md) for how the discovery service name feeds into USM
protocol stats.

**Process monitor:** `pkg/process/monitor` provides the singleton process-lifecycle
monitor (exec/exit via netlink or eBPF event stream) that the discovery module and USM
both depend on for staying current with running processes. See
[`monitor.md`](../process/monitor.md) for the `ProcessMonitor` API and `InitializeEventConsumer`.

### Data flow

```
system-probe (Linux)
  └─> discovery.handleServices
       ├─> enumerate /proc PIDs (via core.PidSet + /proc scan)
       ├─> filter ignored comms (ignoreComms / ignoreFamily)
       ├─> privileged.LanguageDetector  → Language       [pkg/languagedetection/privileged]
       ├─> usm.Detect (DetectionContext) → ServiceMetadata (name + source)
       ├─> apm.Detect                   → Instrumentation
       ├─> tracermetadata.GetTracerMetadataFromPath → TracerMetadata[]
       ├─> netns socket scan            → TCPPorts, UDPPorts
       └─> assemble model.Service
            ├─> InjectedPIDs  (processes with APM library already injected)
            ├─> GPUPIDs       (processes with GPU access, fed by Rust core)
            └─> JSON response to caller (trace-agent / USM / APM injector)
```

**Privileged language detection detail:** `privileged.NewLanguageDetector()` is instantiated once inside the discovery module and caches results by binary device+inode (LRU, size 1000). This avoids repeated ELF parsing when the same binary runs as multiple processes. The `DetectWithPrivileges` call requires the system-probe to run as root or with `CAP_PTRACE`. See [`languagedetection.md`](../languagedetection.md) for the full detector chain (Go build-info, .NET PE/ELF signatures, `TracerDetector`, `InjectorDetector`).

### Adding a new framework detector in `usm/`

1. Create `<framework>.go` in `pkg/discovery/usm/`.
2. Implement the `detector` interface (`detect(remainingArgs []string) (ServiceMetadata, bool)`).
3. Register the detector in `service.go` by adding it to the detector chain for the relevant
   language (Java, Python, Node.js, etc.).

### Adding a new APM instrumentation detector in `apm/`

1. Implement a `detector` function with signature `func(ctx usm.DetectionContext) Instrumentation`.
2. Add an entry to `detectorMap` keyed by the target `language.Language`.

### Testing

```bash
# Unit tests (no special build tags needed for most)
dda inv test --targets=./pkg/discovery/...

# Linux-specific module tests
dda inv test --targets=./pkg/discovery/module/...
```

Integration tests in `module/impl_services_test.go` spawn real processes and assert that
`handleServices` returns the expected service names and languages.

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/languagedetection` | [`languagedetection.md`](../languagedetection.md) | Provides `privileged.LanguageDetector` — the ELF/interpreter-based language detector called by `discovery.module`. The `NewLanguageDetector()` constructor wires `TracerDetector`, `InjectorDetector`, `GoDetector`, and `DotnetDetector`. Also defines `LanguageName` constants mirrored by `discovery/language`, and provides the `GetLanguage(meta)` helper (via `tracermetadata/language`) for mapping tracer-reported language strings to `languagemodels.Language`. |
| `pkg/network/usm` | [`usm.md`](../network/usm.md) | Primary consumer of the `/discovery/services` endpoint. Uses service names from discovery to label USM protocol stats, and relies on the same `ProcessMonitor` infrastructure for uprobe lifecycle management. `NeedProcessMonitor(cfg)` from `usm/config` determines whether the process monitor should be initialised alongside USM. |
| `pkg/process/monitor` | [`monitor.md`](../process/monitor.md) | Singleton process exec/exit monitor (netlink or eBPF event stream). The discovery module and USM both call `GetProcessMonitor()` to track process lifecycles without running duplicate netlink sockets. `SubscribeExec` must be called before `Initialize` to receive callbacks for already-running processes. |
