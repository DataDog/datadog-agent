> **TL;DR:** `pkg/security/secl/rules` implements the runtime model for SECL policy and rule management — providing the data types, loading pipeline, evaluation engine, and kernel-filter (approver) derivation infrastructure used by the Runtime Security Agent.

# pkg/security/secl/rules

## Purpose

This package defines the runtime model for Datadog's **Security Events and Context Language (SECL)** rule and policy system. It provides the data types, loading pipeline, evaluation engine, and filtering infrastructure used by the Runtime Security Agent (CSM Threats) to decide whether a kernel event should trigger a security alert.

## Key elements

### Key types

| Type | File | Description |
|------|------|-------------|
| `RuleDefinition` | `model.go` | YAML/JSON-serialisable definition of a rule: `id`, `expression`, `tags`, `actions`, `filters`, version constraint, combine/override policy, rate-limiting token, and priority. |
| `MacroDefinition` | `model.go` | Named SECL expression fragment that rules can reference. Supports `merge` and `override` combine policies. |
| `PolicyRule` | `policy.go` | A rule as it exists inside a loaded policy, tracking acceptance state, parse/validation errors, the originating `Policy`, and any policies that overrode it. |
| `PolicyMacro` | `policy.go` | A macro as it exists inside a loaded policy. |
| `Rule` | `ruleset.go` | The runtime representation: embeds `PolicyRule` (definition + metadata) and `eval.Rule` (compiled evaluator). |
| `RuleSet` | `ruleset.go` | The central object — a map of event type → `RuleBucket` of compiled rules. Exposes `Evaluate(event)`, `GetApprovers()`, and observer registration via `RuleSetListener`. |
| `RuleBucket` | `bucket.go` | Groups all rules for a single event type. Rules inside a bucket are sorted by execution-context tag, policy type (default first), then user-defined priority. |
| `PolicyLoader` | `policy_loader.go` | Aggregates multiple `PolicyProvider` implementations. `LoadPolicies(opts)` merges providers in order, RC providers taking precedence over local file providers. Uses a debouncer to coalesce rapid reload notifications. |
| `Action` / `ActionDefinition` | `actions.go` / `model.go` | Side-effects attached to a rule (e.g. `set` a scoped variable). Compile-time filter expressions control whether an action runs at event time. |
| `Approvers` | `approvers.go` | `map[Field]FilterValues` — a pre-computed set of field-value constraints derived from the ruleset used to build kernel-level discarders (events that cannot match any rule are dropped early). |

### Key interfaces

#### Filtering sub-system (`rule_filters.go` + `filter/`)

Three filter types gate whether a rule/macro is loaded at all:

| Type | Criterion |
|------|-----------|
| `RuleIDFilter` | Exact rule ID match — used in functional tests to load a single rule. |
| `AgentVersionFilter` | SemVer constraint from `agent_version` field; strips pre-release metadata before comparison. |
| `SECLRuleFilter` | Evaluates the rule's `filters` list (SECL expressions) against a synthetic event; supports short-circuit for `os == "linux"` / `os == "windows"`. The inner implementation lives in `filter/seclrulefilter.go`. |

Filters implement `RuleFilter` (for rules) or `MacroFilter` (for macros). `PolicyLoaderOpts` holds the slices passed to `LoadPolicies`.

### Configuration and build flags

#### Policy providers

`PolicyProvider` is the interface for supplying raw policy data. Built-in constants:

| Constant | Value | Meaning |
|----------|-------|---------|
| `PolicyProviderTypeDir` | `"file"` | Watches a directory for YAML policy files. |
| `PolicyProviderTypeRC` | `"remote-config"` | Receives policies from the Remote Configuration service. |
| `PolicyProviderTypeBundled` | `"bundled"` | Compiled-in default rules. |
| `PolicyProviderTypeWorkload` | `"workload"` | Policies derived from workload identity rules. |

RC providers are re-ordered to come before file providers during `LoadPolicies` so they can override local defaults.

### Key functions

- `RuleSet.Evaluate(event)` — evaluates an event against all buckets; calls `RuleSetListener.RuleMatch` for each matching rule.
- `RuleSet.GetApprovers(capabilities)` — derives field-level approvers from the compiled ruleset for use as eBPF filters.
- `RuleSet.AddRules(rules)` / `RuleSet.AddMacros(macros)` — incremental loading.
- `PolicyLoader.LoadPolicies(opts)` — aggregated load with error accumulation (uses `hashicorp/go-multierror`).

## Usage

The package is consumed exclusively by the **Runtime Security Agent** (`pkg/security/`). The typical lifecycle:

1. A `PolicyLoader` is created with one or more `PolicyProvider` implementations (file, RC, bundled).
2. `LoadPolicies` is called on startup and on each change notification. Macro/rule filters can restrict which rules are loaded (e.g. only for the current OS or agent version).
3. The resulting `[]*Policy` are fed into `NewRuleSet` / `RuleSet.AddRules` to compile SECL expressions into evaluators.
4. The `RuleSet` is handed to the probe subsystem, which calls `GetApprovers` to install eBPF filters and registers itself as a `RuleSetListener` to receive `RuleMatch` callbacks at runtime.
5. When RC pushes a policy update, `PolicyLoader` debounces the notification and triggers a reload.

The `filter/` sub-package (`filter.SECLRuleFilter`) is kept separate to avoid circular imports when other components (e.g. the RC provider) need to evaluate SECL filter expressions without pulling in the full ruleset.
