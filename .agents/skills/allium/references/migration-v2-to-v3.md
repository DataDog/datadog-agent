# Migrating from Allium v2 to v3

This guide covers every change between Allium v2 and v3. It is written for both humans reviewing the release and LLMs tasked with upgrading v2 specifications.

If you are an LLM migrating a v2 spec, read this document in full, then work through the checklist at the end. The checklist verifies completeness but does not repeat the syntax rules and examples you will need from the sections above it.

---

## What changed

Version 3 adds six capabilities and one enforcement change to the language. All v2 constructs retain their meaning. The changes are:

1. **Transition graphs** (`transitions field_name { ... }`), authoritative opt-in declarations of valid lifecycle transitions for enum status fields.
2. **State-dependent field presence** (`when` clause on field declarations), tying a field's presence to the entity's lifecycle state rather than using static `?` optionality.
3. **Derived value `when` propagation**, automatic inference of `when` sets for derived values computed from state-dependent fields.
4. **Backtick-quoted enum literals** (`` `de-CH-1996` ``), allowing external standard values that fall outside snake_case conventions.
5. **Ordered collection semantics** (`Sequence<T>`), distinguishing ordered from unordered collections and restricting `.first`/`.last` to ordered types.
6. **Black box function syntax for collection operations**, reserving dot-method syntax for built-in operations and requiring free-standing call syntax for domain-specific collection operations.

Because the first five changes are additive, a v2 spec that does not use custom dot-methods on collections is valid v3 once the version marker is updated. Specs that use custom dot-methods need rewriting (see [Enforcement change](#enforcement-change-black-box-collection-operations) below).

---

## Required changes

### 1. Update the version marker

The first line of every `.allium` file must change from version 2 to version 3.

v2:
```
-- allium: 2
```

v3:
```
-- allium: 3
```

### 2. Rewrite custom dot-methods on collections (if present)

V3 reserves dot-method syntax on collections for built-in operations only. The full set of built-in dot-methods is: `.count`, `.any()`, `.all()`, `.first`, `.last`, `.unique`, `.add()`, `.remove()`. Any other dot-method call on a collection is now a checker error.

If your v2 spec used dot-method syntax for domain-specific collection operations, rewrite them to free-standing black box function syntax with the collection as the first argument:

v2:
```
events.filter(e => e.recent)
copies.grouped_by(r => r.output_payloads)
pending.min_by(e => e.offset)
```

v3:
```
filter(events, e => e.recent)
grouped_by(copies, r => r.output_payloads)
min_by(pending, e => e.offset)
```

If your v2 spec did not use custom dot-methods, no rewriting is needed.

---

## New constructs available in v3

These constructs did not exist in v2. They are optional: a migrated spec does not need to use them. But they are available, and specs that would benefit from them should adopt them.

### Transition graphs

V2 entities derived their valid transitions implicitly from the rules that operated on them. V3 adds an opt-in mechanism for declaring the valid transitions explicitly, inside the entity body.

```
entity Order {
    status: pending | confirmed | shipped | delivered | cancelled

    transitions status {
        pending -> confirmed
        confirmed -> shipped
        shipped -> delivered
        pending -> cancelled
        confirmed -> cancelled
        terminal: delivered, cancelled
    }
}
```

When a transition graph is declared, it is authoritative: rules whose `ensures` clauses produce transitions not in the graph are validation errors. The checker also enforces that every non-terminal state has at least one outbound edge and that every declared edge is witnessed by at least one rule.

**Syntax rules:**
- The graph lives inside the entity body, below the field it governs, introduced by `transitions field_name`.
- Each line in the block is a directed edge: `from_state -> to_state`.
- Terminal states are declared with `terminal:` followed by a comma-separated list. Absence of outbound edges does not imply terminal status; the declaration is required.
- Every value on the enum field must appear in at least one edge or as a terminal. Every value in the graph must exist on the field. Drift is a hard error.
- Entities with multiple status fields use independent single-field graphs.
- Entities without a declared graph continue to derive transition validity from rules alone, with no change in checker behaviour. The checker does not suggest adding graphs to entities that lack them.

**When to add transition graphs to a migrated spec:** when the entity has a lifecycle field with well-understood valid transitions and you want the checker to enforce them. Particularly valuable for entities where incorrect transitions would be hard to detect from rule inspection alone.

### State-dependent field presence (`when` clause)

V2 used `?` to mark fields that might be absent. In lifecycle entities, many fields are absent in some states and guaranteed present in others, but `?` cannot express this distinction. V3 adds a `when` clause on field declarations that ties presence to lifecycle state.

```
entity Document {
    status: active | deleted
    deleted_at: Timestamp when status = deleted
    deleted_by: User when status = deleted

    transitions status {
        active -> deleted
        deleted -> active
        terminal: deleted
    }
}
```

Fields without `when` are present in all states. Fields with `when` are present only when the named status field holds one of the listed values. The `when` clause references a single status field; that field must have a `transitions` block.

**Presence and absence obligations.** The checker enforces obligations at transition boundaries:

- **Entering** the `when` set (source state outside, target state inside): the rule must set the field.
- **Leaving** the `when` set (source state inside, target state outside): the rule must clear the field (set to `null`).
- **Moving within** or **outside** the `when` set: no obligation.

```
rule SoftDelete {
    when: SoftDelete(document, actor)
    requires: document.status = active
    ensures:
        document.status = deleted
        document.deleted_at = now        -- entering when set: must set
        document.deleted_by = actor      -- entering when set: must set
}

rule RestoreDocument {
    when: RestoreDocument(document)
    requires: document.status = deleted
    ensures:
        document.status = active
        document.deleted_at = null       -- leaving when set: must clear
        document.deleted_by = null       -- leaving when set: must clear
}
```

Accessing a `when`-qualified field without a `requires` guard narrowing to a qualifying state is an error.

**`?` and `when` are orthogonal.** `reviewer_notes: String? when review = approved | rejected` means the field exists in those states but may be null within them. `?` is genuine optionality; `when` is lifecycle-dependent presence. A field may carry both.

**When to adopt `when` clauses in a migrated spec:** when existing `?` fields are not genuinely optional but are instead absent before a certain lifecycle stage and guaranteed present after it. The soft-delete pattern, order fulfilment pipelines and invitation workflows are common candidates. Replace the `?` with a `when` clause referencing the appropriate status values, and add a `transitions` block if one does not already exist.

### Derived value `when` propagation

Derived values computed from `when`-qualified fields automatically inherit the intersection of their inputs' `when` sets:

```
entity Order {
    status: pending | confirmed | shipped | delivered
    shipped_at: Timestamp when status = shipped | delivered
    delivery_confirmed_at: Timestamp when status = delivered

    transitions status {
        pending -> confirmed
        confirmed -> shipped
        shipped -> delivered
        terminal: delivered
    }

    -- Inferred: when status = delivered
    -- (intersection of {shipped, delivered} and {delivered})
    days_in_transit: delivery_confirmed_at - shipped_at
}
```

The checker infers this; the author does not declare it. An author may optionally annotate a derived value with an explicit `when` clause as documentation. When present, the checker verifies it matches the inferred set. A mismatch is an error.

### Backtick-quoted enum literals

V2 enum literals were restricted to snake_case. V3 allows backtick quoting for values that reference external standards with non-snake_case characters:

```
enum InterfaceLanguage { en | de | fr | `de-CH-1996` | es | `zh-Hant-TW` | `sr-Latn` }
enum CacheDirective { `no-cache` | `no-store` | `must-revalidate` | `max-age` }
```

**Syntax rules:**
- Backtick-quoted literals are values, not identifiers. They participate in equality comparison and assignment.
- The checker does not apply case convention rules inside backticks. Comparison is byte-exact after UTF-8 encoding.
- Quoted and unquoted forms are distinct values with no implicit normalisation: `de_ch_1996` and `` `de-CH-1996` `` are different values.
- Backtick-quoted literals are permitted in enum declarations (named and inline), literal comparisons in rules and `ensures` clauses.
- They are not permitted in identifier positions (field names, entity names, rule names, etc.) and cannot appear in arithmetic expressions.

**When to use backtick-quoted literals in a migrated spec:** when enum values reference external standards (BCP 47 language tags, MIME types, HTTP cache directives, currency codes) whose canonical form uses hyphens, dots, mixed case or leading digits. Replace any workaround encodings (underscore-substituted forms) with the standard's canonical form in backticks.

### Ordered collection semantics

V2 treated all collections uniformly. V3 introduces a type distinction between ordered and unordered collections:

- `Set<T>` — unordered collection of unique items (unchanged from v2)
- `List<T>` — ordered collection, declared explicitly as a compound field type on entities
- `Sequence<T>` — ordered collection produced by ordered relationships and their projections. A subtype of `Set`: assignable where an unordered collection is expected, but not the reverse

**Syntax rules:**
- `.first` and `.last` are restricted to ordered collections (`Sequence` or `List<T>`). Using them on a `Set` is a warning in v3, becoming a hard error in the next version.
- `.unique` deduplicates a collection but always produces an unordered `Set`, even when the source is ordered.
- Set arithmetic (`+`, `-`) on ordered collections produces unordered results. The checker reports an error if the result is used where an ordered collection is expected.
- `for item in collection:` iterates in declared order when the source is a `Sequence` or `List<T>`. When the source is a `Set`, iteration order is unspecified.
- Projections preserve ordering: if the source is a `Sequence`, `where` filtering and `-> field` extraction produce a `Sequence` in the same relative order.

**When to adopt ordered semantics in a migrated spec:** when the order of items in a collection carries domain meaning (priority lists, attempt sequences, ranked preferences). If order does not matter, continue using `Set<T>`.

### Enforcement change: black box collection operations

V3 reserves dot-method syntax on collections for the built-in set: `.count`, `.any()`, `.all()`, `.first`, `.last`, `.unique`, `.add()`, `.remove()`. The checker rejects any other dot-method call on a collection.

Domain-specific collection operations must use free-standing black box function syntax with the collection as the first argument:

```
-- Built-in: dot-method syntax (unchanged)
interviewers.any(i => i.can_solo)
confirmations.all(c => c.status = confirmed)
slots.count

-- Domain-specific: free-standing syntax (enforced in v3)
filter(events, e => e.recent)
grouped_by(copies, r => r.output_payloads)
min_by(pending, e => e.offset)
flatMap(groups, g => g.deferred_events)
```

This was the recommended convention in v2 but was not enforced. V3 makes it a hard error.

---

## Naming convention additions

V3 does not add new naming conventions. All naming rules are unchanged from v2. Backtick-quoted enum literals are exempt from case convention rules (the checker does not apply snake_case rules inside backticks).

---

## Migration checklist

Use this checklist when upgrading a v2 spec to v3. Items marked **required** must be done. Items marked **optional** should be done when the spec would benefit.

- [ ] **Required.** Change `-- allium: 2` to `-- allium: 3` on the first line.
- [ ] **Required if applicable.** Rewrite any custom dot-method calls on collections to free-standing black box function syntax.
- [ ] **Optional.** If entities have lifecycle fields with well-understood valid transitions, add `transitions field_name { ... }` blocks.
- [ ] **Optional.** If fields are typed `?` but are structurally absent before a lifecycle stage and present after it, replace `?` with a `when` clause and ensure the referenced status field has a `transitions` block.
- [ ] **Optional.** If enum values reference external standards with non-snake_case characters, replace workaround encodings with backtick-quoted canonical forms.
- [ ] **Optional.** If collection order carries domain meaning, adopt `List<T>` for explicitly ordered fields and note that relationships producing `Sequence` will have ordering semantics when the ordered relationship declaration syntax is introduced.
- [ ] **Optional.** Review `.first` and `.last` usage on `Set` collections. These produce a warning in v3 and will become errors in the next version. Replace with explicit ordering or remove.

---

## Quick reference

| V2 | V3 | Change type |
|----|-----|-------------|
| `-- allium: 2` | `-- allium: 3` | Required |
| No transition graph syntax | `transitions field_name { from -> to; terminal: ... }` | Additive |
| `deleted_at: Timestamp?` (static optionality) | `deleted_at: Timestamp when status = deleted` (state-dependent) | Additive |
| No derived value `when` propagation | Derived values inherit intersected `when` sets from inputs | Additive |
| Enum literals restricted to snake_case | Backtick-quoted literals for external standards (`` `de-CH-1996` ``) | Additive |
| All collections treated uniformly | `Set<T>` (unordered), `List<T>` and `Sequence<T>` (ordered) | Additive |
| Custom dot-methods on collections permitted | Dot-methods reserved for built-ins; custom ops use free-standing syntax | Enforcement |
