> **TL;DR:** `pkg/process/metadata` enriches raw process data with service names (inferred from command lines and runtime-specific heuristics) and language/container context forwarded to WorkloadMeta, powering Service Discovery and APM auto-instrumentation tagging.

# pkg/process/metadata

## Purpose

`pkg/process/metadata` provides the infrastructure for enriching raw process data (collected by `procutil.Probe`) with higher-level metadata: **service names** inferred from command-line arguments and runtime-specific heuristics, and **language/container context** forwarded to WorkloadMeta. This metadata powers Service Discovery, APM auto-instrumentation tagging, and the `process_context` tag that appears on process-level metrics.

The package is structured in three layers:

```
pkg/process/metadata/
├── metadata.go          — Extractor interface
├── parser/              — Service name inference from command lines
│   ├── java/            — Spring Boot jar introspection
│   └── nodejs/          — package.json name lookup
└── workloadmeta/        — Language detection + WorkloadMeta integration
    └── collector/       — Standalone process collector (language detection only)
```

## Key elements

### Key interfaces

#### `metadata.go` — core interface

| Symbol | Kind | Description |
|--------|------|-------------|
| `Extractor` | `interface` | Single method `Extract(procs map[int32]*procutil.Process)`. All extractors in this package implement it. |

### Key types

#### `parser/` — service name extraction

The main type is `ServiceExtractor`. It is created once per `ProcessCheck` and called on every collection cycle.

| Symbol | Kind | Description |
|--------|------|-------------|
| `ServiceExtractor` | `struct` | Implements `metadata.Extractor`. Infers a `process_context:<name>` tag per PID. Thread-safe (uses `sync.RWMutex`). |
| `NewServiceExtractor(enabled, useWindowsServiceName, useImprovedAlgorithm bool) *ServiceExtractor` | constructor | `enabled` gates all processing. `useWindowsServiceName` enables SCM lookup on Windows. `useImprovedAlgorithm` activates deeper heuristics (Spring Boot, Node.js `package.json`, dotnet DLL names, Python module stripping). |
| `(*ServiceExtractor).Extract(procs map[int32]*procutil.Process)` | method | Batch-updates the internal PID→service cache. Skips PIDs whose command line has not changed. |
| `(*ServiceExtractor).ExtractSingle(proc *procutil.Process)` | method | Single-process variant used by the network sender. |
| `(*ServiceExtractor).GetServiceContext(pid int32) []string` | method | Returns the `["process_context:<name>"]` slice for a PID, or `nil`. On Windows with `useWindowsServiceName`, queries the SCM first. |
| `(*ServiceExtractor).Remove(pid int32)` | method | Evicts a PID from the cache (called when a process exits). |
| `ChooseServiceNameFromEnvs(envs []string) (string, bool)` | function | Reads `DD_SERVICE` or `service:` inside `DD_TAGS` from inline environment variables at the start of a command line. |
| `WindowsServiceInfo` | `struct` | Holds `ServiceName []string` and `DisplayName []string` from the Windows SCM for a given PID. Used when multiple services share one `svchost.exe`. |

**Per-runtime heuristics** (all active by default unless noted):

| Runtime | Strategy |
|---------|----------|
| Python | First non-flag argument; strips file extension when `useImprovedAlgorithm` is set. |
| Java | Checks `-Ddd.service=` first; then parses `-jar` name (strips version/SNAPSHOT suffix); introspects Spring Boot `spring.application.name` when `useImprovedAlgorithm` is set. |
| Node.js | Walks up the directory tree from the `.js` entry point to find the nearest `package.json` and reads its `name` field (requires `useImprovedAlgorithm`). |
| .NET | Strips `.dll` suffix from the first non-flag argument (requires `useImprovedAlgorithm`). |
| Ruby, sudo | First non-flag, non-env argument. |
| Fallback | Binary name with file extension stripped. |

#### `parser/java/` — Spring Boot introspection

`javaparser.GetSpringBootAppName(cwd, jarname, args) (string, error)` opens the jar as a ZIP archive, reads `BOOT-INF/classes/` to locate Spring property files, and returns the value of `spring.application.name`. Called by `advancedGuessJavaServiceName` only when `useImprovedAlgorithm` is enabled.

#### `parser/nodejs/`

`nodejsparser.FindNameFromNearestPackageJSON(absFilePath string) (string, bool)` walks parent directories from a `.js` file until it finds a `package.json` containing a `name` field.

### Key functions

#### `workloadmeta/` — language detection and WorkloadMeta integration

| Symbol | Kind | Description |
|--------|------|-------------|
| `WorkloadMetaExtractor` | `struct` | Implements `metadata.Extractor`. Detects the runtime language of new processes, tracks process lifecycle, and publishes diffs over a channel. |
| `NewWorkloadMetaExtractor(sysprobeConfig) *WorkloadMetaExtractor` | constructor | Creates a new extractor with a buffered diff channel (capacity 1). |
| `GetSharedWorkloadMetaExtractor(sysprobeConfig) *WorkloadMetaExtractor` | function | Returns a singleton instance (initialised once via `sync.Once`). Used in the process check path. |
| `(*WorkloadMetaExtractor).Extract(procs map[int32]*procutil.Process)` | method | Diffs the current process snapshot against the internal cache, runs language detection on new PIDs, and sends a `ProcessCacheDiff` to the channel. Drops stale diffs; does not block the caller. |
| `(*WorkloadMetaExtractor).SetLastPidToCid(map[int]string)` | method | Provides the latest PID→container-ID mapping so entities can be enriched with `ContainerId`. |
| `(*WorkloadMetaExtractor).GetAllProcessEntities() (map[string]*ProcessEntity, int32)` | method | Returns a snapshot of the full cache plus its version number. Used at gRPC client startup for a complete initial sync. |
| `(*WorkloadMetaExtractor).ProcessCacheDiff() <-chan *ProcessCacheDiff` | method | Exposes the diff channel for consumers (the gRPC server). |
| `ProcessEntity` | `struct` | `{Pid, ContainerId, NsPid, CreationTime, Language}`. Represents a single process in the WorkloadMeta store. |
| `ProcessCacheDiff` | `struct` | `{Creation []*ProcessEntity, Deletion []*ProcessEntity, cacheVersion int32}`. Sent on every Extract cycle that changes state. |
| `GRPCServer` | `struct` | Wraps the extractor and streams `ProcessCacheDiff` updates to the core agent over gRPC (`ProcessEntityStream` proto service). Supports a single connected client at a time; rejects duplicate connections with `ErrDuplicateConnection`. |
| `NewGRPCServer(config, extractor, tlsConfig) *GRPCServer` | constructor | Creates a gRPC server, optionally with TLS. |
| `Enabled(ddconfig) bool` | function | Returns `language_detection.enabled` from config; always `false` on macOS. |

#### `workloadmeta/collector/`

`Collector` is a lightweight alternative to the full `ProcessCheck`. It is used when only language detection is needed (process check is disabled). It runs `WorkloadMetaExtractor.Extract` on a configurable ticker and publishes results to WorkloadMeta via the gRPC server.

## Usage

### In the process check

`pkg/process/checks.ProcessCheck` wires both extractors together:

```go
// NewProcessCheck (simplified)
serviceExtractor = parser.NewServiceExtractor(enabled, useWindowsServiceName, useImprovedAlgorithm)
wlmExtractor    = workloadmeta.GetSharedWorkloadMetaExtractor(sysprobeConfig)

// On every collection cycle:
serviceExtractor.Extract(procs)    // updates PID→service cache
wlmExtractor.Extract(procs)        // detects languages, publishes diffs
```

`GetServiceContext(pid)` is called when building process payloads to attach the `process_context` tag.

### In the network sender

`pkg/network/sender.ServiceExtractor` calls `ExtractSingle` for individual processes encountered in network connection data, avoiding a full scan.

### Language detection pipeline

The `GRPCServer` started by `Collector` streams `ProcessEntity` diffs to the core agent's WorkloadMeta store, which then propagates language labels to the tagger and APM components.

### Configuration and build flags

## Configuration

| Key | Type | Description |
|-----|------|-------------|
| `language_detection.enabled` | bool | Enables `WorkloadMetaExtractor`. |
| `system_probe_config.process_service_inference.use_windows_service_name` | bool | Windows SCM lookup in `ServiceExtractor`. |
| `system_probe_config.process_service_inference.use_improved_algorithm` | bool | Activates deeper per-runtime heuristics. |
