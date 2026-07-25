# Flare

-----

A flare is a zip archive of everything Datadog support needs to troubleshoot an Agent: logs, effective configuration, status output, diagnose results, runtime state dumps, and optionally pprof profiles — all scrubbed of credentials before they leave the host. The flare component ([`comp/core/flare`](<<<SRC>>>/comp/core/flare/component.go)) assembles the archive by fanning out to every component that registered a flare provider, then uploads it to Datadog's dedicated flare intake. This page covers the architecture: how the archive is built, what goes into it, the scrubbing guarantees, and the four trigger paths. For how to *add* data to flares from your component, see [the flare shared-feature guide](../components/shared_features/flares.md).

## Key packages and files

| Path | Purpose |
|---|---|
| [`comp/core/flare/component.go`](<<<SRC>>>/comp/core/flare/component.go), [`flare.go`](<<<SRC>>>/comp/core/flare/flare.go) | Component interface (`Create`, `CreateWithArgs`, `Send`), the `POST /agent/flare` endpoint, the remote-config task listener |
| [`comp/core/flare/types/types.go`](<<<SRC>>>/comp/core/flare/types/types.go) | `FlareCallback`, `FlareFiller`, `NewProvider`/`NewProviderWithTimeout`, Fx group `flare` |
| [`comp/core/flare/providers.go`](<<<SRC>>>/comp/core/flare/providers.go) | Built-in fillers: Agent log files and `conf.d` config files |
| [`comp/core/flare/helpers/builder.go`](<<<SRC>>>/comp/core/flare/helpers/builder.go) | `FlareBuilder`: staging dir, scrubbers, `permissions.log`, `non_scrubbed_files.json`, zip and permission hardening |
| [`comp/core/flare/helpers/send_flare.go`](<<<SRC>>>/comp/core/flare/helpers/send_flare.go) | Multipart upload to the flare intake, redirect resolution, retries |
| [`pkg/flare/archive.go`](<<<SRC>>>/pkg/flare/archive.go) | Legacy fillers (`ExtraFlareProviders`, frozen): expvars, env vars, endpoint DNS resolution, process-agent and system-probe data, container metadata |
| [`pkg/flare/archive_win.go`](<<<SRC>>>/pkg/flare/archive_win.go), [`archive_linux.go`](<<<SRC>>>/pkg/flare/archive_linux.go) | Platform-specific content: Windows event logs and perf counters; Linux conntrack, SELinux, BTF |
| [`cmd/agent/subcommands/flare/command.go`](<<<SRC>>>/cmd/agent/subcommands/flare/command.go) | The `agent flare` CLI: remote-vs-local decision, profiling capture, upload confirmation |
| [`comp/core/profiler/impl/profiler.go`](<<<SRC>>>/comp/core/profiler/impl/profiler.go) | Collects pprof data from every Agent process for `--profile` |
| [`pkg/util/scrubber/default.go`](<<<SRC>>>/pkg/util/scrubber/default.go) | The shared redaction engine and its default replacers |
| [`pkg/cli/subcommands/dcaflare/command.go`](<<<SRC>>>/pkg/cli/subcommands/dcaflare/command.go) | `datadog-cluster-agent flare`, targeting the Cluster Agent API on port 5005 |

## Two ways a flare is built

```text
                agent flare [caseID]
                       |
        POST https://localhost:5001/agent/flare
                       |
          +------------+------------+
          | IPC OK                  | IPC fails or --local
          v                         v
   RUNNING AGENT builds        CLI PROCESS builds
   the archive in-process      the archive itself
   (live status, running       (adds a `local` marker file
   checks, config-check,       with the IPC error; runs the
   runtime state are real)     local diagnose suite; status
          |                    reads become placeholders)
          +------------+------------+
                       |
        zip path returned / created on disk
                       |
        CLI asks for confirmation, then flare.Send
                       |
   POST https://<version>-flare.agent.<site>/support/flare
```

**Remote (the normal path)**: `agent flare` POSTs to `/agent/flare` on the CMD API server ([`requestArchive`](<<<SRC>>>/cmd/agent/subcommands/flare/command.go)). The *running Agent* builds the archive inside its own process, which is the whole point: in-memory state such as the status page, autodiscovery-resolved configs, and running-check statistics reflect the actual daemon. The server returns the path of the finished zip on local disk, and the CLI takes over for the upload.

**Local (the fallback)**: if the IPC call fails — Agent not running, bad auth token, wrong port — or if `--local` is passed, the CLI process builds the flare itself. Local flares include a `local` marker file recording the IPC error, run the *local* diagnose suite in the CLI process (bounded by a 60 s timeout because Python/CGo initialization can hang; see [Diagnostics](diagnostics.md#agent-diagnose)), and record "unable to get status, is it running?" placeholders where live state would be. Local flares do include one thing remote flares cannot: a copy of the remote-config database, since the CLI can read the file directly while the daemon holds it open.

## The provider model

Any component can contribute files by returning a `flaretypes.Provider` (Fx value group `flare`) wrapping a callback `func(fb flaretypes.FlareBuilder) error` ([`types/types.go`](<<<SRC>>>/comp/core/flare/types/types.go)). When a flare is created, [`flare.go`](<<<SRC>>>/comp/core/flare/flare.go) runs every provider **sequentially**, each in a goroutine raced against a timeout: `flare_provider_timeout` (default 10 s), overridable per request with the `?provider_timeout=` query parameter and per provider with `NewProviderWithTimeout`. A timed-out provider's goroutine is *not* killed — its context is cancelled and the flare moves on, so a badly behaved provider leaks a goroutine in the Agent process. All provider errors and timeouts are written to `flare_creation.log` inside the archive itself, so a partially failed flare still ships with a record of what is missing.

Roughly thirty components provide data this way: configuration (`runtime_config_dump.yaml`), the [tagger](../containers/tagger.md) and [workloadmeta](../containers/workloadmeta.md) dumps, the [logs agent](../pipelines/logs.md), [secrets](../configuration/secrets.md), [autodiscovery](../checks/autodiscovery.md) (`config-check.log`), internal telemetry (`telemetry.log`), the health probe (`health.yaml`), diagnose results (`diagnose.log`), the status page (`status.log`), metadata inventories, the remote-config service, and more. Remote agents registered through the [remote agent registry](introspection.md#the-status-system) contribute files over gRPC.

### Legacy fillers

[`pkg/flare/archive.go`](<<<SRC>>>/pkg/flare/archive.go) predates the component system and holds `ExtraFlareProviders`, an explicitly **frozen** list — new flare data must be a component provider, both for ownership and because reaching into `pkg/flare` from components creates dependency cycles. The legacy fillers still collect a lot: the expvar dump (`expvar/` directory), `envvars.log` (allow-listed environment variables only), `connectivity/resolved_endpoints.txt` (DNS resolution of intake endpoints, which reveals PrivateLink setups), the process-agent's runtime config and check output fetched over its IPC port (6162, unless process checks run in the core agent), [system-probe](../ebpf/system-probe.md) expvars/telemetry/config fetched over its socket, `registry.json` (logs tailing offsets), `install_info.log` and `version-history.json`, and container runtime metadata (`docker_ps.log`, `docker_inspect.log`, kubelet config and pod list, `ecs_metadata.json`).

### Platform-specific content

[`archive_win.go`](<<<SRC>>>/pkg/flare/archive_win.go) adds Windows material: `typeperf`/`lodctr` output and the perf-counter registry strings (for broken-counter diagnosis), exported Windows event logs (Application, System, and `Microsoft-Windows-*` channels), Datadog service status, Datadog registry keys, and IIS application pool state. [`archive_linux.go`](<<<SRC>>>/pkg/flare/archive_linux.go) adds conntrack cached/host tables, eBPF BTF loader info, SELinux `sestatus` and `semodule -l` output, Linux audit logs, and service-discovery state.

## Staging, scrubbing, and packaging

The [`FlareBuilder`](<<<SRC>>>/comp/core/flare/helpers/builder.go) stages everything in a private temp directory (`<tmpdir>/<hostname>/...`) whose permissions are stripped of other-user access on both POSIX (mode bits) and Windows (ACLs). Files enter through a small API with strong defaults:

1. `AddFile`/`CopyFile` pass content through **two scrubbers**: the general default replacers from [`pkg/util/scrubber`](<<<SRC>>>/pkg/util/scrubber/default.go) — API keys (keeping the last 5 characters), app keys, bearer tokens, passwords, `token`/`jwt`-like YAML keys, SNMP credentials, PEM certificate blocks, URL-embedded credentials — plus a flare-specific extra replacer that redacts *any* `api_key: ...` occurrence, catching third-party configs (for example PowerDNS) that the generic patterns miss.
1. Files whose name contains `.yaml` go through the YAML-aware scrubber with `SetPreserveENC(true)`, so `ENC[...]` [secret handles](../configuration/secrets.md) survive scrubbing — support can see that a value is managed by a secrets backend without seeing the value.
1. `AddFileWithoutScrubbing` and `CopyDirToWithoutScrubbing` exist for content where scrubbing would be destructive (pprof binary profiles) or redundant (Agent log files, which the logging layer already scrubbed at write time). Every file that bypasses scrubbing is recorded in `non_scrubbed_files.json` for auditability.
1. `permissions.log` records the original owner and mode of every file copied from disk, which frequently diagnoses "Agent cannot read its own config" issues.

`Save()` zips the staging directory, restricts the zip's permissions, and moves it to the OS temp directory. The archive name embeds the UTC timestamp and the log level active at creation time: `datadog-agent-<timestamp>-<loglevel>.zip` — the filename itself tells support whether debug logs are inside.

## Upload

[`send_flare.go`](<<<SRC>>>/comp/core/flare/helpers/send_flare.go) first issues a HEAD request to resolve any redirect, then streams a multipart POST containing the zip (`flare_file`) plus `case_id`, `email`, `source`, `rc_task_uuid`, `hostname`, and `agent_version` fields, with the `DD-API-KEY` header, to the version-prefixed flare domain: `https://<agent-version>-flare.agent.<site>/support/flare[/<caseID>]`. Transient failures (5xx, network errors) are retried — up to 3 attempts, one second apart. On success the local zip is deleted unless `--keep-archive` was passed. The CLI always shows the archive path and asks for confirmation before uploading (unless `--send`), so an operator can inspect the zip first.

## Triggers

There are four ways a flare gets made:

1. **CLI**: `agent flare [caseID] [--email ...] [--send] [--local] [--profile N] [--with-stream-logs D] [--provider-timeout D] [--keep-archive]`.
1. **GUI**: the [web GUI](diagnostics.md#the-web-gui) has a flare form that builds and submits through the running Agent.
1. **Remote configuration**: the Datadog backend can request a flare from the Fleet page. The flare component subscribes to the `AGENT_TASK` product ([remote config](../configuration/remote-config.md)); `onAgentTaskEvent` handles the `flare` task, optionally capturing profiles (`flare.rc_profiling.profile_duration`, default 30 s) and streaming logs (`flare.rc_streamlogs.duration`, default 60 s) before uploading with the task's `rc_task_uuid` attached.
1. **API**: any authenticated client can `POST /agent/flare` and handle the resulting archive itself.

### Profiling flares

`agent flare --profile N` (N ≥ 30 seconds) turns a flare into a performance investigation bundle. The [profiler component](<<<SRC>>>/comp/core/profiler/impl/profiler.go) collects, from **every** enabled process — core agent on the expvar port, process-agent (6062), security-agent (5011), trace-agent (HTTPS debug port 5012), and system-probe over its socket — two heap snapshots (before/after), an N-second CPU profile, mutex and block profiles, and an execution trace. Because mutex/block profiling is off by default, the CLI temporarily flips `runtime_mutex_profile_fraction` and `runtime_block_profile_rate` through the [runtime settings API](../configuration/runtime-settings.md) (`ExecWithRuntimeProfilingSettings`) and restores the previous values afterwards. `--with-stream-logs <duration>` similarly runs a log stream capture first and bundles the output.

### Other binaries

The [Cluster Agent](../containers/cluster-agent.md) has its own `datadog-cluster-agent flare` ([`pkg/cli/subcommands/dcaflare`](<<<SRC>>>/pkg/cli/subcommands/dcaflare/command.go)) hitting `https://localhost:5005/flare` with the same provider architecture; the security-agent likewise ships a `flare` subcommand. In Kubernetes, `kubectl exec` into the agent container and running `agent flare` uses the normal remote path since the daemon is reachable over localhost.

## Configuration

| Key | Default | Effect |
|---|---|---|
| `flare_provider_timeout` | 10 s | Per-provider time budget during archive creation |
| `flare.rc_profiling.profile_duration` | 30 s | CPU profile length for remote-config-triggered flares |
| `flare.rc_profiling.blocking_rate`, `flare.rc_profiling.mutex_fraction` | 0 | Contention profiling for RC flares |
| `flare.rc_streamlogs.duration` | 60 s | Stream-logs capture length for RC flares |
| `flare.profile_overhead_runtime` | 10 s | Extra provider budget when profiling is requested |
| `server_timeout` | 30 s | CMD API connection deadline — the flare handler explicitly resets it, since flares routinely take longer |

## Gotchas

1. **Local and remote flares differ materially.** Local flares lack live daemon state (status, config-check, running checks) but include the remote-config DB copy and a fresh local diagnose run; remote flares are the inverse. The `local` marker file tells you which kind you are looking at, and why.
1. **Log files are not re-scrubbed.** They are copied with `CopyDirToWithoutScrubbing` on the assumption the logging layer scrubbed at write time. If a component logs a credential through a path the log scrubber does not cover, it ends up in the flare; `non_scrubbed_files.json` is the audit trail.
1. **Provider timeouts leak goroutines.** The raced goroutine is abandoned, not killed. A provider that ignores its context and blocks forever costs the Agent a goroutine per flare.
1. The flare filename intentionally leaks the log level (`datadog-agent-<ts>-<loglevel>.zip`) so support can triage without opening the archive.
1. `ENC[...]` secret handles are preserved only by the flare's YAML scrubbing path — other scrub consumers (status CLI, settings API) redact them like any other value.
1. New flare content must be a component `flaretypes.Provider`; `ExtraFlareProviders` in `pkg/flare` is frozen, and adding to it tends to create import cycles with components.
