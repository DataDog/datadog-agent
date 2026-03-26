> **TL;DR:** `pkg/security` is the integration hub for Datadog Cloud Workload Security (CWS), wiring together the eBPF probe, rule engine, security-agent gRPC bridge, SECL compiler, resolvers, security profiles, and all supporting sub-systems.

# pkg/security — Cloud Workload Security (CWS)

## Purpose

`pkg/security` is the root package for the Datadog **Cloud Workload Security** (CWS) product. It does not contain runnable logic itself; instead it acts as the integration hub that wires together all CWS sub-systems:

- **Probe** (`probe/`) — kernel-space event collection via eBPF (or eBPF-less on Windows/ptracer).
- **Module** (`module/`) — the system-probe module (`CWSConsumer`) that owns the probe lifecycle, the rule engine, the gRPC API servers, rate-limiting, self-tests, and event forwarding.
- **Agent** (`agent/`) — the security-agent process component (`RuntimeSecurityAgent`) that listens to the system-probe gRPC stream and forwards events and activity dumps to the Datadog backend.
- **Rules** (`rules/`) — rule engine (`RuleEngine`) that loads, evaluates and reloads SECL policy sets.
- **SECL** (`secl/`) — the Security Event Composition Language compiler, evaluator, and data model.
- **Resolvers** (`resolvers/`) — process, mount, dentry, network-namespace, and container resolvers that enrich raw kernel events with human-readable context.
- **Security Profiles** (`security_profile/`) — activity-dump collection and anomaly-detection profiles.
- **Config** (`config/`) — `RuntimeSecurityConfig`, `Config`, and `ProbeConfig` types used across all sub-systems.
- **Events** (`events/`) — shared event types, `CustomEvent`, `EventSender`, `RateLimiter`.
- **Serializers** (`serializers/`) — JSON marshalling of `model.Event` for the backend API.
- **Metrics** (`metrics/`) — all statsd metric name constants for CWS.
- **Telemetry** (`telemetry/`) — containers-running telemetry helper.
- **Proto / API** (`proto/api/`) — protobuf definitions for the gRPC interface between system-probe and security-agent.

## Key elements

### Key types

| Type | Package | Description |
|------|---------|-------------|
| `CWSConsumer` | `module` | Central coordinator: owns the probe, rule engine, gRPC servers, rate limiter, and self-tester. Implements the `eventmonitor.EventConsumer` interface for system-probe. |
| `APIServer` | `module` | gRPC server implementation for `SecurityModuleCmd` and `SecurityModuleEvent` — handles status queries, rule reload requests, dump commands, and event streaming to the security-agent. |
| `RuleEngine` | `rules` | Loads and reloads SECL policy sets, calls `Probe.ApplyRuleSet`, dispatches matched events through `EventSender`, and tracks per-rule action statistics. |
| `RuntimeSecurityAgent` | `agent` | Runs inside the security-agent process. Connects to the system-probe gRPC stream, receives serialized events, and forwards them to the Datadog log pipeline via `common.RawReporter`. |
| `RuntimeSecurityConfig` | `config` | Configuration object for all runtime-security knobs (feature flags, sockets, activity dump settings, self-test options, etc.). Populated from `datadog.yaml` under the `runtime_security_config` key. |
| `Config` | `config` | Top-level config wrapper that embeds both `RuntimeSecurityConfig` and `ProbeConfig`. |
| `RateLimiter` | `events` | Per-rule token-bucket rate limiter that prevents event floods from overwhelming the backend. |
| `CustomEvent` | `events` | Synthetic events generated internally (self-test results, heartbeats, rule-set-loaded notifications) that bypass normal kernel-event flow. |
| `SelfTester` | `probe/selftests` | Runs controlled filesystem/process operations after startup to verify the probe detects them; reports success/failure via a `CustomEvent`. |

### Key interfaces

| Interface | Package | Description |
|-----------|---------|-------------|
| `EventSender` | `events` | `SendEvent(rule, event, extTagsCb, service)` — abstracts event forwarding; default implementation is `APIServer`, overridable in tests. |
| `RuleSetListener` | `secl/rules` | Callback interface notified when a rule set is loaded; used by `SelfTester` to inject test rules. |

### Configuration and build flags

#### Configuration keys (datadog.yaml)

```yaml
runtime_security_config:
  enabled: true
  fim_enabled: true
  socket: /opt/datadog-agent/run/runtime-security.sock
  event_monitoring_config:
    socket: /opt/datadog-agent/run/event-monitor.sock
  activity_dump:
    enabled: true
  self_test:
    enabled: true
    send_report: true
```

#### Build flags

| Flag | Effect |
|------|--------|
| `linux` | Full eBPF probe; required for most CWS features. |
| `windows` | Windows kernel-file and registry probe. |
| `linux_bpf` | Enables BTF CO-RE constant fetcher and ring-buffer support inside `constantfetch`. |

## Usage

### How CWS starts

1. **system-probe** instantiates `probe.NewProbe` and then `module.NewCWSConsumer`, which creates:
   - The `APIServer` (gRPC).
   - The `RuleEngine` (loads policies from disk and Remote Config).
   - The `SelfTester` (optional).
2. `CWSConsumer.Start()` starts the gRPC servers, the `RuleEngine` (which calls `Probe.ApplyRuleSet`), and schedules the self-test goroutine.
3. **security-agent** instantiates `agent.NewRuntimeSecurityAgent` and connects to system-probe via gRPC. Received `SecurityEventMessage` payloads are passed to `RawReporter`, which feeds the Datadog log pipeline.

### Event flow

```
kernel (eBPF / ETW)
    └─> probe.EBPFProbe.handleEvent
         └─> Probe.DispatchEvent  (rule evaluation in RuleEngine)
              ├─> RateLimiter.Allow
              └─> APIServer.SendEvent (gRPC stream)
                   └─> RuntimeSecurityAgent.DispatchEvent
                        └─> RawReporter → Datadog backend
```

### Adding a new sub-system

- Implement `probe.EventConsumerHandler` and register with `Probe.AddEventConsumer`.
- Or implement `probe.CustomEventHandler` and register with `Probe.AddCustomEventHandler`.
- For full rule evaluation, implement `rules.RuleSetListener` and pass it to `RuleEngine`.

### Testing

Functional tests live in `pkg/security/tests/`. They use `module.NewCWSConsumer` directly with a test rule policy and assert that probe events match expectations. Run with:

```bash
sudo go test -v -tags=linux_bpf ./pkg/security/tests/...
```

Self-tests are run automatically 15 seconds after probe start (`selftestStartAfter`) and then every 15 minutes (`selftestDelay`). Results are reported as `CustomEvent` payloads and as the `runtime_security.self_test` statsd gauge.

---

## Related documentation

| Doc | Description |
|-----|-------------|
| [probe.md](probe.md) | Kernel event capture via eBPF / ETW: `Probe`, `EBPFProbe`, `PlatformProbe`, discarders, approvers, `constantfetch`. |
| [secl.md](secl.md) | SECL language overview: parser, eval engine, policy/rule management, approver derivation. |
| [secl-model.md](secl-model.md) | CWS event data model: `Event`, all event-type structs, `FieldHandlers`, generated accessors. |
| [rules.md](rules.md) | `RuleEngine`: policy loading, rule evaluation, discarder feedback, policy monitor, heartbeat. |
| [resolvers.md](resolvers.md) | `EBPFResolvers`: dentry, mount, process, cgroup, hash, tags, netns, and other resolver types. |
| [security-profile.md](security-profile.md) | Activity dumps and behavioral security profiles: `Manager`, `ActivityTree`, anomaly detection. |
| [events.md](events.md) | `CustomEvent`, `EventSender`, `RateLimiter`, `AgentContext`, custom rule ID constants. |
| [serializers.md](serializers.md) | `EventSerializer`: JSON marshalling of `model.Event` for the Datadog SIEM backend. |
| [agent.md](agent.md) | `RuntimeSecurityAgent`: gRPC event/dump listener inside the security-agent process. |
| [ptracer.md](ptracer.md) | eBPF-less ptrace-based tracer (`cws-instrumentation`): `CWSPtracerCtx`, `Tracer`, syscall handlers. |
| [secl-compiler.md](secl-compiler.md) | SECL compiler internals: `ast/` parser (participle), `eval/` typed evaluators, `Context`, `ContextPool`. |
| [secl-rules.md](secl-rules.md) | `RuleSet`, `PolicyLoader`, `Approvers`, rule/macro filter types, policy provider constants. |
| [../../pkg/ebpf.md](../../pkg/ebpf.md) | Shared eBPF infrastructure: CO-RE loader, `Manager`, `MapCleaner`, perf/ring-buffer handlers. |
