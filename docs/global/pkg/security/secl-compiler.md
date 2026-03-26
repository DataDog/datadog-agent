> **TL;DR:** The SECL compiler translates human-readable security rule expressions into efficient zero-allocation Go closures via a two-stage pipeline: an `ast/` parser built on participle and an `eval/` code generator that produces typed evaluator functions run against live kernel events.

# pkg/security/secl/compiler

## Purpose

The SECL (Security Event and Condition Language) compiler translates human-readable security rule expressions into efficient, zero-allocation Go closures that can be evaluated against live kernel events. It is split into two sub-packages:

- `ast/` — lexer and parser: turns a SECL expression string into an abstract syntax tree.
- `eval/` — code generator and runtime: walks the AST, compiles it into typed evaluator functions, and executes those functions against event data.

These packages are the foundation for all CWS (Cloud Workload Security) detection rules in `pkg/security/secl/rules` and `pkg/security/rules`.

## Key elements

### Key types

#### `ParsingContext` (`ast/`)

`ParsingContext` is the root factory for all parse operations. It holds three `participle` parser instances (for rules, macros, and standalone expressions) and an optional rule cache.

```go
pc := ast.NewParsingContext(withRuleCache bool)
```

The EBNF lexer embedded in `NewParsingContext` defines all SECL token types: `CIDR`, `IP`, `Variable` (`${...}`), `FieldReference` (`%{...}`), `Duration`, `Regexp` (`r"..."`), `Pattern` (`~"..."`), `Ident`, `String`, `Int`, and punctuation.

| Method | Returns | Description |
|--------|---------|-------------|
| `ParseRule(expr string)` | `*Rule, error` | Parses a full rule expression, with optional cache lookup. |
| `ParseMacro(expr string)` | `*Macro, error` | Parses a macro body (expression, array, or primary). |
| `ParseExpression(expr string)` | `*Expression, error` | Parses a standalone boolean expression. |

#### AST node types

The grammar is a standard expression grammar. Key node types, from root to leaf:

| Node | Role |
|------|------|
| `Rule` | Root of a rule AST; wraps `BooleanExpression`. Carries the original source string in `Expr`. |
| `Macro` | Root of a macro AST; can be an `Expression`, `Array`, or `Primary`. |
| `Expression` | `Comparison (LogicalOp BooleanExpression)?` |
| `Comparison` | `ArithmeticOperation (ScalarComparison \| ArrayComparison)?` |
| `ScalarComparison` | `ScalarOp Comparison` — operators: `==`, `!=`, `<`, `<=`, `>`, `>=`, `=~`, `!~` |
| `ArrayComparison` | `ArrayOp Array` — operators: `in`, `not in`, `allin` |
| `ArithmeticOperation` | `BitOperation ((+ \| -) BitOperation)*` |
| `BitOperation` | `Unary ((\& \| \| \| ^) Unary)*` |
| `Unary` | `(! \| not \| - \| ^)? Primary` |
| `Primary` | Leaf: identifier, CIDR, IP, integer, variable, field reference, string, pattern, regexp, duration, or sub-expression. |
| `Array` | Right-hand side of `in`/`allin`: CIDR, variable, ident, or a literal list of strings/patterns/regexps/CIDRs/numbers/idents. |

### Key interfaces

#### `Model` interface (`eval/`)

```go
type Model interface {
    GetEvaluator(field Field, regID RegisterID, offset int) (Evaluator, error)
    ValidateField(field Field, value FieldValue) error
    ValidateRule(rule *Rule) error
    NewEvent() Event
    GetFieldRestrictions(field Field) []EventType
}
```

Each event model (e.g. `pkg/security/secl/model`) implements `Model`. `GetEvaluator` is the primary extension point: it maps a dotted field path (e.g. `process.file.path`) to a typed `Evaluator` closure backed by the concrete event struct.

### Key functions

#### `Rule` and `Macro`

| Type | Key methods | Description |
|------|-------------|-------------|
| `Rule` | `NewRule(id, expr, pc, opts, tags...)` | Parses and stores the AST. Does not compile yet. |
| | `GenEvaluator(model)` | Compiles the AST into a `RuleEvaluator`. Must be called before `Eval`. |
| | `Eval(ctx *Context) bool` | Evaluates the rule against the event in `ctx`. |
| | `PartialEval(ctx, field) bool` | Evaluates the rule treating `field` as the only free variable (used for kernel-side filtering). |
| | `GetFields() []Field` | All fields referenced by the rule (including macro fields). |
| | `GetEventType() (EventType, error)` | The event type the rule applies to. |
| `RuleEvaluator` | `Eval BoolEvalFnc` | The compiled boolean closure. |
| | `GetFields() []Field` | Fields referenced by this evaluator. |
| | `PartialEval(ctx, field)` | Per-field partial evaluation. |
| `Macro` | `NewMacro(id, expr, model, pc, opts)` | Parses and compiles a macro inline. |
| | `NewStringValuesMacro(id, values, opts)` | Creates a macro from a static string slice without parsing. |
| | `GenEvaluator(expr, model)` | Compiles the macro AST. |

#### Evaluator types

Each expression node compiles to one of these typed evaluators, all implementing the `Evaluator` interface:

| Type | Return type |
|------|-------------|
| `BoolEvaluator` | `bool` |
| `IntEvaluator` | `int` |
| `StringEvaluator` | `string` |
| `StringArrayEvaluator` | `[]string` |
| `StringValuesEvaluator` | `*StringValues` (compiled string set with glob/regexp support) |
| `IntArrayEvaluator` | `[]int` |
| `BoolArrayEvaluator` | `[]bool` |
| `CIDREvaluator` | `net.IPNet` |
| `CIDRArrayEvaluator` | `[]net.IPNet` |
| `CIDRValuesEvaluator` | `*CIDRValues` |

#### Context and caching

`Context` carries the event being evaluated and typed field caches that avoid redundant resolver calls within a single evaluation pass:

```go
type Context struct {
    Event         Event
    StringCache   map[Field][]string
    IPNetCache    map[Field][]net.IPNet
    IntCache      map[Field][]int
    BoolCache     map[Field][]bool
    Registers     map[RegisterID]int       // iterator register values
    RegisterCache map[RegisterID]*RegisterCacheEntry
    IteratorCountCache map[string]int
    // ...
}
```

`ContextPool` (backed by `sync.Pool`) should be used in hot paths to avoid repeated allocation:

```go
pool := eval.NewContextPool()
ctx := pool.Get(event)
matched := rule.Eval(ctx)
pool.Put(ctx)
```

### Configuration and build flags

#### Opts and stores

`Opts` is passed to `NewRule`/`NewMacro` and controls compilation behaviour:

| Field | Description |
|-------|-------------|
| `MacroStore` | Collection of pre-compiled `Macro` objects available by ID within rule expressions. |
| `VariableStore` | Named `SECLVariable` values (dynamic, mutable) usable as `${name}` in expressions. |
| `Constants` | Static named values available at compile time. |
| `LegacyFields` | Field alias map for backward compatibility. |
| `Telemetry` | Optional evaluation telemetry (counters per rule). |

#### Iterator registers

Fields with array subscript syntax `field[x]` introduce a register variable `x`. At evaluation time the rule is executed for each index `0..N-1` (capped at `maxRegisterIteration = 100`); the rule matches if any iteration returns `true`. Only one iterator register per rule is currently supported.

#### Operator weights

The compiler assigns weights to expression nodes to guide evaluation ordering. Higher-weight nodes are evaluated last to minimise unnecessary work:

| Operator | Weight |
|----------|--------|
| Function call | 5 |
| `in` array | 10 |
| Field handler | 50 |
| Glob pattern | 80 |
| Regexp | 100 |
| Pattern array | 1 000 |
| Iterator | 2 000 |

## Usage

Typical usage in the rules engine (`pkg/security/rules/engine.go`):

```go
pc := ast.NewParsingContext(true) // with cache
opts := &eval.Opts{}
opts.WithMacroStore(&eval.MacroStore{})

// compile a macro
macro, _ := eval.NewMacro("my_macro", `["bash", "sh"]`, model, pc, opts)
opts.AddMacro(macro)

// compile a rule
rule, _ := eval.NewRule("my_rule", `process.file.name in my_macro`, pc, opts)
rule.GenEvaluator(model)

// evaluate at runtime
ctx := eval.NewContext(event)
if rule.Eval(ctx) {
    // rule matched
}
```

The `ast` package is also used standalone when rules need to be inspected without compiling (e.g. extracting field names for policy generation).
