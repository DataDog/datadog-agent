> **TL;DR:** `pkg/security/rules` is the runtime rule engine for CWS — it coordinates policy loading from disk, Remote Config, and bundled sources, compiles SECL rules, pushes eBPF kernel filters, and dispatches matched security events to the Datadog backend.

# pkg/security/rules

## Purpose

`pkg/security/rules` is the runtime runtime security rule engine. It ties together policy loading, SECL (Security Event Condition Language) rule evaluation, host/kernel-level event filtering (discarders), and telemetry. The engine sits between the kernel probe (which delivers raw security events) and the event sender (which ships matched alerts to the Datadog backend). The package is broken into three sub-packages:

- **`bundled/`** — ships a set of internal rules that are always present regardless of user configuration (e.g. user-cache refresh, SBOM refresh). Also hosts dynamically generated SBOM-derived rules.
- **`filtermodel/`** — implements the SECL evaluation model used to filter rules _before_ loading them. Evaluates conditions like kernel version, OS type, CORE support, and hostname so that rules that are not applicable to the current host are silently skipped.
- **`monitor/`** — tracks the load status of every policy and rule, emits `ruleset_loaded` and `heartbeat` custom events, and exports per-policy / per-rule StatsD metrics.

## Key elements

### Key types

#### `RuleEngine` (`engine.go`)

The central type. Holds references to all policy providers, the current rule set, rate limiter, policy monitor, and the probe.

```go
type RuleEngine struct {
    config          *config.RuntimeSecurityConfig
    probe           *probe.Probe
    currentRuleSet  *atomic.Value          // *rules.RuleSet
    policyProviders []rules.PolicyProvider
    policyLoader    *rules.PolicyLoader
    policyMonitor   *monitor.PolicyMonitor
    rateLimiter     *events.RateLimiter
    bundledProvider *bundled.PolicyProvider
    ...
}
```

Key methods:

| Method | Description |
|--------|-------------|
| `NewRuleEngine(...)` | Constructs the engine, registers it as a probe event handler, wires up default policy providers. |
| `Start(ctx, reloadChan)` | Loads policies, starts reload goroutines (from channel and from `policyLoader.NewPolicyReady()`), starts heartbeat goroutine, starts policy monitor. |
| `LoadPolicies(providers, sendLoadedReport)` | Core loading path: sets providers on the loader, builds a new `RuleSet`, applies rule/macro filters, calls `probe.ApplyRuleSet` (pushes eBPF filters to kernel), updates rate limiters, notifies API server. |
| `HandleEvent(*model.Event)` | Called by the probe for each kernel event. Evaluates the event against the current rule set; on miss, increments per-event-type counter and evaluates discarders. |
| `RuleMatch(ctx, rule, event)` | Called by the rule set when a rule fires. Records the matched rule for activity dumps, resolves container tags, calls `eventSender.SendEvent`. |
| `EventDiscarderFound(...)` | Forwarded to the probe to push a kernel-space discarder (suppresses future events of the same type for the same file/process). |
| `Stop()` | Closes all policy providers and waits for goroutines. |

### Key interfaces

#### `APIServer` interface (`engine.go`)

```go
type APIServer interface {
    ApplyRuleIDs([]rules.RuleID)
    ApplyPolicyStates([]*monitor.PolicyState)
    GetSECLVariables() map[string]*api.SECLVariableState
}
```

Implemented by `pkg/security/module` to expose the current rule/policy state over gRPC.

### Key functions

#### Policy providers (`engine.go`, `bundled/`)

The engine aggregates three default providers, in priority order:

1. **`bundled.PolicyProvider`** — internal rules + SBOM-derived rules. Always present.
2. **Remote Config provider** (`pkg/security/rconfig`) — rules pushed by the Datadog backend.
3. **Directory provider** (`rules.PoliciesDirProvider`) — YAML files from `config.PoliciesDir` on disk.

`bundled.PolicyProvider` exposes `SetSBOMPolicyDef(workloadKey, *rules.PolicyDef)` and `RemoveSBOMPolicyDef(workloadKey)` to dynamically inject or retract SBOM-based rules with a silent reload (no heartbeat emitted).

Internal rule IDs defined in `bundled/rules.go`:
- `refresh_user_cache` — triggers user/group cache refresh
- `refresh_sbom` — triggers SBOM refresh
- `need_refresh_sbom` — requests a new SBOM scan

#### `filtermodel` sub-package

`RuleFilterModel` / `RuleFilterEvent` implement the `eval.Model` / `eval.Event` interfaces against a virtual event that represents host properties. Fields available in rule `filters:` expressions:

| Field | Type | Notes |
|-------|------|-------|
| `kernel.version.{major,minor,patch,abi,flavor}` | int/string | Kernel version components |
| `os` | string | `runtime.GOOS` |
| `os.id`, `os.platform_id`, `os.version_id` | string | From `/etc/os-release` |
| `os.is_amazon_linux`, `os.is_cos`, `os.is_debian`, `os.is_oracle`, `os.is_rhel`, `os.is_rhel7`, `os.is_rhel8`, `os.is_sles`, `os.is_sles12`, `os.is_sles15` | bool | Distro detection |
| `kernel.core.enabled` | bool | CO-RE support and config enabled |
| `origin` | string | Probe origin (`ebpf`, `ebpfless`, etc.) |
| `hostname` | string | Agent hostname |
| `envs` | []string | Agent environment variables |

`NewRuleFilterModel(cfg, hostname, os)` is the constructor (Linux-only; requires kernel version introspection).

### Configuration and build flags

#### `monitor` sub-package

**`PolicyMonitor`** — started as a background goroutine by `RuleEngine.Start`. Emits:

- `datadog.security_agent.runtime.policies` gauge (every 30 s) tagged with `policy_name`, `policy_source`, `policy_version`.
- `datadog.security_agent.runtime.rules.status` gauge per rule (opt-in, `PolicyMonitorPerRuleEnabled`).

**`PolicyState`** — the serialisable snapshot of a loaded policy, status field is one of:

| Status | Meaning |
|--------|---------|
| `loaded` | All rules loaded successfully |
| `partially_filtered` | Some rules excluded by host filters |
| `partially_loaded` | Some rules failed to parse/compile |
| `fully_rejected` | All rules failed |
| `fully_filtered` | All rules excluded |
| `error` | Policy file could not be parsed |

**`RulesetLoadedEvent`** — sent to the Datadog backend when a new rule set is loaded. Contains the full list of `PolicyState` objects and the kernel-space filter report.

**`HeartbeatEvent`** — sent periodically (every minute for the first 5 times, then every 10 minutes) to confirm the rule set is still active.

**`ReportRuleSetLoaded(bundle, sender, statsd)`** — top-level function used by `RuleEngine.LoadPolicies` to ship the loaded event.

**`NewPoliciesState(rs, filteredRules, errs, includeInternal)`** — builds the `[]*PolicyState` slice from a fully loaded `RuleSet`, filtered-out rules, and errors.

## Usage

The package is instantiated once per CWS module, inside `pkg/security/module/cws.go`:

```go
ruleEngine, err = rules.NewRuleEngine(evm, cfg, probe, rateLimiter, apiServer, sender, statsd, hostname, ipc, listeners...)
ruleEngine.Start(ctx, reloadChan)
```

Policy reload is triggered:
- At startup.
- Via `reloadChan` (sent by the module when it receives a `SIGUSR2` or an API call).
- Automatically when any `PolicyProvider` signals new policies via `policyLoader.NewPolicyReady()`.
- Silently (no heartbeat) when SBOM data changes.

The engine implements `rules.RuleSetListener` so it receives `RuleMatch` and `EventDiscarderFound` callbacks directly from the `secl/rules` evaluation engine.

### End-to-end event flow

```
kernel (eBPF/ETW)
  └─► probe.EBPFProbe               (pkg/security/probe — see probe.md)
        └─► field_handlers_ebpf.go  resolves lazy fields via EBPFResolvers
              └─► RuleEngine.HandleEvent
                    ├─► RuleSet.Evaluate(event)         (secl/rules — see secl-rules.md)
                    │     └─► RuleSetListener.RuleMatch ──► RuleEngine.RuleMatch
                    │               ├─► spManager.HasActiveActivityDump  (security_profile)
                    │               ├─► rateLimiter.Allow               (events — see events.md)
                    │               └─► eventSender.SendEvent
                    └─► EventDiscarderFound ──► probe.OnNewDiscarder (kernel discarder push)
```

### Cross-package interactions

| This package interacts with | Via | Purpose |
|-----------------------------|-----|---------|
| `pkg/security/secl/rules` | `rules.RuleSet`, `rules.PolicyLoader`, `rules.PolicyProvider` | SECL rule compilation, policy loading, approver derivation |
| `pkg/security/secl` (compiler) | `eval.Model`, `eval.RuleEvaluator` | Underlying SECL evaluation engine — see [secl.md](secl.md) |
| `pkg/security/secl/model` | `model.Event`, `model.Model` | Concrete CWS event types evaluated at runtime — see [secl-model.md](secl-model.md) |
| `pkg/security/probe` | `probe.Probe.ApplyRuleSet`, `probe.Probe.OnNewDiscarder` | Pushes eBPF kfilter approvers and discarders to the kernel — see [probe.md](probe.md) |
| `pkg/security/rconfig` | `rconfig.RCPolicyProvider` | Receives remote configuration policy updates from the Datadog backend — see [rconfig.md](rconfig.md) |
| `pkg/security/events` | `events.RateLimiter`, `events.EventSender`, `events.CustomEvent` | Rate-limits outbound events and ships matched signals — see [events.md](events.md) |
| `pkg/security/rules/monitor` | `monitor.PolicyMonitor`, `monitor.ReportRuleSetLoaded` | Emits `ruleset_loaded` / `heartbeat` custom events and StatsD gauges |
| `pkg/security/rules/bundled` | `bundled.PolicyProvider` | Provides always-on internal rules and SBOM-derived rules |
| `pkg/security/rules/filtermodel` | `filtermodel.RuleFilterModel` | Host-aware rule filter evaluated in `LoadPolicies` to skip inapplicable rules |

### SECL rule lifecycle from YAML to kernel

1. **Policy YAML** is read by a `rules.PolicyProvider` (file, RC, or bundled).  The RC provider (`pkg/security/rconfig`) watches three Remote Config products; the bundled provider always injects internal rules. See [rconfig.md](rconfig.md) and [secl-rules.md](secl-rules.md).
2. **`PolicyLoader.LoadPolicies`** merges providers in priority order (RC > local file > bundled), runs `filtermodel.RuleFilterModel` filters, and produces `[]*rules.Policy`.
3. **`RuleSet.AddMacros` / `AddRules`** compiles each SECL expression (via `pkg/security/secl/compiler/eval`) into a closure-based `RuleEvaluator`. See [secl.md](secl.md).
4. **`probe.ApplyRuleSet`** derives kernel approvers (`rules.GetApprovers`) and writes them to eBPF maps via `kfilters`. See [probe.md](probe.md).
5. **`HandleEvent`** is called for every decoded `model.Event`; if the rule set evaluation fires a match, `RuleMatch` calls `eventSender.SendEvent` (rate-limited by `events.RateLimiter`). See [events.md](events.md).
6. On miss, `EventDiscarderFound` pushes a kernel-space inode/pid discarder so future identical events are suppressed without a user-space context switch.
