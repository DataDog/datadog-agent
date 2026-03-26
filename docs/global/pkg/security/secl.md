> **TL;DR:** `pkg/security/secl` is the Security Evaluation and Control Language (SECL) module — providing the parser, typed evaluator, rule/policy management, and approver derivation engine that power all CWS detection rules.

# pkg/security/secl

## Purpose

`pkg/security/secl` is the Security Evaluation and Control Language (SECL) module — the rule DSL powering Datadog Cloud Workload Security (CWS). It provides everything needed to:

1. **Parse** SECL rule expressions into an AST.
2. **Compile** those ASTs into typed, optimised evaluators that run against kernel events at runtime.
3. **Organise** compiled rules into policies and rule sets, complete with macros, variables, approvers (kernel-level pre-filters), and rule actions.

The module lives in its own `go.mod` so it can be imported by both the agent and tooling (documentation generators, validators, etc.) without pulling in the full agent dependency graph. It has **no API stability guarantees**.

---

## Sub-packages

| Sub-package | Role |
|---|---|
| `compiler/ast` | Tokenises and parses SECL text into a Go AST (via [participle](https://github.com/alecthomas/participle)) |
| `compiler/eval` | Compiles AST nodes into closure-based evaluators; defines the `Model` / `Event` interfaces |
| `rules` | Policy loading, rule set management, approver derivation, and rule actions |
| `model` | Concrete CWS event data model (see [secl-model.md](secl-model.md)) |
| `validators` | Schema validation for rule expressions and policy YAML |
| `utils` | Shared small utilities |
| `containerutils` | Container-ID helpers |
| `log` | Thin logger abstraction used within the module |
| `schemas` | JSON schemas for policies |

---

## Key elements

### Key types

The sub-package table above lists the top-level modules. See the sections below for their key exported types and interfaces.

### Key interfaces

#### Core interfaces (`compiler/eval`)

```go
// Model bridges the data model to the evaluator.
type Model interface {
    GetEvaluator(field Field, regID RegisterID, offset int) (Evaluator, error)
    ValidateField(field Field, value FieldValue) error
    ValidateRule(rule *Rule) error
    NewEvent() Event
    GetFieldRestrictions(field Field) []EventType
}

// Event is the interface an event struct must satisfy at evaluation time.
type Event interface {
    Init()
    GetType() EventType
    GetFieldMetadata(field Field) (EventType, reflect.Kind, string, bool, error)
    SetFieldValue(field Field, value interface{}) error
    GetFieldValue(field Field) (interface{}, error)
    GetTags() []string
}
```

### Key functions

#### Language features (`compiler/ast`)

`ParsingContext` is the root factory. Create one with `NewParsingContext(withCache bool)` and call:

| Method | Returns |
|---|---|
| `ParseRule(expr)` | `*ast.Rule` — root of the AST |
| `ParseMacro(expr)` | `*ast.Macro` |
| `ParseExpression(expr)` | `*ast.Expression` |

The SECL lexer supports the following literal kinds, all visible in the AST grammar:

- **Scalar values**: string literals (`"foo"`), integers, booleans
- **Pattern matching**: glob patterns (`~"foo*"`), regular expressions (`r"foo.*"`)
- **Network types**: IP addresses and CIDR ranges
- **Durations**: e.g., `5s`, `1m`, `2h`
- **Variables**: `${varname}` — runtime-substituted values
- **Field references**: `%{field}` — reference another field's value in-expression

Operators available in expressions: `==`, `!=`, `<`, `<=`, `>`, `>=`, `=~` (regex match), `!~`, `in`, `notin`, `allin`, bitwise `&`, `|`, `^`, arithmetic `+`, `-`, unary `!` / `not` / `-` / `^`.

### Evaluation engine (`compiler/eval`)

#### Typed evaluators

Each field type has a corresponding evaluator struct:

| Type | Struct |
|---|---|
| `bool` | `BoolEvaluator` |
| `int` | `IntEvaluator` |
| `string` | `StringEvaluator` |
| `[]string` | `StringArrayEvaluator` / `StringValuesEvaluator` |
| `[]int` | `IntArrayEvaluator` |
| `net.IPNet` | `CIDREvaluator` / `CIDRArrayEvaluator` |

All implement the `Evaluator` interface:

```go
type Evaluator interface {
    Eval(ctx *Context) interface{}
    IsDeterministicFor(field Field) bool
    GetField() string
    IsStatic() bool  // true for constant / scalar evaluators
}
```

Evaluators carry a `Weight` constant that controls evaluation order for short-circuit optimisation (`FunctionWeight=5`, `InArrayWeight=10`, `HandlerWeight=50`, `PatternWeight=80`, `RegexpWeight=100`, `IteratorWeight=2000`).

#### Rule compilation lifecycle

```
text  →  ast.ParsingContext.ParseRule()  →  *ast.Rule
      →  eval.NewRule()                 →  *eval.Rule (unparsed)
      →  rule.GenEvaluator(model)       →  *RuleEvaluator (closure)
```

`RuleEvaluator.Eval(ctx)` is a `BoolEvalFnc` (`func(*Context) bool`) that runs the fully compiled rule.

**Partial evaluation** (`rule.PartialEval(ctx, field)`) fixes all fields except one and evaluates, enabling approver / discarder derivation without a real event.

#### Context and caching

`Context` is the per-event evaluation scratch pad. It holds:
- The current `Event`
- Per-field value caches (`StringCache`, `IPNetCache`, `IntCache`, `BoolCache`)
- Iterator `Registers` and `RegisterCache`
- `MatchingSubExprs` — which sub-expressions matched (used for structured match metadata)

Use `ContextPool` to amortise allocations in the hot path.

#### Field value types

`FieldValueType` constants describe what kind of comparison value a field carries: `ScalarValueType`, `GlobValueType`, `PatternValueType`, `RegexpValueType`, `BitmaskValueType`, `VariableValueType`, `IPNetValueType`, `RangeValueType`.

#### Variables

`SECLVariable` / `Variable` / `ScopedVariable` / `MutableVariable` provide a TTL-backed, optionally scope-pinned mutable state that rules can read and write via `${varname}` syntax. `VariableStore` holds global variables; scoped variables are keyed by `ScopeHashKey`.

### Policy and rule management (`rules`)

#### Core types

| Type | Purpose |
|---|---|
| `RuleDefinition` | YAML-deserialisable rule descriptor (ID, expression, tags, actions, `every`, `combine`, …) |
| `MacroDefinition` | YAML-deserialisable macro descriptor |
| `Policy` | A loaded policy file; holds `[]PolicyRule` and `[]PolicyMacro` |
| `PolicyRule` | Wraps `RuleDefinition` with load state, filter type, policy provenance info |
| `RuleSet` | The runtime registry: rules indexed by event type into `RuleBucket`s |
| `RuleSetListener` | Callback interface — `RuleMatch` and `EventDiscarderFound` |

#### Policy merging

Two `CombinePolicy` constants control how duplicate rule/macro IDs from different policy files are reconciled: `MergePolicy` (array values concatenated) and `OverridePolicy` (later definition wins, with granular `OverrideOptions.Fields` control: `all`, `actions`, `every`, `tags`, `product_tags`).

Five `InternalPolicyType` values control internal policy priority: `DefaultPolicyType`, `CustomPolicyType`, `RemediationPolicyType`, `BundledPolicyType`, `SelftestPolicyType`.

#### Approvers

`Approvers` (`map[eval.Field]FilterValues`) are derived from rules by partial evaluation. They represent the minimal set of field constraints that, if violated, allow the kernel to discard an event before it reaches userspace. This is the main performance lever for high-event-rate systems.

`ApproverStats` tracks per-field approver usage and identifies rules that break approver derivation.

#### Rule actions

`Action` wraps an `ActionDefinition` and an optional `FilterEvaluator` (a compiled SECL expression that gates whether the action fires). Actions compile their filter with `Action.CompileFilter()` and their scope field with `Action.CompileScopeField()`.

#### `RuleSet` API highlights

```go
rs.AddMacros(...)              // register macros before rules
rs.AddRules(...)               // compile and bucket rules
rs.Evaluate(event)             // evaluate event against all matching rules
rs.GetApprovers(fields)        // derive kernel approvers
rs.NotifyRuleMatch(ctx, rule)  // trigger RuleSetListeners
rs.ListRuleIDs()
rs.GetRuleByID(id)
rs.GetPolicies()
```

---

## Usage in the codebase

The typical flow (from `pkg/security/rules/engine.go`) is:

1. `PolicyLoader` reads YAML policy files via `PolicyProvider` implementations (directory, Remote Config, bundled).
2. `PolicyLoader.LoadPolicies()` produces merged `[]*Policy`, applying `RuleIDFilter`, `AgentVersionFilter`, and `SECLRuleFilter` host-condition filters.
3. A `RuleSet` is created with `NewRuleSet(model, eventCtor, opts, evalOpts)`, passing a `model.Model` (the concrete CWS event model from `pkg/security/secl/model`).
4. Macros are registered first with `rs.AddMacros(...)`, then rules with `rs.AddRules(...)`. Each rule's SECL expression is parsed by `ast.ParsingContext` and compiled by `eval.Rule.GenEvaluator(model)` into a `BoolEvalFnc` closure.
5. The resulting `RuleSet` is set atomically on the `RuleEngine`.
6. At rule-set load time, `rs.GetApprovers(kfilters.GetCapabilities())` performs partial evaluation to derive the minimal field-value constraints that can be pushed to the kernel (eBPF map entries). See [probe.md](probe.md) for how `EBPFProbe.ApplyRuleSet` translates these into `kfilters` eBPF map writes.
7. At runtime, `probe.Probe` calls `rs.Evaluate(event)` for each kernel event via `RuleEngine.HandleEvent`. The evaluator runs compiled `BoolEvalFnc` closures against the event's `eval.Context`; matching rules trigger the registered `RuleSetListener`s (which emit security signals, perform rule actions, etc.).
8. When no rule can match a specific inode or PID, `RuleSetListener.EventDiscarderFound` is called and `RuleEngine` forwards this to `probe.OnNewDiscarder`, which writes the discarder into the kernel eBPF map to suppress future events without context switching.

### Variables in rules

SECL variables (`${varname}`) allow stateful, rule-driven data sharing:

- **Built-in variables** (`model.SECLVariables`): `${process.pid}` and `${builtins.uuid4}` are always available.
- **Rule-set variables**: defined via `set` actions on rules; scoped globally or per-container/process.
- **Mutable variables**: updated at event time; later rules in the same evaluation pass see the updated value via the `VariableStore`.

### Writing a new SECL field (model extension)

1. Add the field to the relevant event struct in `pkg/security/secl/model/model_unix.go` with `field:"<dotted.path>"` struct tags and an optional `handler:<MethodName>` tag for lazy resolution.
2. Add the corresponding `Resolve<Field>` method to `FieldHandlers` in `pkg/security/probe/field_handlers_ebpf.go`.
3. Run `go generate` inside `pkg/security/secl/model/` to regenerate `accessors_unix.go`, `field_accessors_unix.go`, and `field_handlers_unix.go`.
4. Use the new field path in a SECL rule expression; it will be compiled and evaluated automatically.

External consumers of just the compiler (e.g., the `accessors` code generator in `pkg/security/generators/accessors/`) import only `pkg/security/secl` for its AST and eval packages.

---

## Related documentation

| Doc | Description |
|-----|-------------|
| [secl-model.md](secl-model.md) | Concrete `Model`/`Event` implementation: all CWS event structs, `FieldHandlers`, generated accessor files. Defines the dotted field paths (`open.file.path`, `process.uid`, …) that SECL rules reference. |
| [secl-compiler.md](secl-compiler.md) | Deep-dive into `ast/` (parser, grammar, node types) and `eval/` (evaluator types, `Context`, `ContextPool`, operator weights, iterator registers). |
| [secl-rules.md](secl-rules.md) | `RuleSet`, `PolicyLoader`, `Approvers`, rule/macro filter types, and `PolicyProvider` constants used by the rule engine. |
| [rules.md](rules.md) | `RuleEngine` in `pkg/security/rules`: how `RuleSet` is built from loaded policies and applied to the probe. |
| [probe.md](probe.md) | `pkg/security/probe`: consumes `rs.GetApprovers()` to install eBPF kfilters, and calls `rs.Evaluate(event)` in the hot path via `RuleEngine.HandleEvent`. |
| [security.md](security.md) | Top-level CWS integration hub — wires probe, rule engine, agent, and all sub-systems together. |
