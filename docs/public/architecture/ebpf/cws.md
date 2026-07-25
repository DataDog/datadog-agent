# Workload Protection (CWS)

-----

Workload Protection — historically Cloud Workload Security (CWS), also called runtime security — detects and optionally blocks suspicious activity on hosts and in containers at runtime: file access, process execution, kernel module loads, DNS queries, privilege changes, and so on. On Linux it observes the kernel through eBPF programs, on Windows through ETW sessions, and on Fargate through a ptrace-based tracer injected into the workload. Observed events are enriched, evaluated against SECL rules, and matches are shipped as JSON security events to a dedicated Datadog intake.

Almost all of the implementation lives in [`pkg/security`](<<<SRC>>>/pkg/security), but it executes across three processes: [system-probe](system-probe.md) does the privileged collection and rule evaluation, the **security-agent** binary is a mostly-unprivileged forwarder that pulls matched events from system-probe over gRPC and posts them to the intake, and the core agent contributes shared services (remote [tagger](../containers/tagger.md)/[workloadmeta](../containers/workloadmeta.md), the [Remote Config](../configuration/remote-config.md) backend). File Integrity Monitoring (FIM) is the same machinery restricted to file rules (`runtime_security_config.fim_enabled`). The compliance and SBOM products that share the security-agent binary are covered in [Compliance and SBOM](compliance.md).

## Process split

```text
            core agent                        system-probe (root)
  +---------------------------+     +--------------------------------------+
  | Remote Config service     |<----| event_monitor module                 |
  | workloadmeta / tagger     |gRPC | +----------------------------------+ |
  | (gRPC servers, port 5001) |     | | eBPF probe / ETW / ebpfless      | |
  +---------------------------+     | | resolvers (path, process, tags)  | |
                                    | | SECL rule engine + policies      | |
            security-agent          | | activity dumps / profiles        | |
  +---------------------------+     | +----------------------------------+ |
  | RuntimeSecurityAgent      |     | APIServer (gRPC, unix socket)        |
  |   GetEventStream ---------+---->|   runtime-security.sock              |
  |   GetActivityDumpStream --+---->|                                      |
  | CWSReporter (logs sender) |     +--------------------------------------+
  +------------+--------------+
               |
               v HTTPS
   runtime-security-http-intake.logs.<site>   (track: secruntime)
   cws-intake.<site>                          (track: secdump, activity dumps)
```

1. **system-probe** loads the `event_monitor` module ([`cmd/system-probe/modules/eventmonitor.go`](<<<SRC>>>/cmd/system-probe/modules/eventmonitor.go)) when `runtime_security_config.enabled`, `runtime_security_config.fim_enabled`, `sbom.enrichment.usage.enabled`, or another event-stream consumer (NPM, USM, GPU) needs it — the enablement matrix is in [`pkg/system-probe/config/config.go`](<<<SRC>>>/pkg/system-probe/config/config.go). It hosts the probe, resolvers, rule engine, activity dumps, and self tests.
1. **security-agent** ([`cmd/security-agent`](<<<SRC>>>/cmd/security-agent)) starts the CWS forwarder ([`pkg/security/agent/start.go`](<<<SRC>>>/pkg/security/agent/start.go)) when `runtime_security_config.enabled` is set in *its* config (`datadog.yaml`/`security-agent.yaml`), and independently the compliance agent when `compliance_config.enabled` is set. If both sub-products are disabled, `RunAgent` exits with `ErrAllComponentsDisabled` ([`cmd/security-agent/subcommands/start/command.go`](<<<SRC>>>/cmd/security-agent/subcommands/start/command.go)).
1. **core agent** serves Remote Config (system-probe's policy client connects to its gRPC `cmd_port` 5001) and remote workloadmeta/tagger consumed by both other processes.
1. **cluster-agent** is involved only for Fargate-on-EKS, where its [admission controller](../containers/admission-controller.md) injects the `cws-instrumentation` tracer into pods.

The generic event-monitor layer ([`pkg/eventmonitor`](<<<SRC>>>/pkg/eventmonitor)) wraps the probe and fans events out to registered `EventConsumer`s. CWS is one consumer among several (process lifecycle for NPM, USM, GPU), but it is privileged: non-CWS consumers may only receive Fork/Exec/Exit/TracerMemfdSeal events (`allowedEventTypes` in [`eventmonitor.go`](<<<SRC>>>/pkg/eventmonitor/eventmonitor.go)); every other event type is exclusively for the rule engine.

## Key packages and files

| Path | Purpose |
|---|---|
| [`pkg/security/probe`](<<<SRC>>>/pkg/security/probe) | Platform probes: [`probe_ebpf.go`](<<<SRC>>>/pkg/security/probe/probe_ebpf.go) (Linux), [`probe_ebpfless.go`](<<<SRC>>>/pkg/security/probe/probe_ebpfless.go) (ptrace/Fargate), [`probe_windows.go`](<<<SRC>>>/pkg/security/probe/probe_windows.go) (ETW) |
| [`pkg/security/ebpf/c/include/hooks`](<<<SRC>>>/pkg/security/ebpf/c/include/hooks) | The eBPF programs: one header per hooked operation (`open.h`, `exec.h`, `ptrace.h`, `module.h`, `iouring.h`, ...) |
| [`pkg/security/probe/kfilters`](<<<SRC>>>/pkg/security/probe/kfilters) | Approvers: in-kernel allow-lists computed from the ruleset |
| [`pkg/security/probe/discarders_linux.go`](<<<SRC>>>/pkg/security/probe/discarders_linux.go) | Discarders: in-kernel deny-entries pushed back from user space |
| [`pkg/security/probe/erpc`](<<<SRC>>>/pkg/security/probe/erpc) | eRPC: userspace-to-kernel request channel (dentry resolution, discarder push) |
| [`pkg/security/probe/eventstream`](<<<SRC>>>/pkg/security/probe/eventstream) | Ring-buffer / perf-map event transport with reordering |
| [`pkg/security/resolvers`](<<<SRC>>>/pkg/security/resolvers) | Enrichment: dentry→path, mount, process tree, cgroup/container, user/group, hash, SBOM, netns, SELinux, user sessions |
| [`pkg/security/secl`](<<<SRC>>>/pkg/security/secl) | SECL (separate Go module): [`compiler/eval`](<<<SRC>>>/pkg/security/secl/compiler/eval/eval.go), [`model`](<<<SRC>>>/pkg/security/secl/model) (event model, generated accessors), [`rules`](<<<SRC>>>/pkg/security/secl/rules) (policies, loader, approver computation) |
| [`pkg/security/seclwin`](<<<SRC>>>/pkg/security/seclwin) | Auto-generated Windows-only copy of SECL — never edit by hand |
| [`pkg/security/rules/engine.go`](<<<SRC>>>/pkg/security/rules/engine.go) | `RuleEngine`: policy providers, load/reload, heartbeats, `ApplyRuleSet` |
| [`pkg/security/rules/bundled`](<<<SRC>>>/pkg/security/rules/bundled) | Bundled internal policy (`refresh_user_cache`, `refresh_sbom` rules) |
| [`pkg/security/rconfig/policies.go`](<<<SRC>>>/pkg/security/rconfig/policies.go) | Remote Config policy provider (`CWS_DD`, `CWS_CUSTOM`, `CWS_REMEDIATION`) |
| [`pkg/security/module`](<<<SRC>>>/pkg/security/module) | CWS consumer in system-probe: [`cws.go`](<<<SRC>>>/pkg/security/module/cws.go) (`CWSConsumer`), [`server.go`](<<<SRC>>>/pkg/security/module/server.go) (`APIServer`, retention queue), [`msg_sender.go`](<<<SRC>>>/pkg/security/module/msg_sender.go) (direct intake sender) |
| [`pkg/security/agent`](<<<SRC>>>/pkg/security/agent) | `RuntimeSecurityAgent` inside the security-agent process: gRPC client, event dispatch |
| [`pkg/security/reporter/reporter.go`](<<<SRC>>>/pkg/security/reporter/reporter.go) | `CWSReporter`: headless [logs pipeline](../pipelines/logs.md) sender for raw JSON events |
| [`pkg/security/common/logs_context.go`](<<<SRC>>>/pkg/security/common/logs_context.go) | Intake endpoints and track names (`secruntime`, `secinfo`, `compliance`) |
| [`pkg/security/security_profile`](<<<SRC>>>/pkg/security/security_profile) | Activity dumps, security profiles, anomaly detection, storage backends |
| [`pkg/security/ptracer`](<<<SRC>>>/pkg/security/ptracer) + [`cmd/cws-instrumentation`](<<<SRC>>>/cmd/cws-instrumentation) | ebpfless mode: ptrace+seccomp tracer injected into Fargate workloads |
| [`pkg/security/proto/api/api.proto`](<<<SRC>>>/pkg/security/proto/api/api.proto) | gRPC contract between system-probe and security-agent |
| [`pkg/security/config/config.go`](<<<SRC>>>/pkg/security/config/config.go) | `RuntimeSecurityConfig`: parses every `runtime_security_config.*` key |
| [`pkg/security/probe/config/config.go`](<<<SRC>>>/pkg/security/probe/config/config.go) | Probe config: `event_monitoring_config.*` keys |
| [`pkg/config/setup/system_probe_cws.go`](<<<SRC>>>/pkg/config/setup/system_probe_cws.go) | Authoritative list of CWS config defaults |
| [`cmd/system-probe/subcommands/runtime/command.go`](<<<SRC>>>/cmd/system-probe/subcommands/runtime/command.go) | CWS CLI: `system-probe runtime policy check/reload`, `self-test`, `activity-dump`, `security-profile` |
| [`pkg/security/tests`](<<<SRC>>>/pkg/security/tests) | Functional tests (`dda inv security-agent.functional-tests`) |
| [`docs/cloud-workload-security`](<<<SRC>>>/docs/cloud-workload-security) | SECL language reference and backend event JSON schemas |

## The Linux event pipeline

### Kernel: hooks and the event stream

eBPF programs in [`pkg/security/ebpf/c/include/hooks`](<<<SRC>>>/pkg/security/ebpf/c/include/hooks) attach to kernel functions via kprobes by default, or fentry when `event_monitoring_config.event_stream.use_fentry` is enabled (off by default, with kprobe fallback allowed). CO-RE is the default loading strategy; runtime compilation exists behind `event_monitoring_config.runtime_compilation.enabled` but is off. Events are written to the `events` map — a ring buffer when the kernel supports it and `event_monitoring_config.event_stream.use_ring_buffer` is true (the default), otherwise a per-CPU perf map. The perf path goes through a reorderer in [`pkg/security/probe/eventstream`](<<<SRC>>>/pkg/security/probe/eventstream) because CPU-parallel perf events arrive out of order; the ring buffer does not need one.

### In-kernel filtering: approvers, discarders, eRPC

Sending every kernel event to user space would be prohibitively expensive, so CWS filters in the kernel, controlled by `event_monitoring_config.enable_kernel_filters` (with `enable_approvers` and `enable_discarders` toggles):

1. **Approvers** ([`pkg/security/probe/kfilters`](<<<SRC>>>/pkg/security/probe/kfilters)) are per-event-type allow-lists derived from the loaded ruleset when `probe.ApplyRuleSet` runs — for example, if all `open` rules match on a set of basenames or flags, only opens matching them are emitted. The derivation logic lives beside the SECL rules code in [`pkg/security/secl/rules/approvers.go`](<<<SRC>>>/pkg/security/secl/rules/approvers.go).
1. **Discarders** ([`discarders_linux.go`](<<<SRC>>>/pkg/security/probe/discarders_linux.go)) work in the opposite direction: when the rule engine proves at runtime that an object (an inode, a PID) can never match any rule, `RuleEngine.EventDiscarderFound` pushes a deny-entry into a kernel map so future events on that object are dropped at the source. Discarders are flushed progressively over `flush_discarder_window` on every policy reload.
1. **eRPC** ([`pkg/security/probe/erpc`](<<<SRC>>>/pkg/security/probe/erpc)) is the userspace-to-kernel request channel used for both of the above plus dentry path resolution: user space issues an ioctl-like syscall that an eBPF program intercepts and answers from kernel maps.

### User space: decode, resolvers, dispatch

`EBPFProbe.handleEvent` unmarshals the binary event and resolves fields lazily through the field handlers and [`resolvers.EBPFResolvers`](<<<SRC>>>/pkg/security/resolvers/resolvers_ebpf.go): dentry keys become file paths (via eRPC or map walking), cgroup IDs become container IDs, and the process ancestry comes from the `proc_cache`/`pid_cache` eBPF maps mirrored in the user-space `ProcessResolver`. Container tags are resolved through the remote tagger. Resolution is lazy on purpose — a field is only computed if a rule or serializer actually reads it.

`EBPFProbe.DispatchEvent` ([`probe_ebpf.go`](<<<SRC>>>/pkg/security/probe/probe_ebpf.go)) then hands the event to, in order: the security-profile manager (which marks in-profile events and feeds activity trees), the wildcard handlers (the CWS `RuleEngine`), and finally the per-event-type event-monitor consumers (process/network/GPU).

### Rule evaluation and rate limiting

`RuleSet.Evaluate` runs the compiled SECL evaluators ([`pkg/security/secl/compiler/eval`](<<<SRC>>>/pkg/security/secl/compiler/eval/eval.go)). On a match, `RuleEngine.RuleMatch` → `CWSConsumer.SendEvent` → a per-rule-ID token bucket ([`pkg/security/events/rate_limiter.go`](<<<SRC>>>/pkg/security/events/rate_limiter.go), default `event_server.rate` 10/s with burst 40) → `APIServer.SendEvent`. Rate-limited matches are counted in `datadog.runtime_security.rules.rate_limiter.drop` but never logged.

Besides rule matches, the same `EventSender` path carries **custom events** with internal rule IDs ([`pkg/security/events/custom.go`](<<<SRC>>>/pkg/security/events/custom.go)): ruleset-loaded reports, heartbeats (one per minute for five minutes after a reload, then every ten minutes), self-test results, kill-action reports, and anomaly detections.

### Serialization and the tag-retention queue

The `APIServer` ([`pkg/security/module/server.go`](<<<SRC>>>/pkg/security/module/server.go)) serializes events to JSON with the easyjson serializers in [`pkg/security/serializers`](<<<SRC>>>/pkg/security/serializers) and attaches host and container tags. Events whose container tags have not resolved yet sit in a **retention queue** for up to `event_server.retention` (6 s default), retried until the tagger answers; past `event_retry_queue_threshold` they are forced out with whatever tags are available (`missingTagsCount` telemetry). Under sustained burst, the outbound channel has drop-oldest semantics — silently expired messages are counted per rule in `expiredEvents`.

## Transport to intake

Three topologies, selected by configuration:

| Mode | Config | Flow |
|---|---|---|
| Legacy pull (default) | — | system-probe hosts the `SecurityModuleEvent` gRPC service on `runtime_security_config.socket` (`/opt/datadog-agent/run/runtime-security.sock`; `localhost:3335` on Windows). The security-agent's `RuntimeSecurityAgent` opens the `GetEventStream` and `GetActivityDumpStream` server streams ([`pkg/security/agent/client.go`](<<<SRC>>>/pkg/security/agent/client.go)) and forwards each message |
| Reversed push | `runtime_security_config.event_grpc_server: security-agent` | The security-agent hosts `SecurityAgentAPI` (client-streaming `SendEvent`) and system-probe pushes to it ([`remote_event_server.go`](<<<SRC>>>/pkg/security/module/remote_event_server.go)). Setting it to `system-probe` makes system-probe host `SecurityAgentAPI` instead — used with `vsock://` addresses so remote system-probes inside micro-VMs can push their events to the host system-probe, which forwards them onward |
| Direct send | `runtime_security_config.direct_send_from_system_probe: true` | system-probe builds the logs pipeline itself (`DirectEventMsgSender` in [`msg_sender.go`](<<<SRC>>>/pkg/security/module/msg_sender.go)) and posts straight to the intake; the security-agent CWS half does not run at all |

In the non-direct modes, the security-agent side wraps a headless logs pipeline: `CWSReporter` ([`pkg/security/reporter/reporter.go`](<<<SRC>>>/pkg/security/reporter/reporter.go)) points at `runtime-security-http-intake.logs.<site>` with track `secruntime` (or the plain `logs` track if `runtime_security_config.use_secruntime_track: false`). Events tagged with the `secinfo` track (remediation reports) go through a second reporter to the same host. Activity dumps take a different intake entirely: `cws-intake.<site>`, track `secdump`, as multipart uploads ([`storage/backend/forwarder.go`](<<<SRC>>>/pkg/security/security_profile/storage/backend/forwarder.go)). These pipelines reuse the logs sender machinery but do not pass through the logs agent.

## Policies: SECL, providers, Remote Config

Rules are written in SECL (Security Event Correlation Language; see the [in-repo language docs](<<<SRC>>>/docs/cloud-workload-security)) and grouped into policy files. `RuleEngine.gatherDefaultPolicyProviders` ([`engine.go`](<<<SRC>>>/pkg/security/rules/engine.go)) assembles providers in precedence order:

1. **Bundled** ([`pkg/security/rules/bundled`](<<<SRC>>>/pkg/security/rules/bundled)): hardcoded internal rules (`refresh_user_cache`, `refresh_sbom`) plus, when enabled, SBOM-generated policies. Invisible in policy reports unless `policies.monitor.report_internal_policies` is set.
1. **Remote Config** ([`pkg/security/rconfig/policies.go`](<<<SRC>>>/pkg/security/rconfig/policies.go)): active when RC is enabled globally *and* `runtime_security_config.remote_configuration.enabled` (default true). The provider runs its own RC gRPC client against the core agent's `cmd_port`, authenticated with the IPC token, subscribed to the products `CWS_DD` (Datadog-managed default policies), `CWS_CUSTOM` (customer rules), and `CWS_REMEDIATION`. Updates are debounced 5 s, then trigger a full ruleset reload; per-config ACK/error status flows back through RC `ApplyStatus`.
1. **Policies directory** (`runtime_security_config.policies.dir`, default `/etc/datadog-agent/runtime-security.d`): `default.policy` loads as the "default" policy, other files sort alphabetically.

At load time each rule passes through filters: agent-version constraints and SECL `filters:` expressions evaluated against a synthetic host model ([`pkg/security/rules/filtermodel`](<<<SRC>>>/pkg/security/rules/filtermodel)) exposing OS, kernel version, CO-RE support, origin, and hostname. Every load or reload compiles a fresh `RuleSet`, calls `probe.ApplyRuleSet` (recomputing approvers and pushing them to kernel maps), flushes discarders, re-applies rate limiters, and emits a `ruleset_loaded` custom event. Reload triggers: SIGHUP ([`reloader_linux.go`](<<<SRC>>>/pkg/security/module/reloader_linux.go)), an RC update, an SBOM policy update (silent — no `ruleset_loaded` event or heartbeat reset), or `system-probe runtime policy reload` over the cmd socket. Policy state monitoring is reported through [`pkg/security/rules/monitor`](<<<SRC>>>/pkg/security/rules/monitor).

### Enforcement (kill actions)

Rules can carry kill/remediation actions, governed by `runtime_security_config.enforcement.*`. Two gates are easy to miss:

1. Kill actions only fire for rules originating from sources in `enforcement.rule_source_allowed` (default: `file` and `remote-config`).
1. Losing the Remote Config connection **disables enforcement** (`rcStateCallback` → `probe.EnableEnforcement(false)`), re-enabled on reconnect — a deliberate central kill switch, but surprising if you expect kill rules to be autonomous. Per-rule disarmers (`enforcement.disarmer.*`) additionally auto-disarm a rule that kills across too many distinct containers or executables.

### Self tests

On startup (+15 s) and periodically afterwards, CWS performs synthetic actions — create/chmod/chown a temporary file on Linux, create a file and open a registry key on Windows — matched by injected self-test policies ([`pkg/security/probe/selftests`](<<<SRC>>>/pkg/security/probe/selftests)). Results surface as a custom event and the `datadog.runtime_security.self_test` metric; failures retry every 15 s up to 25 times, then hourly. Self tests can be triggered over gRPC (`RunSelfTest`) except in ebpfless mode.

## Activity dumps, security profiles, anomaly detection

The security-profile subsystem ([`pkg/security/security_profile`](<<<SRC>>>/pkg/security/security_profile)) learns what a workload normally does and uses that baseline both to suppress noise and to detect anomalies:

1. **Activity dumps** (`runtime_security_config.activity_dump.*`): the manager traces up to `traced_cgroups_count` (default 5) container cgroups at a time, recording exec/open/DNS/IMDS events (rate-limited in kernel at 500 events/s) into an activity tree for `dump_duration` (15 min default). Finished dumps are encoded (protobuf profile format by default), stored locally under `${run_path}/runtime-security/profiles`, and streamed over `GetActivityDumpStream` to the security-agent, whose `ActivityDumpRemoteBackend` multiparts them to `cws-intake.<site>` on the `secdump` track (or sent directly under direct-send mode).
1. **Security profiles** (`runtime_security_config.security_profile.enabled`, default true) aggregate activity trees per workload image (up to `max_image_tags` versions). Once a profile is stable it drives **auto-suppression** — rule matches for behavior already in the profile are dropped (`auto_suppression.event_types`, default exec and DNS) — and **anomaly detection**, where events diverging from a stable profile emit `anomaly_detection` custom events, subject to minimum stable periods, warm-up, and a dedicated rate limiter.
1. **V2** (`security_profile.v2.enabled`, off by default) merges the dump/profile split into a single manager ([`manager_v2.go`](<<<SRC>>>/pkg/security/security_profile/manager_v2.go)) with per-event-type sampling.

The local storage directory for dumps must equal `security_profile.dir` when both features are on — validated at config load, both defaulting to `${run_path}/runtime-security/profiles`.

## Beyond eBPF: Fargate and Windows

### ebpfless mode and cws-instrumentation (Fargate)

On ECS/EKS Fargate there is no privileged system-probe with eBPF access. `runtime_security_config.ebpfless.enabled` switches to the `EBPFLessProbe` ([`probe_ebpfless.go`](<<<SRC>>>/pkg/security/probe/probe_ebpfless.go)), auto-enabled when `fargate.IsSidecar()` detects the Agent running as a Fargate sidecar. Instead of kernel hooks, syscall events come from the **`cws-instrumentation`** binary ([`cmd/cws-instrumentation`](<<<SRC>>>/cmd/cws-instrumentation), tracer in [`pkg/security/ptracer`](<<<SRC>>>/pkg/security/ptracer)): it wraps the workload entrypoint, traces it with ptrace+seccomp, decodes syscalls in-process, and streams msgpack-encoded events over a plain TCP connection to the probe at `runtime_security_config.ebpfless.socket` (default `localhost:5678`).

Getting the tracer into the workload differs by platform: on ECS you edit the task definition to wrap the command manually; on EKS Fargate the Cluster Agent's admission webhook ([`pkg/clusteragent/admission/mutate/cwsinstrumentation`](<<<SRC>>>/pkg/clusteragent/admission/mutate/cwsinstrumentation), enabled by `admission_controller.cws_instrumentation.enabled`) injects an init container that copies `cws-instrumentation` into a shared volume and rewrites the pod command, and can also wrap `kubectl exec` sessions. In ebpfless mode there are no kernel maps: approvers and discarders are no-ops, and gRPC-triggered self tests are unsupported. Running metrics switch to `datadog.security_agent.fargate_runtime.running` with `mode:fargate_ecs|fargate_eks` tags.

### Windows (ETW)

On Windows, CWS runs inside the `datadog-system-probe` service with the `datadog-security-agent` service still forwarding; the event channel is TCP `localhost:3335` since Unix sockets are not used. The `WindowsProbe` ([`probe_windows.go`](<<<SRC>>>/pkg/security/probe/probe_windows.go)) subscribes to ETW sessions for file I/O ([`probe_kernel_file_windows.go`](<<<SRC>>>/pkg/security/probe/probe_kernel_file_windows.go)), registry ([`probe_kernel_reg_windows.go`](<<<SRC>>>/pkg/security/probe/probe_kernel_reg_windows.go)), and the Windows audit provider ([`probe_auditing_windows.go`](<<<SRC>>>/pkg/security/probe/probe_auditing_windows.go)); process events come from the process-monitor driver. Tuning knobs: `windows_filename_cache_max`, `windows_registry_cache_max`, `etw_events_channel_size`, and a write-event rate limiter.

The SECL model is reduced to file/registry/process/user-session events and generated into [`pkg/security/seclwin`](<<<SRC>>>/pkg/security/seclwin) so the Windows build does not pull Linux-only dependencies — the package is generated (`bazel run //pkg/security/seclwin:sync`); editing it by hand is a lint error. When runtime security is enabled on Windows, `fim_enabled` is force-set to true unless explicitly configured ([`adjust_security.go`](<<<SRC>>>/pkg/system-probe/config/adjust_security.go)).

## Configuration

CWS internals read from the **system-probe** config namespace even though the keys look like `datadog.yaml` keys — `NewRuntimeSecurityConfig` ([`pkg/security/config/config.go`](<<<SRC>>>/pkg/security/config/config.go)) reads exclusively from the system-probe config. Defaults live in [`pkg/config/setup/system_probe_cws.go`](<<<SRC>>>/pkg/config/setup/system_probe_cws.go); env override prefix is `DD_RUNTIME_SECURITY_CONFIG_*`.

| Group | Keys (defaults) |
|---|---|
| Enablement | `runtime_security_config.enabled` (false), `.fim_enabled` (false; FIM-only mode loads only FIM rules) |
| Policies | `.policies.dir` (`/etc/datadog-agent/runtime-security.d`), `.policies.monitor.*`, `.remote_configuration.enabled` (true) |
| Transport | `.socket`, `.cmd_socket`, `.event_grpc_server`, `.direct_send_from_system_probe` (false), `.use_secruntime_track` (true), `.endpoints.*` |
| Event server | `.event_server.rate` (10), `.event_server.burst` (40), `.event_server.retention` (6 s), `.event_retry_queue_threshold` |
| Self test | `.self_test.enabled` (true), `.self_test.send_report` (true) |
| Dumps/profiles | `.activity_dump.*` (enabled true), `.security_profile.*` (enabled true, `v2.enabled` false), `.security_profile.anomaly_detection.*`, `.security_profile.auto_suppression.*` |
| Enforcement | `.enforcement.enabled` (true), `.enforcement.rule_source_allowed`, `.enforcement.disarmer.*` |
| Enrichment | `.hash_resolver.*` (enabled true), `.sbom.*`, `.user_sessions.ssh.enabled`, `.env_as_tags`, `.imds_ipv4` (169.254.169.254 — must parse as valid IPv4 or config load fails hard) |
| Fargate | `.ebpfless.enabled` (false; auto on Fargate sidecar), `.ebpfless.socket` (`localhost:5678`) |
| Probe (event monitor) | `event_monitoring_config.enable_kernel_filters`, `.event_stream.{use_ring_buffer,use_fentry,use_kprobe_fallback,buffer_size}`, `.envs_with_value` (env vars captured verbatim, e.g. `LD_PRELOAD`), `.custom_sensitive_words`, `.dns_resolution.*`, `.network.*`, `.syscalls_monitor.enabled`, `.network_process.enabled` (feeds NPM) |

/// warning
`runtime_security_config.enabled` must be set in **both** configs: `system-probe.yaml` turns on the probe and rule engine; `datadog.yaml` (or `security-agent.yaml`) turns on the security-agent forwarder. Enabling only one side produces no events and is a classic support case. The Helm chart and installer set both for you.
///

## Deployment-mode differences

| Environment | Behavior |
|---|---|
| Linux host install | Three systemd units (agent, system-probe, security-agent); Unix sockets under `/opt/datadog-agent/run/` |
| Docker / Kubernetes DaemonSet | Same process split as separate containers in the agent pod; `HOST_ROOT`/`HOST_PROC`/`HOST_SYS` env vars redirect resolvers to host mounts; the Helm chart shares the `runtime-security.d` and socket volumes between containers |
| Fargate (ECS/EKS) | ebpfless mode auto-enabled; workloads instrumented via `cws-instrumentation` (manually for ECS, admission webhook for EKS); hostname may legitimately be empty |
| Micro-VM / remote system-probe | `event_grpc_server: system-probe` with `vsock://` socket addresses |
| Windows | ETW-based FIM, reduced SECL model, TCP localhost sockets, no activity-dump storage backend on the security-agent side |
| macOS | Not supported (the probe requires Linux or Windows build tags) |

See [Runtime environments](../deployment/environments.md) for the general environment matrix.

## IPC and ports

| Channel | Transport | Auth | Purpose |
|---|---|---|---|
| `runtime_security_config.socket` (`/opt/datadog-agent/run/runtime-security.sock`; Windows `localhost:3335`; vsock supported) | gRPC | none (filesystem permissions) | `SecurityModuleEvent`: `GetEventStream`, `GetActivityDumpStream` |
| Cmd socket (`runtime_security_config.cmd_socket`, derived `cmd-runtime-security.sock`) | gRPC | none | `SecurityModuleCmd` (status, policy reload, dumps, self test) + the `SBOMCollector` stream (see [Compliance and SBOM](compliance.md)) |
| security-agent cmd port **5010** (localhost, TLS) | HTTPS | IPC auth token | `security-agent status/flare/config` CLI backend ([`cmd/security-agent/api`](<<<SRC>>>/cmd/security-agent/api)) |
| security-agent expvar port **5011** | HTTP | none | expvar telemetry |
| Core agent gRPC `cmd_port` **5001** | gRPC | IPC token + TLS | Remote workloadmeta/tagger/hostname for the security-agent; Remote Config for system-probe CWS policies |
| ebpfless probe `localhost:5678` | TCP (msgpack) | none | `cws-instrumentation` ptracer → system-probe event stream |
| Intakes | HTTPS | API key | `runtime-security-http-intake.logs.<site>` (tracks `secruntime`, `secinfo`), `cws-intake.<site>` (track `secdump`) |

The IPC auth token machinery is described in [Inter-process communication](../processes/ipc.md); note that the CWS Unix sockets deliberately do not use it.

## Observability and support surfaces

1. `system-probe runtime` subcommands drive the cmd socket: `policy check` (compile policies offline), `policy reload`, `self-test`, `activity-dump list/stop/generate`, `security-profile show`, discarder dumps.
1. The security-agent registers with the core agent's remote-agent registry ([`comp/core/remoteagent/fx-securityagent`](<<<SRC>>>/comp/core/remoteagent/fx-securityagent/fx.go)) so `agent status` and [flares](../operations/flare.md) aggregate its state; system-probe's flare provider pulls loaded CWS policies via [`pkg/security/flareregistry`](<<<SRC>>>/pkg/security/flareregistry/registry.go).
1. [`comp/metadata/securityagent`](<<<SRC>>>/comp/metadata/securityagent) ships a scrubbed security-agent configuration inventory payload every 10 minutes.

## Gotchas

1. **Dual-config enablement**: see the warning above — `runtime_security_config.enabled` in only one of system-probe/security-agent config yields silence, not an error at startup (the security-agent logs a connection-oriented error at best).
1. **FIM implies CWS infrastructure**: `fim_enabled` alone still loads the whole event-monitor module; enabling runtime auto-enables FIM, and disabling runtime force-disables activity dumps and profiles ([`adjust_security.go`](<<<SRC>>>/pkg/system-probe/config/adjust_security.go)).
1. **Reloads spike event rates**: every policy reload flushes discarders progressively over `flush_discarder_window`; expect a temporary event-rate surge after `policy reload` or an RC update.
1. **Rate limiting is per rule ID** and drops are metrics-only (`datadog.runtime_security.rules.rate_limiter.drop`); a noisy rule can starve itself invisibly.
1. **RC outage disarms kill actions** and reconnection re-arms them; kill actions additionally only honor rules from `enforcement.rule_source_allowed`.
1. **The tag-retention queue drops oldest under burst** — events waiting on container tags can silently expire (counted in `expiredEvents` per rule).
1. **SBOM usage mode is not CWS**: `sbom.enrichment.usage.enabled` spins up the full eBPF probe with only a `UsageConsumer` ([`usage_consumer.go`](<<<SRC>>>/pkg/security/module/usage_consumer.go)) and no rule engine — deliberately unbilled; the running metrics differ. Details in [Compliance and SBOM](compliance.md).
1. **`pkg/security/seclwin` is generated** — regenerate with `bazel run //pkg/security/seclwin:sync`, never edit.
1. **Only Fork/Exec/Exit/TracerMemfdSeal reach non-CWS consumers** of the event monitor; if you are adding a consumer and missing events, check `allowedEventTypes` first.
