# Migrating from Allium v1 to v2

This guide covers every change between Allium v1 and v2. It is written for both humans reviewing the release and LLMs tasked with upgrading v1 specifications.

If you are an LLM migrating a v1 spec, read this document in full, then work through the checklist at the end. The checklist verifies completeness but does not repeat the syntax rules and examples you will need from the sections above it.

---

## What changed

Version 2 adds six capabilities to the language. None of the existing v1 syntax was removed or altered; every v1 construct still means what it meant before. The changes are:

1. **Contract references** (`demands`, `fulfils`) in surfaces, for expressing programmatic integration contracts with typed signatures and invariants.
2. **Module-level contracts** (`contract`), direction-agnostic obligation declarations that surfaces reference via a `contracts:` clause.
3. **Guidance annotations** (`@guidance`) in rules, contracts and surfaces, for non-normative implementation advice.
4. **Expression-bearing invariants** (`invariant Name { expression }`), machine-readable assertions at top-level and entity-level scope.
5. **The `implies` operator**, a boolean operator available in all expression contexts.
6. **Config composition** — config parameter defaults that reference imported module parameters by qualified name, with arithmetic expressions for derived defaults.

Because all changes are additive, a v1 spec is valid v2 once the version marker is updated. No existing syntax needs rewriting.

---

## Required changes

### 1. Update the version marker

The first line of every `.allium` file must change from version 1 to version 2. This is the only change required for every spec.

v1:
```
-- allium: 1
```

v2:
```
-- allium: 2
```

### 2. Adjust section order (only when adopting new constructs)

V2 introduces two new sections. The full section order is now:

```
use declarations
Given
External Entities
Value Types
Contracts              ← new, between Value Types and Enumerations
Enumerations
Entities and Variants
Config
Defaults
Rules
Invariants             ← new, between Rules and Actor Declarations
Actor Declarations
Surfaces
Deferred Specifications
Open Questions
```

Empty sections are still omitted. No existing sections moved: the two new sections slot between existing ones. If your v1 spec does not adopt contracts or expression-bearing invariants, no section headers need adding and the existing order is already correct.

If you add contracts, place the section header after Value Types:

```
------------------------------------------------------------
-- Contracts
------------------------------------------------------------
```

If you add expression-bearing invariants, place the section header after Rules:

```
------------------------------------------------------------
-- Invariants
------------------------------------------------------------
```

---

## New constructs available in v2

These constructs did not exist in v1. They are optional: a migrated spec does not need to use them. But they are available, and specs that would benefit from them should adopt them.

### Contract references in surfaces (`demands`, `fulfils`)

V1 surfaces had `exposes`, `provides`, `guarantee`, `related` and `timeout`. V2 adds a `contracts:` clause for programmatic integration contracts.

Use `demands` when the surface requires something from the counterpart. Use `fulfils` when the surface supplies something to the counterpart. Each entry references a module-level `contract` declaration by name.

```
contract DeterministicEvaluation {
    evaluate: (event_name: String, payload: ByteArray) -> EventOutcome

    @invariant Determinism
        -- For identical inputs, evaluate must produce
        -- byte-identical outputs across all instances.

    @guidance
        -- Avoid allocating during evaluation where possible.
}

contract EventSubmitter {
    submit: (key: String, event_name: String, payload: ByteArray) -> EventSubmission
}

surface DomainIntegration {
    facing framework: FrameworkRuntime

    contracts:
        demands DeterministicEvaluation
        fulfils EventSubmitter
}
```

**Syntax rules:**
- `contracts:` entries use `demands` or `fulfils` followed by a PascalCase contract name.
- Each contract name may appear at most once per surface.
- Referenced contract names must resolve to a `contract` declaration in scope.
- Contract bodies contain typed signatures and `@`-prefixed annotations (`@invariant`, `@guidance`). No entity, value, enum or variant declarations.

**When to add contract references to an existing v1 surface:** if the surface describes a boundary between code (framework and module, service and plugin, API and consumer) rather than between a user and an application, and the contract involves typed operations with specific properties.

### Module-level contracts

Contracts are declared at module level in the Contracts section. Surfaces reference them via the `contracts:` clause.

```
-- Module-level declaration (in the Contracts section)
contract Codec {
    serialize: (value: Any) -> ByteArray
    deserialize: (bytes: ByteArray) -> Any

    @invariant Roundtrip
        -- deserialize(serialize(value)) produces a value
        -- equivalent to the original for all supported types.
}

contract EventSubmitter {
    submit: (event: DomainEvent) -> Acknowledgement
}

-- Surface references contracts with direction markers
surface DataPipeline {
    facing processor: ProcessorModule

    contracts:
        demands Codec
        fulfils EventSubmitter
}
```

**Syntax rules:**
- `contracts:` entries use `demands` or `fulfils` followed by a contract name.
- Contract identity is determined by module-qualified name. Same-named contracts from different modules are a structural error.
- Contracts are imported atomically via `use`. Partial imports are not supported.

### Guidance annotations in rules

Rules can now end with an `@guidance` annotation containing non-normative implementation advice.

```
-- v1: no guidance clause
rule ExpireInvitation {
    when: invitation: Invitation.expires_at <= now
    requires: invitation.status = pending
    ensures: invitation.status = expired
}

-- v2: guidance added as final annotation
rule ExpireInvitation {
    when: invitation: Invitation.expires_at <= now
    requires: invitation.status = pending
    ensures: invitation.status = expired

    @guidance
        -- Expire in a background job rather than blocking the
        -- request path. Batch expiration where possible.
}
```

**Syntax rules:**
- `@guidance` must appear after all structural clauses and after all other annotations in its containing construct.
- Content is opaque prose using indented comment syntax (`--`). The checker does not parse it.
- `@guidance` is also valid inside contracts and at surface level. In contracts it provides implementation advice scoped to that contract's operations. At surface level it provides advice about the boundary as a whole.
- The `@` sigil marks prose annotations: constructs whose structure (placement, ordering) the checker validates, but whose content it does not evaluate. The same sigil convention applies to `@invariant` and `@guarantee`.

### Expression-bearing invariants

V1 had no mechanism for machine-readable assertions over entity state. V2 adds expression-bearing invariants at two scopes.

**Top-level invariants** assert system-wide properties. They go in the new Invariants section after Rules:

```
invariant NonNegativeBalance {
    for account in Accounts:
        account.balance >= 0
}

invariant UniqueEmail {
    for a in Users:
        for b in Users:
            a != b implies a.email != b.email
}
```

**Entity-level invariants** assert properties scoped to a single entity. They go inside entity declarations alongside fields:

```
entity Account {
    balance: Decimal
    credit_limit: Decimal
    status: active | frozen | closed

    invariant SufficientFunds {
        balance >= -credit_limit
    }

    invariant FrozenAccountsCannotTransact {
        status = frozen implies pending_transactions.count = 0
    }
}
```

**Syntax rules:**
- Expression-bearing invariants use `invariant Name { expression }` (no `@`, braces).
- Prose-only invariants in contracts use `@invariant Name` (with `@`, no colon). These are distinct constructs.
- Invariant names are PascalCase.
- Expressions must be pure: no `.add()`, `.remove()`, `.created()`, no trigger emissions, no `now`.
- `for x in Collection:` inside an invariant body is a universal quantifier (all elements must satisfy).

**When to add invariants to a migrated spec:** if the spec has properties that should always hold (non-negative balances, uniqueness constraints, referential integrity) and those properties are currently implicit or expressed only in prose comments.

### The `implies` operator

V2 adds `implies` to the expression language. `a implies b` is equivalent to `not a or b`. It has the lowest precedence of any boolean operator, binding looser than `and` and `or`.

`implies` is available in all expression contexts, not only invariants. It reads naturally in `requires` guards, derived boolean values and `if` conditions:

```
-- In a requires clause
requires: user.role = admin implies user.mfa_enabled

-- In a derived value
is_compliant: is_verified implies documents.count > 0

-- In an invariant
invariant ClosedAccountsEmpty {
    for account in Accounts:
        account.status = closed implies account.balance = 0
}
```

### Config parameter references and expressions

V1 config parameters could only have literal defaults. V2 allows defaults to reference parameters from imported modules, and to use arithmetic expressions.

```
use "./core.allium" as core

config {
    -- Literal default (valid in both v1 and v2)
    max_retries: Integer = 3

    -- Qualified reference default (v2 only)
    batch_size: Integer = core/config.batch_size

    -- Expression default (v2 only)
    extended_timeout: Duration = core/config.base_timeout * 2
    buffer_size: Integer = core/config.batch_size + 10
    retry_limit: Integer = max_retries - 1
}
```

**Syntax rules:**
- Qualified references use the form `alias/config.param_name`.
- Arithmetic operators: `+`, `-`, `*`, `/` with standard precedence. Parentheses for explicit precedence.
- Both local and qualified references are valid in expressions.
- The config reference graph must be acyclic.
- Type compatibility: Integer with Integer, Duration with Duration (for `+`/`-`), Duration with Integer (for `*`/`/`), Integer with Duration (for `*` only), Decimal with Decimal, Decimal with Integer (for `*`/`/`), Integer with Decimal (for `*` only). Scalar multiplication is commutative (`2 * core/config.timeout` and `core/config.timeout * 2` are both valid). Addition and subtraction require matching types.
- Expressions resolve once at config resolution time, not dynamically.

**When to use config references in a migrated spec:** when a consuming spec duplicates a library spec's config value, or derives a value from it (double the timeout, batch size minus a buffer).

---

## Naming convention additions

V2 extends PascalCase to two new constructs:

| Construct | Convention | Example |
|-----------|-----------|---------|
| Contract names | PascalCase | `Codec` |
| Invariant names | PascalCase | `NonNegativeBalance` |

All other naming conventions are unchanged from v1.

---

## Migration checklist

Use this checklist when upgrading a v1 spec to v2. Items marked **required** must be done. Items marked **optional** should be done when the spec would benefit.

- [ ] **Required.** Change `-- allium: 1` to `-- allium: 2` on the first line.
- [ ] **Required if adopting new constructs.** Verify section order matches v2 (Contracts after Value Types, Invariants after Rules). If neither section is present, existing order is already correct.
- [ ] **Optional.** If the spec has surfaces describing code-to-code boundaries, consider declaring `contract` blocks and referencing them via a `contracts:` clause with `demands`/`fulfils`.
- [ ] **Optional.** If rules or surfaces have implementation-specific notes in comments, consider moving them into `@guidance` annotations (valid as the final annotation in rules and at surface level).
- [ ] **Optional.** If the spec has properties that must always hold (uniqueness, non-negativity, referential constraints), express them as `invariant Name { expression }` blocks.
- [ ] **Optional.** If any expression (invariants, requires, derived values) would read more clearly with implication logic, use the `implies` operator.
- [ ] **Optional.** If config defaults duplicate or derive from imported module parameters, use qualified references and expressions.

---

## Quick reference

| V1 | V2 | Change type |
|----|-----|-------------|
| `-- allium: 1` | `-- allium: 2` | Required |
| Sections: Value Types → Enumerations | Sections: Value Types → **Contracts** → Enumerations | Required (if contracts present) |
| Sections: Rules → Actor Declarations | Sections: Rules → **Invariants** → Actor Declarations | Required (if invariants present) |
| No contract references in surfaces | `contracts:` clause with `demands`/`fulfils` entries | Additive |
| No module-level contracts | `contract Name { ... }` in Contracts section | Additive |
| No `@guidance` annotation | `@guidance` in rules (final annotation), contracts and surfaces | Additive |
| No expression-bearing invariants | `invariant Name { expression }` at top-level and entity-level | Additive |
| No `implies` operator | `a implies b` (lowest boolean precedence) | Additive |
| Config defaults are literals only | Config defaults can reference `alias/config.param` and use arithmetic | Additive |
