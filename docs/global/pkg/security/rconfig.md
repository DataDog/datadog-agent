> **TL;DR:** `pkg/security/rconfig` provides the Remote Configuration (RC) policy provider for CWS — subscribing to `CWSDefaultPolicies`, `CWSCustomPolicies`, and `CWSRemediation` RC products and feeding debounced policy updates into the `RuleEngine` for automatic rule reloads.

# pkg/security/rconfig — Remote Configuration policy provider for CWS

## Purpose

`pkg/security/rconfig` provides the **Remote Configuration (RC) policy provider** for Cloud Workload Security (CWS). It subscribes to three RC products (`CWSDefaultPolicies`, `CWSCustomPolicies`, `CWSRemediation`) via a gRPC client, debounces rapid updates, and implements the `rules.PolicyProvider` interface so that the `RuleEngine` can reload its rule set whenever the Datadog backend pushes new CWS policies.

The package has no build tag and works on all platforms where CWS runs.

## Key elements

### Key types

| Type | File | Description |
|------|------|-------------|
| `RCPolicyProvider` | `policies.go` | Main type. Wraps a Remote Config `client.Client`, caches the three latest policy maps (`lastDefaults`, `lastCustoms`, `lastRemediations`), and implements `rules.PolicyProvider`. |

`RCPolicyProvider` satisfies the `rules.PolicyProvider` interface — enforced by the compile-time assertion `var _ rules.PolicyProvider = (*RCPolicyProvider)(nil)`.

### Key functions

#### Constructor

```go
func NewRCPolicyProvider(
    dumpPolicies bool,
    setEnforcementCallback func(bool),
    ipc ipc.Component,
) (*RCPolicyProvider, error)
```

- Creates a gRPC RC client subscribed to `state.ProductCWSDD`, `state.ProductCWSCustom`, and `state.ProductCWSRemediation`.
- Poll interval: `1 s` (`securityAgentRCPollInterval`).
- `dumpPolicies`: when true, each received policy is written to a temp file for debugging.
- `setEnforcementCallback`: called with `true` when RC connectivity is confirmed, and with whatever state RC reports when the connection changes.

### Key interfaces

#### Methods implementing `rules.PolicyProvider`

| Method | Description |
|--------|-------------|
| `Start()` | Registers update callbacks for all three products, starts the debouncer and the RC client. |
| `LoadPolicies(macroFilters, ruleFilters)` | Iterates the cached configs in stable (sorted key) order, calls `rules.LoadPolicy` for each, reports apply status back to the RC client (`ApplyStateAcknowledged` or `ApplyStateError`), and returns `[]*rules.Policy` plus any accumulated errors. |
| `SetOnNewPoliciesReadyCb(cb func(silent bool))` | Registers the callback invoked after the debounce window elapses. RC-triggered reloads always pass `silent=false` (they produce a heartbeat event). |
| `Close()` | Stops the debouncer and closes the RC client. No-op if never started. |
| `Type()` | Returns `rules.PolicyProviderTypeRC`. |

### Configuration and build flags

#### Debouncing

Rapid policy pushes (e.g., several configs arriving in quick succession) are coalesced: each update callback calls `r.debouncer.Call()`, and `onNewPoliciesReady` fires only once after `debounceDelay = 5 s` of silence.

#### RC products subscribed

| Product constant | Policy type assigned |
|-----------------|---------------------|
| `state.ProductCWSDD` | `rules.DefaultPolicyType` |
| `state.ProductCWSCustom` | `rules.CustomPolicyType` |
| `state.ProductCWSRemediation` | `rules.RemediationPolicyType` |

#### Constants

| Constant | Value |
|----------|-------|
| `securityAgentRCPollInterval` | `1 * time.Second` |
| `debounceDelay` | `5 * time.Second` |
| `agentName` (internal) | `"security-agent"` |

## Usage

`RCPolicyProvider` is instantiated inside `pkg/security/rules/engine.go` when remote configuration is enabled:

```go
// pkg/security/rules/engine.go
rcPolicyProvider, err := rconfig.NewRCPolicyProvider(
    e.config.RemoteConfigurationDumpPolicies,
    e.rcStateCallback,
    e.ipc,
)
rcPolicyProvider.Start()
// rc provider is added to e.policyProviders
```

The `RuleEngine` holds a list of `rules.PolicyProvider` implementations (local file provider, bundled provider, RC provider). On each reload cycle it calls `LoadPolicies` on every provider and merges the resulting policy sets. Because `RCPolicyProvider` calls `SetOnNewPoliciesReadyCb` to trigger reloads, no additional wiring is needed — policy updates from the Datadog backend flow automatically into the active rule set.

### Provider priority

The `RuleEngine` aggregates three default providers in this order (highest priority first):

1. **RC provider** (`rconfig.RCPolicyProvider`) — rules pushed from the Datadog backend.
2. **Directory provider** (`rules.PoliciesDirProvider`) — YAML files on disk.
3. **Bundled provider** (`bundled.PolicyProvider`) — always-on internal rules and SBOM-derived rules.

When the same rule ID is defined in multiple providers, the first provider wins.

### Apply-state reporting

After `LoadPolicies` finishes processing each remote config entry, the result is reported back to the RC backend via the gRPC client:

- `ApplyStateAcknowledged` — policy was parsed and loaded successfully.
- `ApplyStateError` — parsing or loading failed (error message included).

This feedback is visible in the Datadog UI under Remote Configuration's apply-state column and is also consumed by the `monitor.PolicyMonitor` which emits `ruleset_loaded` and `heartbeat` custom events (see [rules.md](rules.md)).

### Relationship to `comp/remote-config/rcclient`

`RCPolicyProvider` does **not** use the fx-managed `rcclient.Component`. Instead it constructs its own lightweight gRPC client directly (using `pkg/config/remote/client`) with `agentName = "security-agent"` and a 1-second poll interval. This keeps the CWS rule engine independent from the main agent's RC component lifecycle.

The `comp/remote-config/rcclient` component uses the same underlying RC infrastructure (`pkg/config/remote`) but is intended for the core-agent process. For the security-agent process, `rconfig.NewRCPolicyProvider` is the correct entry point.

See [comp/remote-config/rcclient.md](../../../comp/remote-config/rcclient.md) for the component-based counterpart and [rules.md](rules.md) for how the `RuleEngine` consumes all providers.

## Related documentation

| Doc | Description |
|-----|-------------|
| [rules.md](rules.md) | `RuleEngine` that consumes `RCPolicyProvider` as one of its three policy providers; describes the full policy loading lifecycle and provider priority order. |
| [security.md](security.md) | Top-level CWS overview; RC-driven policy updates appear in the event flow as automatic rule reloads without agent restart. |
| [../../../comp/remote-config/rcclient.md](../../../comp/remote-config/rcclient.md) | The fx-component-based RC client used in the core-agent process; `rconfig` builds its own equivalent client directly for the security-agent process. |
