# pkg/flare

## Purpose

`pkg/flare` contains the logic for building diagnostic **flare archives** — compressed support bundles that operators send to Datadog Support when troubleshooting an agent issue. A flare collects configuration files, log files, runtime state (expvars, goroutine dumps, health checks, telemetry), environment variables, system-probe diagnostics, container/Kubernetes metadata, and pprof profiles into a single ZIP that is uploaded to a Datadog support case.

The package does **not** own the ZIP-building machinery itself; that lives in `comp/core/flare/helpers` and `comp/core/flare/builder`. What `pkg/flare` provides is the concrete collection functions ("providers") that decide what data goes into the archive, plus entry points used by the agent CLI and the flare component.

---

## Package Layout

| Sub-package | Responsibility |
|---|---|
| `pkg/flare` (root) | Core-agent providers: config dumps, system-probe stats, workload/tagger lists, process-agent checks, goroutine dump, ECS metadata, remote config, version history |
| `pkg/flare/common` | Shared helpers used across all agent flavors: `GetConfigFiles`, `GetLogFiles`, `GetExpVar`, `GetEnvVars` |
| `pkg/flare/clusteragent` | Cluster Agent flare: `CreateDCAArchive`, Kubernetes metadata map, cluster checks, HPA/custom metrics, autoscaler lists |
| `pkg/flare/securityagent` | Security Agent flare: `CreateSecurityAgentArchive`, compliance policy files, runtime security policy files |
| `pkg/flare/priviledged` | Tiny helpers for system-probe and security-agent that need elevated access: `GetSystemProbeSocketPath`, `GetHTTPData` |

Platform-specific files follow the standard Go build-tag naming (`_linux`, `_win`, `_nix`, `_nodocker`, etc.).

---

## Key Elements

### Root package

**`RemoteFlareProvider`** — wraps an `ipc.Component` (the secure IPC client) and provides methods that make authenticated HTTP calls to the running agent process:

- `GetTaggerList(url)` — fetches and pretty-prints the agent's tagger entity list.
- `GetWorkloadList(url)` — fetches and formats the workload-metadata dump.
- `GetGoRoutineDump()` — hits `/debug/pprof/goroutine?debug=2` on the expvar port.

**`ExtraFlareProviders(workloadmeta, ipc) []*FlareFiller`** — the main registration function called by `comp/core/flare/flare.go` to attach all non-componentized providers to the archive. This is the legacy entry point; new data should be added via the `types.NewProvider` FX mechanism instead.

**`SendFlare(cfg, archivePath, caseID, email, source)`** — deprecated thin wrapper around `helpers.SendTo`; prefer the `Send` method on the flare component.

**`PrintConfigCheck(w, cr, withDebug)`** — renders a `ConfigCheckResponse` for the `config-check.log` file in the archive.

### common

**`GetConfigFiles(fb, confSearchPaths)`** — copies `.yaml`/`.yml` check configs from `confd_path` and the dist `conf.d` directory plus the main `datadog.yaml`, `system-probe.yaml`, and `security-agent.yaml`.

**`GetLogFiles(fb, logFileDir)`** — flushes the logger and copies `*.log` files from the log directory without scrubbing (log scrubbing happens at the builder level).

**`GetExpVar(fb)`** — dumps all Go `expvar` values as YAML and also queries the trace-agent's `/debug/vars` endpoint.

**`GetEnvVars()`** — returns the subset of environment variables on an allowlist (`allowedEnvvarNames`). Sensitive vars containing `_KEY` or `_AUTH_TOKEN` in their name are never included regardless of the allowlist.

### clusteragent

**`CreateDCAArchive(local, distPath, logFilePath, pdata, statusComponent, diagnose, ipc) (string, error)`** — top-level function that builds and saves the cluster-agent archive. It assembles config files, logs, cluster-checks metadata, agent/cluster-agent DaemonSet/Deployment YAML manifests, Helm values, the Datadog CRD, HPA status, autoscaler list, tagger list, workload list, performance profiles, and the runtime config dump.

**`GetClusterAgentConfigCheck(w, withDebug, client)`** — queries the cluster-agent IPC port for its config check and writes the result.

**`QueryDCAMetrics()`** — fetches the Prometheus metrics endpoint exposed by the cluster agent.

### securityagent

**`CreateSecurityAgentArchive(local, logFilePath, statusComponent) (string, error)`** — builds the security-agent archive including the agent status, logs, configs, compliance rule files (`compliance_config.dir`), and runtime security policies (`runtime_security_config.policies.dir`).

### priviledged

**`GetSystemProbeSocketPath() string`** — reads `system_probe_config.sysprobe_socket` from the system-probe config; used by the root package when querying system-probe over its Unix socket.

**`GetHTTPData(client, url) ([]byte, error)`** — generic HTTP GET helper shared by system-probe and security-agent callers.

### Types (comp/core/flare/types)

Components contribute data to flares via the **provider pattern**:

```go
// Register a provider via FX
flaretypes.NewProvider(func(fb flaretypes.FlareBuilder) error {
    fb.AddFileFromFunc("my-data.log", collectMyData)
    return nil
})
```

- **`FlareBuilder`** — the interface for adding files: `AddFile`, `AddFileFromFunc`, `CopyFileTo`, `CopyDirTo`, `AddFileWithoutScrubbing`, `RegisterFilePerm`, `Logf`, `IsLocal`.
- **`FlareFiller`** — pairs a `FlareCallback` with an optional `FlareTimeout`; used internally by the flare component to run each provider with a deadline.
- **`Provider`** — FX-tagged wrapper (`group:"flare"`) for contributing a `FlareFiller` via dependency injection.
- **`FlareArgs`** — optional arguments passed by the caller (profiling duration/rates, stream-logs duration).

---

## Usage

### How a flare is triggered

1. **CLI** — `datadog-agent flare <case-id>` calls the agent's `/flare` HTTP endpoint (IPC).
2. **Remote Config** — a `TaskFlare` Remote Config task triggers `flare.CreateWithArgs` directly inside the agent process.
3. **Local fallback** — if the agent is unreachable, `NewFlareBuilder(local=true, …)` creates a minimal archive from whatever is readable on disk.

### How the flare component assembles the archive

`comp/core/flare/flare.go` (`newFlare`) collects all `[]*types.FlareFiller` registered under the `"flare"` FX group, appends the legacy providers from `ExtraFlareProviders`, and runs each with a configurable timeout (`flare_provider_timeout`). Each provider writes into the shared `FlareBuilder`; at the end `fb.Save()` produces the ZIP.

### Adding data to a flare

To add a new file to core-agent flares, create a component and register a provider:

```go
// In your component's provide function:
return flaretypes.NewProvider(func(fb flaretypes.FlareBuilder) error {
    return fb.AddFileFromFunc("my-component.log", myCollectFunc)
})
```

For the cluster agent use `CreateDCAArchive` and extend `createDCAArchive` directly (or use the same provider FX mechanism if your component is already wired into the cluster agent's FX app).

### Security note

The `FlareBuilder` scrubs known secret patterns automatically. However, the recommendation in `archive.go` is to avoid capturing secret-containing data in the first place, not to rely solely on scrubbing.

---

## Related packages

| Package | Relationship |
|---|---|
| [`comp/core/flare`](../../comp/core/flare.md) | The fx component that orchestrates archive creation and upload. `pkg/flare.ExtraFlareProviders` is the legacy registration entry point invoked by `newFlare` inside the component. New data should be contributed via `flaretypes.NewProvider` in the component's FX graph instead. |
| [`pkg/util/scrubber`](util/scrubber.md) | All content added to the archive via `FlareBuilder.AddFile` / `AddFileFromFunc` / `CopyFileTo` is automatically processed by the scrubber. The builder uses `SetPreserveENC(true)` and `SetShouldApply(...)` to skip replacers newer than the flare's agent version, avoiding false-positive redactions on older archives. |
| [`pkg/util/archive`](util/archive.md) | `comp/core/flare/helpers/builder.go` calls `archive.Zip` to package the collected diagnostic directory into the final `.zip` file that is uploaded to Datadog Support. All path-traversal and symlink mitigations in `archive.Zip` apply to flare archives. |
