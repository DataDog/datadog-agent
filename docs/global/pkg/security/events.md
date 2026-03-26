> **TL;DR:** `pkg/security/events` defines the shared event vocabulary for the CWS pipeline — including the `Event`/`EventSender` interfaces, `CustomEvent` for agent-generated telemetry, custom rule ID constants, and the token-bucket rate limiters that gate outbound security signals.

# pkg/security/events

## Purpose

`pkg/security/events` defines the types, constants, and interfaces used to represent, classify, rate-limit, and dispatch security events from the runtime security agent to the Datadog backend. It is the shared vocabulary for the event pipeline: the probe produces `model.Event` values; the rule engine matches them; this package provides the plumbing to send the results.

## Key elements

### Key types

#### `Event` interface and `EventSender` (`event.go`)

```go
type Event interface {
    GetWorkloadID() string
    GetTags() []string
    GetType() string
    GetActionReports() []model.ActionReport
    GetFieldValue(eval.Field) (interface{}, error)
}

type EventSender interface {
    SendEvent(rule *rules.Rule, event Event, extTagsCb func() ([]string, bool), service string)
}
```

`Event` is implemented by both `model.Event` (kernel-sourced events) and `CustomEvent` (agent-generated telemetry events). `EventSender` is implemented in `pkg/security/module` and routes events to the Datadog log intake.

#### `BackendEvent` and `AgentContext` (`event.go`)

JSON wrapper types for the wire format sent to the backend. `AgentContext` carries rule metadata (rule ID, version, policy name/version) plus agent context (OS, arch, kernel version, distribution). It is embedded in every outbound event.

`OriginalRuleID` preserves the original rule ID when a rule is overridden or duplicated (e.g. by Remote Config). `RuleActions` carries the serialised action list from the rule definition (kill, hash, set variables, etc.) and is omitted when empty.

```go
type AgentContext struct {
    RuleID         string
    OriginalRuleID string
    RuleVersion    string
    RuleActions    []json.RawMessage  // serialised rule action list, omitempty
    PolicyName     string
    PolicyVersion  string
    Version        string   // agent version
    OS             string
    Arch           string
    Origin         string
    KernelVersion  string
    Distribution   string
}
```

#### `CustomEvent` and `CustomEventCommonFields` (`custom.go`)

```go
type CustomEvent struct {
    eventType     model.EventType
    tags          []string
    marshalerCtor func() EventMarshaler
}
```

Agent-generated events (heartbeats, self-test results, anomaly detections, …) that are sent on built-in "custom" rules. `MarshalJSON` defers to the `EventMarshaler` supplied at construction time. Use `NewCustomEvent(eventType, marshaler, tags...)` or the lazy variant `NewCustomEventLazy`.

`CustomEventCommonFields` embeds `Timestamp`, `Service` (`"runtime-security-agent"`), and `AgentContainerContext` (the agent's own container ID/start time). Call `FillCustomEventCommonFields(acc)` to populate them.

### Key functions

#### Custom rule ID constants (`custom.go`)

| Constant | Value | Description |
|----------|-------|-------------|
| `RulesetLoadedRuleID` | `"ruleset_loaded"` | Emitted when a new rule set is loaded |
| `HeartbeatRuleID` | `"heartbeat"` | Periodic liveness signal |
| `AnomalyDetectionRuleID` | `"anomaly_detection"` | Security profile behavioral anomaly |
| `AbnormalPathRuleID` | `"abnormal_path"` | Unexpected path detected during resolution |
| `SelfTestRuleID` | `"self_test"` | Agent self-test result |
| `NoProcessContextErrorRuleID` | `"no_process_context"` | Event received with no process context |
| `BrokenProcessLineageErrorRuleID` | `"broken_process_lineage"` | Process ancestry chain is broken |
| `EBPFLessHelloMessageRuleID` | `"ebpfless_hello_msg"` | eBPFless probe hello handshake |
| `InternalCoreDumpRuleID` | `"internal_core_dump"` | Internal agent core dump event |
| `SysCtlSnapshotRuleID` | `"sysctl_snapshot"` | Sysctl parameter snapshot |
| `RawPacketActionRuleID` | `"rawpacket_action"` | Raw packet enforcement action |
| `FailedDNSRuleID` | `"failed_dns"` | DNS packet that could not be decoded |
| `RemediationStatusRuleID` | `"remediation_status"` | Remediation action status |

`AllCustomRuleIDs()` returns the full list (minus heartbeat and EBPFless IDs) used to pre-register limiters.

`AllSecInfoRuleIDs()` returns the subset of custom rule IDs routed over the `SecInfo` pipeline track (currently only `remediation_status`). Events on this track are dispatched by `RuntimeSecurityAgent.DispatchEvent` to `secInfoReporter` rather than the main `reporter`.

`NewCustomRule(id, description, opts)` creates a minimal `*rules.Rule` with no SECL expression, used to send custom events on pseudo-rules.

### Key interfaces

#### Rate limiting (`rate_limiter.go`, `std_limiter.go`, `token_limiter.go`, `ad_limiter.go`)

`RateLimiter` holds a map of `rule_id → Limiter` and gate-keeps all outbound events.

```go
type Limiter interface {
    Allow(event Event) bool
    SwapStats() []utils.LimiterStat
}
```

Three implementations:

| Type | Behaviour |
|------|-----------|
| `StdLimiter` | Token bucket (via `golang.org/x/time/rate`). Default for user rules: 100 ms interval, burst of 40. |
| `TokenLimiter` | Token bucket keyed by a field extracted from the event (e.g. `process.file.path`). Up to 500 unique tokens. Used when `rule.Def.RateLimiterToken` is set. |
| `AnomalyDetectionLimiter` | Keyed by `event.GetWorkloadID()`. Configured with `AnomalyDetectionRateLimiterNumKeys` and `AnomalyDetectionRateLimiterNumEventsAllowed`. |

Default limits for custom rules:

| Rule | Limit |
|------|-------|
| `ruleset_loaded`, `heartbeat`, `ebpfless_hello_msg` | Unlimited |
| `abnormal_path`, `no_process_context`, `broken_process_lineage`, `internal_core_dump`, `failed_dns`, `rawpacket_action` | 1 per 30 s |

`RateLimiter.Apply(ruleSet, customRuleIDs)` rebuilds the limiter map when the rule set changes. `SendStats()` emits `datadog.security_agent.runtime.rate_limiter.drop` and `datadog.security_agent.runtime.rate_limiter.allow` counters.

### Configuration and build flags

#### JSON helpers (`json.go`, `event_easyjson.go`)

`EventMarshaler` interface:
```go
type EventMarshaler interface {
    ToJSON() ([]byte, error)
}
```

`event_easyjson.go` is generated code (easyJSON) for `AgentContext` and `BackendEvent`. Regenerate with:
```
go generate ./pkg/security/events/...
```

## Usage

The `events` package is used throughout `pkg/security`:

- **`pkg/security/rules`** — creates a `RateLimiter`, calls `rateLimiter.Apply(rs, AllCustomRuleIDs())` after every reload, calls `rateLimiter.Allow(ruleID, event)` before `eventSender.SendEvent`. See [rules.md](rules.md) for the end-to-end event flow.
- **`pkg/security/module`** — implements `EventSender`; receives `(rule, event, extTagsCb, service)` and serialises/forwards to the log intake. See [security.md](security.md) for how `CWSConsumer` owns this flow.
- **`pkg/security/rules/monitor`** — builds `CustomEvent` values for `ruleset_loaded` and `heartbeat` events using `NewCustomEvent` / `FillCustomEventCommonFields`.
- **`pkg/security/probe`** — builds `CustomEvent` values for `abnormal_path`, `self_test`, `no_process_context`, etc. See [probe.md](probe.md).
- **`pkg/security/serializers`** — uses `AgentContext` / `BackendEvent` as the outermost JSON envelope when serialising full security events. See [serializers.md](serializers.md).
- **`pkg/security/agent`** — `DispatchEvent` routes `SecurityEventMessage` payloads to `reporter` or `secInfoReporter` based on the `Track` field, which maps to the `SecInfo` track constant defined in `pkg/security/common`. See [agent.md](agent.md) and [common.md](common.md).

## Related documentation

| Doc | Description |
|-----|-------------|
| [security.md](security.md) | Top-level CWS overview: how `EventSender`, `RateLimiter`, and `CustomEvent` fit in the overall event pipeline. |
| [rules.md](rules.md) | `RuleEngine.RuleMatch` calls `rateLimiter.Allow` then `eventSender.SendEvent`; `monitor` sub-package emits `ruleset_loaded` / `heartbeat` custom events. |
| [serializers.md](serializers.md) | `AgentContext` / `BackendEvent` are the outermost JSON envelope; `MarshalCustomEvent` serialises `CustomEvent` payloads. |
| [probe.md](probe.md) | Probe emits `CustomEvent` values for self-tests, abnormal paths, and eBPFless hello messages via `CustomEventHandler`. |
| [agent.md](agent.md) | `RuntimeSecurityAgent.DispatchEvent` routes inbound `SecurityEventMessage` by `Track`; the track value corresponds to the `TrackType` constants in `pkg/security/common`. |
| [common.md](common.md) | `SecInfo` / `SecRuntime` / `Logs` track type constants used to select the log intake pipeline. |
