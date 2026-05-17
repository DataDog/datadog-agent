# Language reference

## File structure

An Allium specification file (`.allium`) begins with a language version marker, followed by these sections in order:

```
-- allium: 3
-- Comments use double-dash
-- use declarations (optional)

------------------------------------------------------------
-- Given
------------------------------------------------------------

-- Entity instances this module operates on (optional)

------------------------------------------------------------
-- External Entities
------------------------------------------------------------

-- Entities managed outside this specification

------------------------------------------------------------
-- Value Types
------------------------------------------------------------

-- Structured data without identity (optional section)

------------------------------------------------------------
-- Contracts
------------------------------------------------------------

-- Reusable obligation contracts referenced by surfaces

------------------------------------------------------------
-- Enumerations
------------------------------------------------------------

-- Named enumerations shared across entities (optional section)

------------------------------------------------------------
-- Entities and Variants
------------------------------------------------------------

-- Entities managed by this specification, plus their variants

------------------------------------------------------------
-- Config
------------------------------------------------------------

-- Configurable parameters for this specification

------------------------------------------------------------
-- Defaults
------------------------------------------------------------

-- Default entity instances

------------------------------------------------------------
-- Rules
------------------------------------------------------------

-- Behavioural rules organised by flow

------------------------------------------------------------
-- Invariants
------------------------------------------------------------

-- System-wide and entity-level property assertions

------------------------------------------------------------
-- Actor Declarations
------------------------------------------------------------

-- Entity types that can interact with surfaces

------------------------------------------------------------
-- Surfaces
------------------------------------------------------------

-- Boundary contracts between parties

------------------------------------------------------------
-- Deferred Specifications
------------------------------------------------------------

-- References to detailed specs defined elsewhere

------------------------------------------------------------
-- Open Questions
------------------------------------------------------------

-- Unresolved design decisions
```

### Formatting

Indentation is significant. Blocks opened by a colon (`:`) after `for`, `if`, `else`, `ensures`, `exposes`, `provides`, `contracts`, `related` and `timeout` are delimited by consistent indentation relative to the parent clause. Named blocks opened by a keyword and PascalCase name followed by `{ ... }` (`contract`, `invariant`) use brace delimiters. Prose annotations prefixed with `@` (`@invariant`, `@guarantee`, `@guidance`) are followed by indented comment lines that form their body. `contracts:` entries use `demands`/`fulfils` modifiers followed by a contract name. Comments use `--`. Commas may be used as field separators for single-line entity and value type declarations; newlines are the standard separator for multi-line declarations.

### Naming conventions

- **PascalCase**: entity names, variant names, rule names, trigger names, actor names, surface names, contract names, invariant names (`InterviewSlot`, `CandidateSelectsSlot`, `DeterministicEvaluation`, `Purity`)
- **snake_case**: field names, config parameters, derived values, enum literals, relationship names (`expires_at`, `max_login_attempts`, `pending`). Enum literals that reference external standards may use backtick-quoted forms containing characters outside the snake_case set (`` `de-CH-1996` ``, `` `no-cache` ``)
- **Entity collections**: natural English plurals of the entity name (`Users`, `Documents`, `Candidacies`)

---

## Module given

A `given` block declares the entity instances a module operates on. All rules in the module inherit these bindings.

```
given {
    pipeline: HiringPipeline
    calendar: InterviewCalendar
}
```

Rules then reference `pipeline.status`, `calendar.available_slots`, etc. without ambiguity about what they refer to.

Not every module needs a `given` block. Rules scoped by triggers on domain entities (e.g., `when: invitation: Invitation.expires_at <= now`) get their entities from the trigger binding. `given` is for specs where rules operate on shared instances that exist once per module scope, such as a pipeline, a catalog or a processing engine.

`given` bindings must reference entity types declared in the same module or imported via `use`. Imported module instances are accessed via qualified names (`scheduling/calendar`) and do not need to appear in the local `given` block. Modules that operate only on imported instances may omit the `given` block entirely.

This is distinct from surface `context`, which binds a parametric scope for a boundary contract (e.g., `context assignment: SlotConfirmation`).

---

## Contracts

A `contract` declaration defines a named, direction-agnostic obligation at module level. Surfaces reference contracts in a `contracts:` clause using `demands` (the counterpart must implement) or `fulfils` (this surface supplies) direction markers.

### Declaration syntax

```
contract Codec {
    serialize: (value: Any) -> ByteArray
    deserialize: (bytes: ByteArray) -> Any

    @invariant Roundtrip
        -- deserialize(serialize(value)) produces a value
        -- equivalent to the original for all supported types.

    @guidance
        -- Implementations should handle versioned payloads
        -- by inspecting a version prefix in the byte array.
}
```

Contract bodies contain typed signatures and annotations (`@invariant`, `@guidance`). Entity, value, enum and variant declarations are prohibited inside contracts. Types referenced in signatures must be declared at module level or imported via `use`.

### Referencing contracts in surfaces

Surfaces reference contracts in a `contracts:` clause. Each entry uses `demands` or `fulfils` to indicate the direction of the obligation:

```
surface DomainIntegration {
    facing framework: FrameworkRuntime

    contracts:
        demands Codec
        demands DeterministicEvaluation
        fulfils EventSubmitter

    @guarantee AllOperationsIdempotent
        -- All operations exposed by this surface are safe to retry.
}
```

`@guarantee` is a surface-level prose assertion about the boundary as a whole; see [Surfaces](#surfaces) for the full clause reference.

The surface inherits all signatures, invariants and guidance from each referenced contract. Each contract name may appear at most once per surface.

### Contract identity

Contract identity is determined by module-qualified name, consistent with entity and value type identity rules. Two contracts are "the same" if and only if they resolve to the same module-qualified declaration. Composed surfaces referencing the same module-qualified contract are not in conflict. Surfaces referencing identically named contracts from different modules are a structural error.

### Imports

Contracts are importable across modules via `use`, following the same coordinate system as entity imports. Contract imports are atomic: a contract is imported as a complete unit. Partial imports (importing individual signatures from a contract) are not supported.

### No type parameters

Contracts do not support type parameters. Signatures may use `Any` where type generality is needed, with invariants expressing the type relationships in prose.

---

## Entities

### External entities

Entities referenced but managed outside this specification:

```
external entity Role {
    title: String
    required_skills: Set<Skill>
    location: Location
}
```

External entities define their structure but not their lifecycle. The specification checker will warn when external entities are referenced, reminding that another spec or system governs them.

External entities can also serve as **type placeholders**: an entity with minimal or no fields that the consuming spec substitutes with a concrete type. This enables reusable patterns where the library spec depends on an abstraction and the consumer provides the implementation.

```
-- In a comments library spec
external entity Commentable {}

entity Comment {
    parent: Commentable
    ...
}

-- The consuming spec provides its own entity as the Commentable
```

The consuming spec maps its entity to the placeholder by using it wherever the library expects the placeholder type. This is dependency inversion at the spec level: the library depends on the abstraction, the consumer supplies the concrete type.

### Internal entities

```
entity Candidacy {
    -- Fields (required)
    candidate: Candidate
    role: Role
    status: pending | active | completed | cancelled

    -- Relationships (navigate to related entities)
    invitation: Invitation with candidacy = this
    slots: InterviewSlot with candidacy = this

    -- Projections (filtered subsets)
    confirmed_slots: slots where status = confirmed
    pending_slots: slots where status = pending

    -- Derived (computed values)
    is_ready: confirmed_slots.count >= 3
    has_expired: invitation.expires_at <= now
}
```

### Value types

Structured data without identity. No lifecycle, compared by value not reference. Use for concepts such as time ranges and addresses.

```
value TimeRange {
    start: Timestamp
    end: Timestamp

    -- Derived
    duration: end - start
}

value Location {
    name: String
    timezone: String
    country: String?
}
```

Value types have no identity, are immutable and are embedded within entities. Entities have identity, lifecycle and rules that govern them.

### Sum types

Sum types (discriminated unions) specify that an entity is exactly one of several alternatives.

```
entity Node {
    path: Path
    kind: Branch | Leaf              -- discriminator field
}

variant Branch : Node {
    children: List<Node?>            -- variant-specific field
}

variant Leaf : Node {
    data: List<Integer>              -- variant-specific fields
    log: List<Integer>
}
```

A sum type has three parts: a **discriminator field** whose type is a pipe-separated list of variant names, **variant declarations** using `variant X : BaseEntity`, and **variant-specific fields** that only exist for that variant. Variants inherit all fields from the base entity; the discriminator is set automatically on creation.

**Distinguishing sum types from enums:** unquoted lowercase values are enum literals (`status: pending | active`), unquoted capitalised values are variant references (`kind: Branch | Leaf`). Backtick-quoted values are always enum literals regardless of case (`` `de-CH-1996` ``). The validator checks that unquoted capitalised names correspond to `variant` declarations.

**Creating variant instances** — always via the variant name, not the base:

```
ensures: MentionNotification.created(user: recipient, comment: comment, mentioned_by: author)
-- Not: Notification.created(...)  -- Error: must specify which variant
```

**Type guards** narrow an entity to a specific variant, enabling access to its fields. They appear in `requires` clauses (guarding the entire rule) and `if` expressions (guarding a branch):

```
-- requires guard: entire rule assumes Leaf
rule ProcessLeaf {
    when: ProcessNode(node)
    requires: node.kind = Leaf
    ensures: Results.created(data: node.data + node.log)
}

-- if guard: branch-level narrowing
rule ProcessNode {
    when: ProcessNode(node)
    ensures:
        if node.kind = Branch:
            for child in node.children: ProcessNode(child)
        else:
            Results.created(data: node.data + node.log)
}
```

Accessing variant-specific fields outside a type guard is an error. Sum types guarantee exhaustiveness (all variants declared upfront), mutual exclusivity (exactly one variant), type safety (variant fields only within guards) and automatic discrimination (set on creation).

A `.created` trigger on the base entity fires when any variant is created. The bound variable holds the specific variant instance, and type guards can narrow it:

```
rule HandleNotification {
    when: notification: Notification.created
    ensures:
        if notification.kind = MentionNotification:
            ...
}
```

Use sum types when variants have fundamentally different data or behaviour. Do not use when simple status enums suffice or variants share most of their structure.

### Field types

**Primitive types:**
- `String` — text
- `Integer` — whole numbers. Underscores are ignored in numeric literals for readability: `100_000_000`
- `Decimal` — numbers with fractional parts (use for money, percentages)
- `Boolean` — `true` or `false`
- `Timestamp` — point in time. The built-in value `now` evaluates to the current timestamp. Its evaluation model depends on context: in derived values, `now` re-evaluates on each read (making the derived value volatile); in ensures clauses, `now` is bound to the rule execution timestamp (a snapshot); in temporal triggers, `now` is the evaluation timestamp with fire-once semantics.
- `Duration` — length of time, written as a numeric literal with a unit suffix: `.seconds`, `.minutes`, `.hours`, `.days`, `.weeks`, `.months`, `.years` (e.g., `24.hours`, `7.days`, `30.seconds`). Both singular and plural forms are valid: `1.hour` and `24.hours`.

Primitive types have no properties or methods. For domain-specific string types (email addresses, URLs), use value types or plain `String` fields with descriptive names. For operations on primitives beyond the built-in operators, use black box functions (e.g., `length(password)`, `hash(password)`).

**Compound types:**
- `Set<T>` — unordered collection of unique items
- `List<T>` — ordered collection (use when order matters). A compound field type declared explicitly on entities
- `Sequence<T>` — ordered collection produced by ordered relationships and their projections. `Sequence` is a subtype of `Set`: an ordered collection is assignable where an unordered one is expected, but not the reverse. `List<T>` is a field type you declare explicitly; `Sequence` is the collection type the checker infers when a relationship is ordered. Both carry ordering semantics, but they occupy different positions in the grammar
- `T?` — optional (may be absent). Reserved for genuinely optional fields: a user's nickname, a note that may or may not exist. For fields whose presence depends on lifecycle state, use a `when` clause instead (see below).

**Checking for absent values:**
```
requires: request.reminded_at = null      -- field is absent/unset
requires: request.reminded_at != null     -- field has a value
```

`null` represents the absence of a value for optional fields.

`field = null` and `field != null` are presence checks, not comparisons. `field = null` is true when the field is absent; `field != null` is true when the field has a value. Comparisons with null produce false: `null <= now` is false, `null > 0` is false. Arithmetic with null produces null: `null + 1.day` is null. This means temporal triggers on optional fields (e.g., `when: user: User.next_digest_at <= now`) do not fire when the field is absent.

**State-dependent field presence (`when` clause):**

A field declaration may carry a `when` clause tying its presence to lifecycle state:

```
entity Order {
    status: pending | confirmed | shipped | delivered | cancelled
    customer: Customer
    total: Money
    tracking_number: String when status = shipped | delivered
    shipped_at: Timestamp when status = shipped | delivered
    delivery_confirmed_at: Timestamp when status = delivered

    transitions status {
        pending -> confirmed
        confirmed -> shipped
        shipped -> delivered
        terminal: delivered, cancelled
    }
}
```

Fields without `when` are present in all states. Fields with `when` are present only when the named status field holds one of the listed values. The `when` clause references a single status field; that field must have a `transitions` block.

`?` and `when` are orthogonal. `reviewer_notes: String? when review = approved | rejected` means the field exists in those states but may be null within them. `?` is genuine optionality; `when` is lifecycle-dependent presence. A field may carry both.

Entities with multiple status fields use the qualified form to disambiguate:

```
entity Document {
    status: draft | published | archived
    review: pending | approved | rejected

    transitions status { ... }
    transitions review { ... }

    published_at: Timestamp when status = published | archived
    reviewer_notes: String when review = rejected
}
```

Each `when` clause references one status field. Compound conditions across multiple status fields (`when status = published and review = rejected`) are not supported; use invariants for cross-field constraints.

The `when` keyword appears in three syntactic positions: rule triggers (`when: TriggerCondition`, with colon), surface and provides guards (`Action(...) when condition`, without colon, after an action), and field declarations (`field: Type when status = value`, without colon, after a type). The grammar is unambiguous at each position. Rule triggers are identified by the colon. Surface guards follow an action or related clause. Field `when` clauses follow a type declaration.

**Presence and absence obligations.** Obligations fire when a rule crosses the boundary of a field's `when` set:

- **Entering** (source state outside `when` set, target state inside): the rule must set the field.
- **Leaving** (source state inside `when` set, target state outside): the rule must clear the field.
- **Moving within** (both states inside): no obligation. The field is already present and remains present; the rule may update it but need not.
- **Moving outside** (both states outside): no obligation.

```
entity Document {
    status: active | deleted
    deleted_at: Timestamp when status = deleted
    deleted_by: User when status = deleted

    transitions status {
        active -> deleted
        deleted -> active
        terminal: active
    }
}

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

**Access without guard.** Accessing a `when`-qualified field without a `requires` guard narrowing to a qualifying state is an error:

```
-- Error: tracking_number requires status in {shipped, delivered}
rule BadAccess {
    when: SomeEvent(order)
    ensures: Label.created(tracking: order.tracking_number)
}

-- Valid: requires narrows to a qualifying state
rule GenerateLabel {
    when: GenerateLabel(order)
    requires: order.status = shipped
    ensures: Label.created(tracking: order.tracking_number)
}
```

**Convergent transitions.** If two rules reach the same state and the entity requires a field at that state, both must set it:

```
rule CancelByCustomer {
    when: CustomerCancels(order)
    requires: order.status = pending
    ensures:
        order.status = cancelled
        order.cancelled_at = now         -- required: entering when set
        order.cancelled_by = customer    -- required: entering when set
}

rule CancelByTimeout {
    when: order: Order.created_at + 48.hours <= now
    requires: order.status = pending
    ensures:
        order.status = cancelled
        order.cancelled_at = now         -- required: entering when set
        -- Error: cancelled_by not set
}
```

**Enumerated types (inline):**
```
status: pending | confirmed | declined | expired
```

**Named enumerations:**
```
enum Recommendation { strong_yes | yes | no | strong_no }
enum DayOfWeek { monday | tuesday | wednesday | thursday | friday | saturday | sunday }
```

Named enumerations define a reusable set of values. Declare them in the Enumerations section of the file. Reference them as field types: `recommendation: Recommendation`. Inline enums (`status: pending | active`) are equivalent but anonymous; use named enums when the same set of values appears in multiple fields or entities.

**Backtick-quoted enum literals:**

Enum values that reference external standards may contain characters outside the snake_case set (hyphens, dots, mixed case, leading digits). Enclose these in backticks:

```
enum InterfaceLanguage { en | de | fr | `de-CH-1996` | es | `zh-Hant-TW` | `sr-Latn` }
enum CacheDirective { `no-cache` | `no-store` | `must-revalidate` | `max-age` }
```

Backtick-quoted literals are values, not identifiers. They participate in equality comparison and assignment. The checker does not apply case convention rules inside backticks. Comparison is byte-exact after UTF-8 encoding; authors are responsible for using the canonical form from the external standard. Quoted and unquoted forms are distinct values with no implicit normalisation: `de_ch_1996` and `` `de-CH-1996` `` are different values.

Backtick-quoted literals are permitted in enum declarations (named and inline), literal comparisons in rules and `ensures` clauses. They are not permitted in identifier positions (field names, entity names, variant names, config parameter names, derived value names, rule/trigger/invariant names). They cannot appear in arithmetic expressions.

Inline enums are anonymous: they have no type identity. Two inline enum fields cannot be compared with each other, whether on the same entity or across entities; the checker reports an error. Use a named enum when values need to be compared across fields. Named enums are distinct types: a field typed `Recommendation` cannot be compared with a field typed `DayOfWeek`, even if they happen to share a literal.

This catches a common mistake when tracking previous state:

```
-- Error: cannot compare two inline enum fields
entity Order {
    status: pending | shipped | delivered
    previous_status: pending | shipped | delivered
}
requires: order.status != order.previous_status    -- checker error

-- Fix: extract a named enum
enum OrderStatus { pending | shipped | delivered }
entity Order {
    status: OrderStatus
    previous_status: OrderStatus
}
requires: order.status != order.previous_status    -- valid
```

### Transition graphs

A transition graph declares the valid lifecycle transitions for an enum status field. When present, the graph is authoritative: rules whose `ensures` clauses produce transitions not in the graph are validation errors. Entities without a declared graph continue to derive transition validity from rules alone, with no change in checker behaviour.

The graph lives inside the entity body, below the field it governs, introduced by the `transitions` keyword followed by the field name:

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

**Edges.** Each line in the block is a directed edge using `->`: `from_state -> to_state`. The graph declares which transitions are possible (structural topology), not when or why they occur (conditional logic). Conditions remain in rules. The graph says "this edge exists"; the rule says "this edge fires under these conditions". A complete understanding of lifecycle behaviour requires reading both the graph and the rules.

**Terminal states.** Terminal states are declared with a `terminal:` clause listing the terminal values. This is the sole mechanism for terminal marking. Absence of outbound edges does not imply terminal status; the checker requires explicit declaration.

**Completeness obligations.** When a graph is declared, the checker enforces two obligations:
1. Every non-terminal state has at least one outbound edge in the graph.
2. Every declared edge is witnessed by at least one rule whose `requires`/`ensures` pair can produce that transition.

The converse (every rule transition appears in the graph) is enforced by the authoritative relationship itself. The graph is identified by entity and field name (e.g. `Order.status`) in error messages; no separate name declaration is needed.

**Enum reference, not redeclaration.** The graph references enum values already declared on the field. The checker enforces exact correspondence: every value in the graph must exist on the field, and every field value must appear in at least one edge or as a terminal. Drift is a hard error.

**Opt-in.** The checker does not emit warnings or suggestions about missing graphs on entities that lack them. The construct earns adoption through demonstrated value, not through tooling pressure.

**Multiple status fields.** Entities with multiple status fields use independent single-field graphs. Cross-field constraints are expressed through invariants.

**Generality.** Transition graphs currently target enum status fields. The syntax is designed to extend to variant discriminators and other lifecycle-bearing fields in future versions without structural changes.

**Interaction with state-dependent fields.** When a transition graph is declared, the checker uses its structure alongside `when` clauses on field declarations to enforce presence and absence obligations at each transition (see [Field types](#field-types)) and to verify that `when`-qualified fields are only accessed within qualifying state guards.

**Entity references:**
```
candidate: Candidate
role: Role
```

### Relationships

Always use singular entity names; the relationship name indicates plurality:

```
-- One-to-one (singular relationship name)
invitation: Invitation with candidacy = this

-- One-to-many (plural relationship name, but singular entity name)
slots: InterviewSlot with candidacy = this
feedback_requests: FeedbackRequest with interview = this

-- Self-referential
replies: Comment with reply_to = this
```

The `with X = this` syntax declares a relationship by naming the field on the related entity that points back. `this` refers to the enclosing entity instance. The syntax is the same whether the relationship is one-to-one, one-to-many or self-referential.

The relationship name determines the cardinality:

- **Singular name** (e.g., `invitation`) — at most one related entity. The value is the entity instance, or `null` if none exists. Equivalent to `T?`. If multiple entities match a singular relationship, the specification is in error and the checker should report it.
- **Plural name** (e.g., `slots`) — zero or more related entities. The value is a collection, empty if none exist.

Relationships currently produce `Set` (unordered). The declaration syntax for ordered relationships (which would produce `Sequence`) is pending a follow-up ALP. The semantic model for ordered collections is defined; see [Collection operations](#collection-operations) for the type-level rules.

### Projections

Named filtered views of relationships:

```
-- Simple status filter
confirmed_slots: slots where status = confirmed

-- Multiple conditions
active_requests: feedback_requests where status = pending and requested_at > cutoff

-- Projection with mapping
confirmed_interviewers: confirmations where status = confirmed -> interviewer
```

The `-> field` syntax extracts a field from each matching entity. When the extracted field is optional (`T?`), null values are excluded from the result: the projection produces `Set<T>`, not `Set<T?>`.

Projections preserve ordering. If the source collection is a `Sequence`, the projection result is also a `Sequence` in the same relative order. This applies to both `where` filtering and `-> field` extraction. Field extraction on ordered collections retains duplicates (two source elements navigating to the same target produce two entries in sequence order); use `.unique` to deduplicate, which produces an unordered `Set`.

### Derived values

Computed from other fields. Always read-only and automatically updated.

```
-- Boolean derivations
is_valid: interviewers.any(i => i.can_solo) or interviewers.count >= 2
is_expired: expires_at <= now
all_responded: pending_requests.count = 0

-- Value derivations
time_remaining: deadline - now

-- Parameterised derived values
can_use_feature(f): f in plan.features
has_permission(p): p in role.effective_permissions
```

Parameters are locally scoped to the expression. Parameterised derived values cannot reference module `given` bindings or global state; they operate only on the entity's own fields and their parameter. No side effects.

---

## Rules

Rules define behaviour: what happens when triggers occur.

### Rule structure

```
rule RuleName {
    when: TriggerCondition

    let binding1 = expression      -- bindings can appear before requires

    requires: Precondition1
    requires: Precondition2

    let binding2 = expression      -- or between requires and ensures

    ensures: Postcondition1
    ensures: Postcondition2

    @guidance                       -- optional, always last
        -- Non-normative implementation advice.
}
```

| Clause | Purpose |
|--------|---------|
| `when` | What triggers this rule |
| `for` | Iterate: apply the rule body for each element in a collection |
| `let` | Local variable bindings (can appear anywhere after `when`) |
| `requires` | Preconditions that must be true (rule fails if not met) |
| `ensures` | What becomes true after the rule executes |
| `@guidance` | Non-normative implementation advice (optional, always last) |

Place `let` bindings where they make the rule most readable, typically just before the clause that first uses them.

### Derived value `when` propagation

Derived values computed from `when`-qualified fields inherit the intersection of their inputs' `when` sets:

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

The checker infers this; the author does not declare it. If the intersection is empty, the derived value is unreachable and the checker reports an error.

An author may optionally annotate a derived value with an explicit `when` clause as documentation:

```
days_in_transit: delivery_confirmed_at - shipped_at when status = delivered
```

When present, the checker verifies it matches the inferred set. A mismatch is an error. When absent, the inferred set applies silently. The checker exports inferred `when` sets as structured data alongside state-level summaries.

**Cross-entity access.** Accessing a `when`-qualified field on a related entity requires a guard narrowing the related entity's status to a qualifying state. `candidacy.order.tracking_number` requires that the rule's context narrows `order.status` to a qualifying state.

### Rule-level iteration

A `for` clause applies the rule body once per element in a collection. The binding variable is available in all subsequent clauses.

```
rule ProcessDigests {
    when: schedule: DigestSchedule.next_run_at <= now
    for user in Users where notification_setting.digest_enabled:
        let settings = user.notification_setting
        ensures: DigestBatch.created(user: user, ...)
}
```

The `where` keyword filters the collection, consistent with projection syntax. The indented body contains the rule's `let`, `requires` and `ensures` clauses scoped to each element.

This is the same `for x in collection:` construct used in ensures blocks and surfaces. The body inherits the constraints of its enclosing context: at rule level it wraps `let`, `requires` and `ensures` clauses; inside an ensures block it wraps postconditions (state changes, entity creation, trigger emissions, removal assertions); inside a surface it wraps the items permitted by the enclosing clause (`exposes`, `provides` or `related`).

### Multiple rules for the same trigger

When multiple rules share a trigger, their `requires` clauses determine which fires. If preconditions overlap such that multiple rules could match simultaneously, this is a spec ambiguity. The specification checker should warn when rules with the same trigger have overlapping preconditions.

### Trigger types

**External stimulus** — action from outside the system:
```
when: AdminApprovesInterviewers(admin, suggestion, interviewers, times)
when: CandidateSelectsSlot(invitation, slot)
```

**Optional parameters** use the `?` suffix:
```
when: InterviewerReportsNoInterview(interviewer, interview, reason, details?)
```

**State transition** — entity changed state:
```
when: interview: Interview.status transitions_to scheduled
when: confirmation: SlotConfirmation.status transitions_to confirmed
```

The variable before the colon binds the entity that triggered the transition. `transitions_to` fires when a field transitions to the specified value from a different value, not on initial entity creation (use `.created` for that). It is valid for enum fields, boolean fields and entity reference fields. When a transition graph is declared for the field, only transitions in the graph are structurally valid; rules producing other transitions are validation errors.

**State becomes** — entity has a value, whether by creation or transition:
```
when: interview: Interview.status becomes scheduled
```

`becomes` fires both when an entity is created with the specified value and when a field transitions to that value from a different value. Like `transitions_to`, it is valid for enum fields, boolean fields and entity reference fields. It is equivalent to writing a `transitions_to` rule and a `.created` rule with a `requires` guard, combined into a single trigger. Use `becomes` when the rule should apply regardless of how the entity arrived at the state. Use `transitions_to` when the rule should only apply to transitions (e.g., sending a "rescheduled" notification that doesn't apply on initial creation).

**Temporal** — time-based condition:
```
when: invitation: Invitation.expires_at <= now
when: interview: Interview.slot.time.start - 1.hour <= now
when: request: FeedbackRequest.requested_at + 24.hours <= now
```

Temporal triggers use explicit `var: Type` binding, the same as state transitions and entity creation. The binding names the entity instance and its type. Temporal triggers fire once when the condition becomes true. Always include a `requires` clause to prevent re-firing:
```
rule InvitationExpires {
    when: invitation: Invitation.expires_at <= now
    requires: invitation.status = pending  -- prevents re-firing
    ensures: invitation.status = expired
}
```

**Derived condition becomes true:**
```
when: interview: Interview.all_feedback_in
when: slot: InterviewSlot.is_valid
```

Derived condition triggers fire when the value transitions from false to true, the same semantics as temporal triggers. If the derived value could revert to false and become true again, include a `requires` clause to prevent re-firing, just as with temporal triggers.

**Entity creation** — fires when a new entity is created:
```
when: batch: DigestBatch.created
when: mention: CommentMention.created
```

**Chained from another rule's trigger emission:**
```
when: AllConfirmationsResolved(candidacy)
```

A rule chains from another by subscribing to a trigger emission. The emitting rule includes the event in an ensures clause:

```
ensures: AllConfirmationsResolved(candidacy: candidacy)
```

The receiving rule subscribes via its `when` clause. This uses the same syntax as external stimulus triggers, but the stimulus comes from another rule rather than from outside the system.

### Preconditions (requires)

Preconditions must be true for the rule to execute. If not met, the trigger is rejected.

```
requires: invitation.status = pending
requires: not invitation.is_expired
requires: slot in invitation.slots
requires: interviewer in interview.interviewers
requires:
    interviewers.count >= 2
    or interviewers.any(i => i.can_solo)
```

**Precondition failure behaviour:**
- For external stimulus triggers: The action is rejected; caller receives an error
- For temporal/derived triggers: The rule simply does not fire; no error
- For chained triggers: The chain stops; previous rules' effects still apply

### Local bindings (let)

```
let confirmation = SlotConfirmation{slot, interviewer}
let time_until = interview.slot.time.start - now
let is_urgent = time_until < 24.hours
let is_modified =
    interviewers != suggestion.suggested_interviewers
    or proposed_times != suggestion.suggested_times
```

### Discard bindings

Use `_` where a binding is required syntactically but the value is not needed. Multiple `_` bindings in the same scope do not conflict.

```
when: _: LogProcessor.last_flush_check + flush_timeout_hours <= now
when: SomeEvent(_, slot)
for _ in items: Counted(batch)
```

### Postconditions (ensures)

Postconditions describe what becomes true. They are declarative assertions about the resulting state, not imperative commands.

In state change assignments (`entity.field = expression`), the expression on the right references pre-rule field values. This avoids circular definitions: `user.count = user.count + 1` means the resulting count equals the original count plus one. Conditions within ensures blocks (`if` guards, creation parameters, trigger emission parameters) reference the resulting state as defined by the state changes. A `let` binding within an ensures block introduces a name visible to all subsequent statements in that block.

Worked example: suppose `account.balance` is 100 before the rule fires.

```
ensures: account.balance = account.balance + 50       -- RHS reads pre-rule value: 100 + 50 = 150
ensures:
    if account.balance > 120:                          -- condition reads resulting state: 150 > 120, true
        Notification.created(account: account, type: high_balance)
```

The assignment reads 100 (the pre-rule value). The `if` guard reads 150 (the resulting state after the assignment).

Common mistake: assuming `if` guards in ensures read pre-rule values. Suppose `order.status` is `pending` before the rule fires.

```
ensures: order.status = shipped
ensures:
    if order.status = pending:                             -- WRONG: reads resulting state (shipped), so this is false
        Notification.created(to: order.customer, template: order_pending_reminder)
    if order.status = shipped:                             -- reads resulting state (shipped), so this is true
        Notification.created(to: order.customer, template: order_shipped)
```

The author likely meant "if the order was pending before we changed it". But the `if` guard inside ensures reads the resulting state, not the pre-rule state. To test pre-rule values, use a `let` binding or `requires` clause before the ensures block.

Ensures clauses have four forms:

**State changes** — modify an existing entity's fields:
```
ensures: slot.status = booked
ensures: invitation.status = accepted
ensures: candidacy.retry_count = candidacy.retry_count + 1
ensures: user.locked_until = null              -- clearing an optional field
```

Setting an optional field to `null` asserts the field becomes absent. Only valid for fields typed as optional (`T?`).

**Entity creation** — create a new entity using `.created()`:
```
ensures: Interview.created(
    candidacy: invitation.candidacy,
    slot: slot,
    interviewers: slot.confirmed_interviewers,
    status: scheduled
)

ensures: Email.created(
    to: candidate.email,
    template: interview_invitation,
    data: { slots: slots }
)

ensures: CalendarInvite.created(
    attendees: interviewers + candidate,
    time: slot.time,
    duration: interview_type.duration
)
```

Entity creation uses `.created()` exclusively. Domain meaning lives in entity names and rule names, not in creation verbs. `Email.created(...)` not `Email.sent(...)`.

When creating entities that need to be referenced later in the same ensures block, use explicit `let` binding:
```
ensures:
    let slot = InterviewSlot.created(time: time, candidacy: candidacy, status: pending)
    for interviewer in interviewers:
        SlotConfirmation.created(slot: slot, interviewer: interviewer)
```

A `let` binding within an ensures block is visible to all subsequent statements in that block, including nested `for` loops. It does not leak outside the ensures block.

**Trigger emission** — emit a named event that other rules can chain from:
```
ensures: CandidateInformed(
    candidate: candidacy.candidate,
    about: slot_unavailable,
    data: { available_alternatives: remaining_slots }
)

ensures: UserMentioned(user: mention.user, comment: comment, mentioned_by: author)
ensures: FeatureUsed(workspace: workspace, feature: feature, by: user)
```

Trigger emissions are observable outcomes, not entity creation. They have no `.created()` call and are referenced by other rules' `when` clauses. Parameter values follow normal expression resolution: bound names are resolved first, then enum literals if the parameter has a declared type on the receiving rule. Bare identifiers that resolve to neither a binding nor an enum literal are a checker warning.

**Entity removal:**
```
ensures: not exists target_membership
ensures: not exists CommentMention{comment, user}
```

See [Existence](#existence) in the expression language for the full syntax including bulk removal and the distinction from soft delete.

**Bulk updates:**
```
ensures:
    for s in invitation.proposed_slots:
        s.status = cancelled
```

**Conditional outcomes:**
```
ensures:
    if candidacy.retry_count < 2:
        candidacy.status = pending_scheduling
    else:
        candidacy.status = scheduling_stalled
        Notification.created(...)
```

---

## Expression language

### Navigation

```
-- Field access
interview.status
candidate.email

-- Relationship traversal
interview.feedback_requests
candidacy.slots

-- Chained navigation
interview.candidacy.candidate.email
feedback_request.interview.slot.time

-- Optional navigation (short-circuits to null if left side is null)
inherits_from?.effective_permissions
reply_to?.author

-- Null coalescing (provides default when left side is null)
identity.timezone ?? "UTC"
inherits_from?.effective_permissions ?? {}

-- State-dependent fields: ?. is not needed for when-qualified fields
-- when the requires clause narrows to a qualifying state
order.tracking_number         -- valid when requires: order.status = shipped

-- Self-reference
this                                        -- the instance being defined or identified
replies: Comment with reply_to = this       -- all Comments whose reply_to is this entity
```

`this` refers to the instance of the enclosing type. It is valid in two contexts:

- **Entity declarations**: `this` is the current entity instance. Available in relationships, projections and derived values.
- **Actor `identified_by` expressions**: `this` is the entity instance being tested for actor membership (see [Actor declarations](#actor-declarations)).

### Join lookups

For entities that connect two other entities (join tables):

```
let confirmation = SlotConfirmation{slot, interviewer}
let feedback_request = FeedbackRequest{interview, interviewer}
```

Curly braces with field names look up the specific instance where those fields match. Any number of fields can be specified. Each name serves as both the field name on the entity and the local variable whose value is matched. The lookup must match at most one entity; if the fields do not uniquely identify a single instance, the specification is ambiguous and the checker should report an error. If no entity matches, the binding is null. Use `exists` to test whether a lookup matched before accessing fields on it; accessing fields on a null binding is an error.

When the local variable name differs from the field name, use the explicit form:

```
let actor_membership = WorkspaceMembership{user: actor, workspace: workspace}
let share = ResourceShare{resource: resource, user: inviter}
requires: not exists User{email: new_email}
```

### Collection operations

```
-- Count
slots.count
pending_requests.count

-- Membership
slot in invitation.slots
interviewer in interview.interviewers

-- Any/All (always use explicit lambda)
interviewers.any(i => i.can_solo)
confirmations.all(c => c.status = confirmed)

-- Filtering (in projections and expressions)
slots where status = confirmed
requests where status in {submitted, escalated}

-- Iteration (introduces a scope block)
for slot in slots: ...

-- Set mutation (ensures-only, modifies a relationship)
interviewers.add(new_interviewer)
interviewers.remove(leaving_interviewer)

-- Set arithmetic (expression-level, produces a new set)
all_permissions: permissions + inherited_permissions
removed_mentions: old_mentions - new_mentions

-- First/last (ordered collections only: Sequence or List<T>)
attempts.first
attempts.last

-- Deduplicate (produces unordered Set)
ordered_interviewers.unique
```

`.first` and `.last` are restricted to ordered collections (`Sequence` or `List<T>`). Using them on a `Set` is a warning in the current version, becoming a hard error in the next version.

`.unique` deduplicates a collection. Because deduplication discards positional information, the result is always an unordered `Set`, even when the source is ordered.

`for item in collection:` iterates in declared order when the source is a `Sequence` or `List<T>`. When the source is a `Set`, iteration order is unspecified.

`.add()` and `.remove()` are ensures-only mutations on a relationship. Set `+` and `-` are expression-level operations that produce new sets without mutating anything. When applied to ordered collections (`Sequence` or `List<T>`), `+` and `-` produce unordered results (`Set<T>`). The checker reports the type change if the result is used where ordering is expected.

Dot-method syntax on collections is reserved for built-in operations. The built-in dot-methods are: `.count`, `.any()`, `.all()`, `.first`, `.last`, `.unique`, `.add()`, `.remove()`. The checker rejects any dot-method call on a collection whose name is not in this set. Domain-specific collection operations use free-standing black box function syntax with the collection as the first argument (see [Black box functions](#black-box-functions)).

### Comparisons

```
status = pending
status != proposed
count >= 2
expires_at <= now
time_until < 24.hours
status in {confirmed, declined, expired}
provider not in user.linked_providers
```

`{value1, value2, ...}` is a set literal used with `in` and `not in` for membership tests. This is the same set literal syntax used in field declarations and expressions.

### Arithmetic

```
candidacy.retry_count + 1
interview.slot.time.start - now
feedback_request.requested_at + 24.hours
now + 7.days
recent_failures.count / config.window_sample_size
price * quantity
```

Four operators: `+`, `-`, `*`, `/`. Standard precedence: `*` and `/` bind tighter than `+` and `-`. Use parentheses to override. Arithmetic involving null produces null (e.g., `null + 1.day` is null). Derived values computed from optional fields are implicitly optional.

### Boolean logic

```
interviewers.count >= 2 or interviewers.any(i => i.can_solo)
invitation.status = pending and not invitation.is_expired
not (a or b)  -- equivalent to: not a and not b
```

`and` and `or` short-circuit left to right. If the left operand of `or` is true, the right operand is not evaluated; if the left operand of `and` is false, the right operand is not evaluated. This permits patterns like `not exists x or not x.is_valid`, where the right side is only reached when `x` exists.

### Implication

```
account.status = closed implies account.balance = 0
not user.is_verified implies user.permissions.count = 0
```

`implies` has the lowest precedence of any boolean operator, binding looser than `and` and `or`. `a implies b` is equivalent to `not a or b`. Available in all expression contexts. Its primary use case is invariant assertions, but it reads naturally in `requires` guards and derived boolean values as well.

### Conditional expressions

```
-- Inline (single values)
email_status: if settings.email_on_mention = never: skipped else: pending
thread_depth: if is_reply: reply_to.thread_depth + 1 else: 0

-- Block (multiple outcomes)
ensures:
    if candidacy.retry_count < 2:
        candidacy.status = pending_scheduling
    else:
        candidacy.status = scheduling_stalled
        Notification.created(...)
```

Both forms use the same `if condition: ... else: ...` syntax. The inline form is for single-value assignments only. If either branch needs multiple statements or entity creation, use block form. Omit `else` when only the true branch has an effect.

Multi-branch conditionals use `else if`:

```
let preference =
    if notification.kind = MentionNotification: settings.email_on_mention
    else if notification.kind = ReplyNotification: settings.email_on_comment
    else if notification.kind = ShareNotification: settings.email_on_share
    else: immediately
```

Each `else if` adds a branch. The final `else` provides a fallback.

`exists` can also be used as a condition in `if` expressions, not just in `requires`. When `exists x` is used as an `if` condition, `x` is guaranteed non-null within the `if` body and can be accessed safely:

```
ensures:
    if exists existing:
        not exists existing
    else:
        CommentReaction.created(comment: comment, user: user, emoji: emoji)
```

### Existence

The `exists` keyword checks whether an entity instance exists. Use `not exists` for negation.

```
-- Entity looked up via let binding
let user = User{email}
requires: exists user

-- Join entity lookup
requires: exists WorkspaceMembership{user, workspace}

-- Negation
requires: not exists User{email: email}
requires: not exists ResourceInvitation{resource, email}
```

In `ensures` clauses, `not exists` asserts that an entity has been removed from the system:

```
-- Entity removal
ensures: not exists target_membership
ensures: not exists CommentMention{comment, user}

-- Bulk removal
ensures:
    for d in workspace.deleted_documents:
        not exists d
```

If the entity is already absent, the postcondition is trivially satisfied (no error, no operation). This follows from declarative semantics: `not exists x` asserts a property of the resulting state, not an imperative command.

This is distinct from soft delete, which changes a field rather than removing the entity:

```
-- Soft delete (entity still exists, status changes)
ensures: document.status = deleted

-- Hard delete (entity no longer exists)
ensures: not exists document
```

### Literals

```
-- Set literals
permissions: { "documents.read", "documents.write" }
features: { basic_editing, api_access }

-- Object literals (anonymous records, used in creation parameters and trigger emissions)
data: { candidate: candidate, time: time }
data: { slots: remaining_slots }
data: { unlocks_at: user.locked_until }
```

Object literals are anonymous record types. They carry named fields but have no declared type. Use them for ad-hoc data in entity creation parameters and trigger emission payloads where defining a named type would add ceremony without clarity. Object literals always require explicit `key: value` pairs; `{ x }` is a set literal containing `x`, not an object with shorthand.

### Black box functions

Black box functions represent domain logic too complex or algorithmic for the spec level. They appear in expressions and their behaviour is described by comments or deferred specifications. Black box functions always use free-standing call syntax; they never use dot-method syntax.

```
-- Scalar black box functions
hash(password)                              -- black box
verify(password, user.password_hash)        -- black box
parse_mentions(body)                        -- black box: extracts @username
next_digest_time(user)                      -- black box: uses digest_day_of_week

-- Collection-operating black box functions (collection as first argument)
filter(events, e => e.recent)               -- black box
grouped_by(copies, r => r.output_payloads)  -- black box
min_by(pending, e => e.offset)              -- black box
flatMap(groups, g => g.deferred_events)     -- black box
```

Black box functions are pure (no side effects) and deterministic for the same inputs within a rule execution.

The distinction between built-in operations and black box functions is syntactic: dot-method calls on collections (`.count`, `.any()`, `.all()`, `.first`, `.last`, `.unique`, `.add()`, `.remove()`) are built-in with language-defined semantics. Free-standing function calls are black box with implementation-defined semantics. The checker enforces this boundary: an unrecognised dot-method on a collection is an error. Built-in operations may chain from the result of a black box function call, since the result is a collection: `filter(events, e => e.recent).count` is valid.

### The `with` and `where` keywords

`with` declares how entities are connected. `where` selects from those connections.

A relationship declaration says "these are the InterviewSlots that belong to this Candidacy". A projection says "of those slots, show me the confirmed ones":

```
-- Relationship: declares which InterviewSlots belong to this Candidacy
slots: InterviewSlot with candidacy = this

-- Projection: of those slots, keep the confirmed ones
confirmed_slots: slots where status = confirmed
```

Because `with` defines a relationship from the universe of all instances, it needs `this` as an anchor — the predicate must reference the enclosing entity to establish the link. Because `where` filters an already-scoped collection, `this` would be meaningless and must not appear.

- **`with`** appears in relationship declarations. The predicate defines the structural link and must reference `this`.
- **`where`** appears in projections, iteration, surface context, actor identification and surface `let` bindings. The predicate filters an existing collection and must not reference `this`.

```
-- Surface context (where)
context assignment: SlotConfirmation where interviewer = viewer

-- Actor identification (where)
User where role = admin

-- Iteration (where)
for user in Users where notification_setting.digest_enabled:

-- Surface let binding (where)
let comments = Comments where parent = parent and status = active
```

Both `with` and `where` predicates support the same expression language as `requires` clauses: field navigation (including chained), comparisons, arithmetic, boolean combinators (`and`, `or`, `not`), bare boolean expressions and `in` for set membership. `where notification_setting.digest_enabled` and `where notification_setting.digest_enabled = true` are equivalent.

### Entity collections

The pluralised type name refers to all instances of that entity:

```
for user in Users where notification_setting.digest_enabled:
    ...
```

`Users` means all instances of `User`. Use natural English plurals: `Users`, `Documents`, `Workspaces`, `Candidacies`.

Entity collections are typically used in rule-level `for` clauses and surface `let` bindings to iterate or filter across all instances of a type.

---

## Invariants

Invariants are named assertions about properties that must hold over entity state. They appear at two scopes: top-level (system-wide properties) and entity-level (properties of individual instances).

### Top-level invariants

Top-level invariants assert properties over entity collections. They appear in the Invariants section after Rules:

```
invariant NonNegativeBalance {
    for account in Accounts:
        account.balance >= 0
}

invariant NoOverlappingInterviews {
    for a in Interviews:
        for b in Interviews:
            a != b and a.candidate = b.candidate
                implies not (a.start < b.end and b.start < a.end)
}

invariant UniqueEmail {
    for a in Users:
        for b in Users:
            a != b implies a.email != b.email
}
```

Each invariant has a PascalCase name and a brace-delimited body containing an expression that evaluates to a boolean.

### Entity-level invariants

Entity-level invariants assert properties scoped to a single entity type. They appear inside entity declarations alongside fields, relationships and derived values:

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

Within an entity-level invariant, field names resolve to the enclosing entity's fields without qualification. `this` refers to the entity instance.

### Expression language

Invariant expressions use the existing expression language without extension:

| Construct | Example |
|-----------|---------|
| Navigation | `account.balance`, `slot.interview.candidate` |
| Optional navigation | `parent?.status` |
| Comparisons | `balance >= 0`, `status = active`, `status in {active, pending}` |
| Boolean logic | `a and b`, `a or b`, `not a`, `a implies b` |
| Arithmetic | `balance + pending`, `count * rate` |
| Collection operations | `slots.count`, `slots.any(s => s.status = confirmed)` |
| Quantification | `for x in Collection: expression` (universal, all elements must satisfy) |
| Existence | `exists entity`, `not exists entity` |
| Null coalescing | `field ?? default` |
| Let bindings | `let total = debit + credit` (must be pure) |

`for x in Collection:` in an invariant body is a universal quantifier: the invariant holds when the expression is true for every element. This reuses the existing `for` iteration syntax with assertion semantics rather than ensures semantics. Nested quantification is permitted.

### Purity constraints

Invariant expressions must be pure:

- **No side effects.** Invariants cannot use `.add()`, `.remove()`, `.created()` or trigger emissions.
- **No `now`.** `now` is prohibited because it is volatile (re-evaluates on each read). Stored timestamp fields like `created_at` are permitted because they are stored state. The distinction is volatility, not temporality.
- **`let` bindings** are permitted but must themselves be pure expressions, subject to the same restrictions.

### Checking semantics

Invariants are logical assertions over entity state, not runtime checks. Checking frequency and strategy are tooling concerns: PBT checks invariants after rule sequences, the model checker checks exhaustively, the trace validator checks against reconstructed state.

Invariant expressions that reference `when`-qualified fields must scope their quantification to qualifying states. `for o in Orders: o.tracking_number != null` is an error if `tracking_number` carries a `when` clause, because the field does not exist in all states. Use a guard: `for o in Orders: o.status in {shipped, delivered} implies o.tracking_number != null`.

### Prose-only vs expression-bearing invariants

Two syntactically distinct forms exist:

- `@invariant Name` (sigil prefix, followed by indented comments) — prose annotation, used in contracts
- `invariant Name { expression }` (no sigil, braces) — expression-bearing, at top-level and entity-level scopes

The prose annotation describes a property informally. The expression-bearing form is a machine-readable assertion that tooling can exercise. When a prose annotation is promoted to the expression-bearing form, the `@` is dropped and a `{ expr }` body is added in its place.

### Recognising expressible invariants

Expression-bearing invariants assert properties over entity state at a single point in time. They answer the question "given the current state of all entities, does this property hold?" Not all important properties have this shape.

**Expressible** (use `invariant Name { expr }`):

- Uniqueness across entity instances: "no two instances share a priority"
- Relationships between fields on the same entity: "save_block = must_save implies expected_save_version != null"
- Bounds on field values: "gap >= 1", "version >= 1"
- Structural relationships between collections: "L1 and L2 never hold the same key", "distinct causal groups have disjoint entity key sets"
- Subset and partition relationships: "processed events are a subset of group events", "processed and deferred together cover all events"

The common thread: the property can be checked by reading current field values and navigating current relationships. No knowledge of history, ordering or external state is required.

**Not expressible** (use prose comments or `@invariant` in contracts):

- Cross-instance agreement: "all instances produce byte-identical outputs for the same input." This compares the behaviour of independent processes, not entity state.
- Temporal ordering: "event 2 sees the entity state left by event 1." This is about the order in which rules executed, not a static property.
- Evaluation function contracts: "the function is pure and deterministic." This constrains code behaviour, not entity state. Use `@invariant` inside a `contract` declaration.
- Counterfactual properties: "if a crash occurred, recovery could reconstruct this state." This reasons about a hypothetical scenario, not the current state.
- Monotonicity: "the watermark never decreases." This compares current state to prior state. A single-point-in-time invariant can assert a lower bound (`watermark >= -1`) but not that the value has not decreased since last observed.

When in doubt, try writing the expression. If it requires comparing two moments in time, reasoning about what another process would do, or referencing the order in which rules fired, it belongs in prose.

---

## Deferred specifications

Reference detailed specifications defined elsewhere:

```
deferred InterviewerMatching.suggest    -- see: detailed/interviewer-matching.allium
deferred SlotRecovery.initiate          -- see: slot-recovery.allium
```

This allows the main specification to remain succinct while acknowledging that detail exists elsewhere.

Deferred specifications are invoked at call sites using dot notation. They can appear as standalone ensures clauses or as expressions that return a value:

```
-- Standalone invocation (the deferred spec handles the outcome)
ensures: InterviewerMatching.suggest(candidacy)

-- Expression usage (the deferred spec returns a value)
ensures: OnCallPaged(team: EscalationPolicy.at_level(level), priority: immediate)
```

Unlike black box functions, which model opaque external computations, deferred specifications represent Allium logic that is fully specified elsewhere. The deferred declaration signals that the detail exists and is maintained separately.

---

## Open questions

Capture unresolved design decisions:

```
open question "Admin ownership - should admins be assigned to specific roles?"
open question "Multiple interview types - how is type assigned to candidacy?"
```

Open questions are surfaced by the specification checker as warnings, indicating the spec is incomplete.

---

## Config

A `config` block declares configurable parameters for the specification. Each parameter has a name, type and default value.

```
config {
    min_password_length: Integer = 12
    max_login_attempts: Integer = 5
    lockout_duration: Duration = 15.minutes
    reset_token_expiry: Duration = 1.hour
}
```

Rules reference config values with dot notation:

```
requires: length(password) >= config.min_password_length
ensures: token.expires_at = now + config.reset_token_expiry
```

External specs declare their own config blocks. Consuming specs configure them via the qualified name:

```
oauth/config {
    session_duration: 8.hours
    link_expiry: 15.minutes
}
```

External config values are referenced as `oauth/config.session_duration`.

### Config parameter references

A config parameter's default value can reference a parameter from an imported module's config block using a qualified name:

```
use "./core.allium" as core

config {
    instance_id: String                                     -- mandatory, no default
    required_copies: Integer = core/config.required_copies  -- defaults to core's value
    publish_delay: Duration = core/config.publish_delay     -- defaults to core's value
}
```

The local parameter can still be overridden by any consuming module. Resolution order:

1. If the consuming module sets the parameter explicitly, that value wins.
2. Otherwise, the qualified reference is followed. If the referenced parameter was itself overridden, the overridden value is used.
3. Otherwise, the referenced parameter's own default value is used.

Chains of references resolve transitively: if A defaults to B and B defaults to C, A resolves to C's value. The checker warns on chains longer than two levels of indirection.

The config reference graph must be acyclic. The checker reports an error if resolving a config default would revisit a parameter already in the resolution chain. When two modules both override the same parameter in a shared dependency (diamond dependency), the checker reports a conflict rather than silently picking one.

Renaming is permitted. The local parameter name need not match the referenced parameter's name, allowing domain-appropriate vocabulary.

### Expression-form config defaults

Config parameter defaults can be expressions combining qualified references, local config references and literal values with arithmetic operators:

```
use "./core.allium" as core

config {
    base_timeout: Duration = core/config.base_timeout
    extended_timeout: Duration = core/config.base_timeout * 2
    buffer_size: Integer = core/config.batch_size + 10
    retry_limit: Integer = max_attempts - 1                 -- local reference
}
```

Operators: `+`, `-`, `*`, `/` with standard precedence. Parenthesised sub-expressions are permitted for explicit precedence (`(base + 1) * 2`). Both local and qualified references are valid in expressions. The acyclicity rule applies uniformly to both cross-module and local reference edges. Expression-form defaults are evaluated once at config resolution time, after all overrides have been applied. They are not re-evaluated dynamically.

Type compatibility table for config default expressions:

| Left | Operator | Right | Result |
|------|----------|-------|--------|
| Integer | `+` `-` `*` `/` | Integer | Integer |
| Duration | `+` `-` | Duration | Duration |
| Duration | `*` `/` | Integer | Duration |
| Integer | `*` | Duration | Duration |
| Decimal | `+` `-` `*` `/` | Decimal | Decimal |
| Decimal | `*` `/` | Integer | Decimal |
| Integer | `*` | Decimal | Decimal |

Integer division uses truncation toward zero. All other type combinations are type errors. `Duration * Decimal` and `Decimal * Duration` are type errors; duration scaling uses Integer multipliers only. Commutative rows are listed for scalar multiplication only; addition and subtraction require matching types (Integer with Integer, Duration with Duration, Decimal with Decimal). Config default expressions are restricted to arithmetic operators and config references; boolean expressions are not permitted.

For default entity instances (seed data, base configurations), use `default` declarations.

---

## Defaults

Default declarations create named entity instances that exist unconditionally. They are available to all rules and surfaces without requiring creation by any rule.

```
default InterviewType all_in_one = { name: "All in one", duration: 75.minutes }

default Role viewer = {
    name: "viewer",
    permissions: { "documents.read" }
}

default Role editor = {
    name: "editor",
    permissions: { "documents.write" },
    inherits_from: viewer
}
```

---

## Modular specifications

### Namespaces

Namespaces are prefixes that organise names. Use qualified names to reference entities and triggers from other specs:

```
entity Candidacy {
    candidate: Candidate
    authenticated_via: google-oauth/Session
}
```

### Using other specs

The `use` keyword brings in another spec with an alias:

```
use "github.com/allium-specs/google-oauth/abc123def" as oauth
use "github.com/allium-specs/feedback-collection/def456" as feedback

entity Candidacy {
    authenticated_via: oauth/Session
    ...
}
```

Coordinates are immutable references (git SHAs or content hashes), not version numbers. No version resolution algorithms, no lock files. A spec is immutable once published.

### Referencing external entities and triggers

External specs' entities are used directly with qualified names:

```
rule RequestFeedback {
    when: interview: Interview.slot.time.start + 5.minutes <= now
    ensures: feedback/Request.created(
        subject: interview,
        respondents: interview.interviewers,
        deadline: 24.hours
    )
}
```

### Responding to external triggers

Any trigger or state transition from another spec can be responded to. No extension points need to be declared:

```
rule AuditLogin {
    when: oauth/SessionCreated(session)
    ensures: AuditLog.created(event: login, user: session.user)
}

rule NotifyOnFeedbackSubmitted {
    when: feedback/Request.status transitions_to submitted
    ensures:
        for admin in Users where role = admin:
            Notification.created(to: admin, template: feedback_received)
}
```

### Configuration

Imported specs expose their own config parameters. Consuming specs set values via the qualified name:

```
use "github.com/allium-specs/google-oauth/abc123def" as oauth

oauth/config {
    session_duration: 8.hours
    link_expiry: 15.minutes
}
```

Reference external config values as `oauth/config.session_duration`. This uses the same `config` mechanism as local config blocks (see [Config](#config)).

### Breaking changes

Avoid breaking changes: accrete (add new fields, triggers, states; never remove or rename). If a breaking change is necessary, publish under a new name rather than a new version. Consumers update at their own pace; old coordinates remain valid forever.

### Local specs

For specs within the same project, use relative paths:

```
use "./candidacy.allium" as candidacy
use "./scheduling.allium" as scheduling
```

External entities in one spec may be internal entities in another. The boundary is determined by the `external` keyword, not by file location.

---

## Surfaces

A surface defines a contract at a boundary. A boundary exists wherever two parties interact: a user and an application, a framework and its domain modules, a service and its consumers. Each surface names the boundary and specifies what each party exposes and provides, with a `contracts:` clause for programmatic integration obligations.

Surfaces serve two purposes:
- **Documentation**: Capture expectations about what each party sees, must contribute and can use
- **Test generation**: Generate tests that verify the implementation honours the contract

Surfaces do not specify implementation details (database schemas, wire protocols, thread models, UI layout). They specify the behavioural contract both sides must honour.

### Actor declarations

When a surface has a specific external party, declare actor types:

```
actor Interviewer {
    identified_by: User where role = interviewer
}

actor Admin {
    identified_by: User where role = admin
}

actor AuthenticatedUser {
    identified_by: User where active_sessions.count > 0
}
```

The `identified_by` expression specifies the entity type and condition that identifies the actor. It takes the form `EntityType where condition`, where the condition uses the entity's own fields, derived values and relationships. When an actor type is used in a `facing` clause, the binding variable has the entity type from the actor's `identified_by` expression. For example, `facing viewer: Interviewer` where `Interviewer` has `identified_by: User where role = interviewer` binds `viewer` as type `User`.

When an actor's identity depends on a context that varies per surface, declare the expected context type with a `within` clause and reference it in `identified_by`:

```
actor WorkspaceAdmin {
    within: Workspace
    identified_by: User where WorkspaceMembership{user: this, workspace: within}.can_admin = true
}
```

The `within` clause declares the entity type this actor requires from the surface's `context` binding. This makes the dependency explicit: the checker can verify that any surface using this actor provides a compatible context.

Two keywords are available inside `identified_by`:

- `this` — the entity instance being tested (here, the User). Same semantics as `this` in entity declarations.
- `within` — the entity bound by the `context` clause of the surface that uses this actor, constrained to the type declared in the actor's `within` clause.

```
surface WorkspaceManagement {
    facing admin: WorkspaceAdmin
    context workspace: Workspace    -- matches WorkspaceAdmin's within: Workspace
    ...
}
```

An actor declaration with a `within` clause can only be used in surfaces that declare a `context` clause. The surface's context type must match the actor's declared `within` type.

The `facing` clause accepts either an actor type or an entity type directly. Use actor declarations when the boundary has specific identity requirements (e.g., `WorkspaceAdmin` requires admin membership). Use entity types directly when any instance of that entity can interact (e.g., `facing visitor: User` for a public-facing surface). For integration surfaces where the external party is code rather than a person, declare an actor type with a minimal `identified_by` expression rather than leaving the type undeclared.

### Surface structure

```
surface SurfaceName {
    facing party: ActorType
    context item: EntityType [where predicate]
    let binding = expression

    exposes:
        item.field [when condition]
        ...

    provides:
        Action(party, item, ...) [when condition]
        ...

    contracts:
        demands ContractName             -- counterpart must implement
        fulfils ContractName             -- this surface supplies

    @guarantee ConstraintName
        -- Constraint description.

    @guidance
        -- Non-normative advice.

    related:
        OtherSurface(item.relationship) [when condition]
        ...

    timeout:
        RuleName [when temporal_condition]
}
```

Variable names (`party`, `item`) are user-chosen, not reserved keywords. All clauses are optional.

| Clause | Purpose |
|--------|---------|
| `facing` | Who is on the other side of the boundary |
| `context` | What entity or scope this surface applies to (one surface instance per matching entity; absent when no entity matches) |
| `let` | Local bindings, same as in rules |
| `exposes` | Visible data (supports `for` iteration over collections) |
| `provides` | Available operations with optional when-guards (parameters are per-action inputs from the party) |
| `contracts` | References to module-level `contract` declarations with direction markers. `demands ContractName` indicates the counterpart must implement; `fulfils ContractName` indicates this surface supplies |
| `@guarantee` | Named constraint that must hold across the boundary (prose annotation; PascalCase name required) |
| `@guidance` | Non-normative implementation advice (prose annotation; no name; must appear last) |
| `related` | Associated surfaces reachable from this one; the parenthesised expression evaluates to the entity instance that the target surface's `context` clause binds to, and its type must match the target surface's context type |
| `timeout` | References to temporal rules that apply within this surface's context (the rule name must correspond to a defined rule with a temporal trigger) |

### Examples

```
surface InterviewerPendingAssignments {
    facing viewer: Interviewer

    context assignment: InterviewAssignment
        where interviewer = viewer and status = pending

    exposes:
        assignment.interview.scheduled_time
        assignment.interview.candidate.name
        assignment.interview.duration

    provides:
        InterviewerConfirmsAssignment(viewer, assignment)
        InterviewerDeclinesAssignment(viewer, assignment, reason?)
}
```

```
surface InterviewerDashboard {
    facing viewer: Interviewer

    context assignment: SlotConfirmation where interviewer = viewer

    exposes:
        assignment.slot.time
        assignment.slot.candidacy.candidate.name
        assignment.status
        assignment.slot.other_confirmations.interviewer.name

    provides:
        InterviewerConfirmsSlot(viewer, assignment.slot)
            when assignment.status = pending
        InterviewerDeclinesSlot(viewer, assignment.slot)
            when assignment.status = pending

    related:
        InterviewDetail(assignment.slot.interview)
            when assignment.slot.interview != null
}
```

**Contract reference example** — contracts are declared at module level and referenced in surfaces via a `contracts:` clause with `demands`/`fulfils` direction markers.

```
contract DeterministicEvaluation {
    evaluate: (event_name: String, payload: ByteArray, current_state: ByteArray) -> EventOutcome

    @invariant Determinism
        -- For identical inputs, evaluate must produce
        -- byte-identical outputs across all instances.

    @invariant Purity
        -- No I/O, no clock, no mutable state outside arguments.
}

contract EventSubmitter {
    submit: (idempotency_key: String, event_name: String, payload: ByteArray) -> EventSubmission

    @invariant AtMostOnceProcessing
        -- Within the TTL window, duplicate submissions
        -- receive the cached response.
}

surface DomainIntegration {
    exposes:
        EntityKey
        EventOutcome

    contracts:
        demands DeterministicEvaluation
        fulfils EventSubmitter
}
```

**Invariant annotations** — `@invariant` inside a contract is a named, scoped prose annotation about a property of the operations in that contract. It carries a PascalCase name and a prose description in indented comment lines. Invariant names must be unique within their contract.

`@invariant` is distinct from `@guarantee`. `@guarantee` is a surface-level annotation about the boundary contract as a whole. `@invariant` describes a property scoped to a specific contract. The expression-bearing `invariant Name { expression }` construct (no sigil, no colon, brace-delimited body) is a separate form that appears at top-level and entity-level scopes (see [Invariants](#invariants)).

**Timeout example** — a `timeout` clause references an existing temporal rule by name and binds it to the surface's context. The rule name must correspond to a rule with a temporal trigger defined elsewhere in the spec. The `when` condition is optional: include it to restate the temporal expression for readability, or omit it when the rule name is self-explanatory. When present, the checker verifies the `when` condition matches the referenced rule's trigger.

```
surface InvitationView {
    facing recipient: Candidate

    context invitation: ResourceInvitation where email = recipient.email

    exposes:
        invitation.resource.name
        invitation.is_valid

    provides:
        AcceptInvitation(invitation, recipient) when invitation.is_valid

    timeout:
        InvitationExpires when invitation.expires_at <= now
}
```

The rule name alone is sufficient when the temporal condition is clear from the rule's name:

```
    timeout:
        InvitationExpires
```

When the `when` condition is included, it serves as inline documentation. The checker verifies it matches the referenced rule's trigger, preventing drift between the surface and the rule.

---

## Validation rules

A valid Allium specification must satisfy:

**Structural validity:**
1. All referenced entities and values exist (internal, external or imported)
2. All entity fields have defined types
3. All relationships reference valid entities (singular names) and include a backreference to `this` in their `with` predicate. `with` is used for relationship declarations and must reference `this`; `where` is used for filtering (projections, iteration, surface context, actor identification, surface `let`) and must not reference `this`
4. All rules have at least one trigger and at least one ensures clause
5. All triggers are valid (external stimulus, state transition, state becomes, entity creation, temporal, derived or chained)
6. All rules sharing a trigger name must use the same parameter count and positional types. Parameter binding names may differ between rules. Optional parameters (typed `T?`) may be omitted at call sites; omitted optional parameters bind to `null`

**State machine validity (without transition graph):**
7. All status values are reachable via some rule
8. All non-terminal status values have exits
9. No undefined states: rules cannot set status to values not in the enum

**Transition graph validity (when a `transitions` block is declared):**
7a. Rules whose `ensures` clauses produce transitions not in the declared graph are errors (authoritative relationship)
7b. Every non-terminal state in the graph has at least one outbound edge
7c. Every declared edge in the graph is witnessed by at least one rule whose `requires`/`ensures` pair can produce that transition
7d. Every enum value on the field appears in at least one edge or in the `terminal:` clause; every value in the graph exists on the field (exact correspondence)
7e. Terminal states must be explicitly declared with a `terminal:` clause

**State-dependent field validity (when `when` clauses are present):**
7f. Every state in a `when` clause must be a valid value of the referenced status field
7g. The field referenced in a `when` clause must have a `transitions` block
7h. Rules transitioning into the `when` set (source state outside, target state inside) must set the field
7i. Rules transitioning out of the `when` set (source state inside, target state outside) must clear the field
7j. Transitions within the `when` set (both states inside) or outside it (both states outside) carry no obligation
7k. Accessing a `when`-qualified field without a `requires` guard narrowing to a qualifying state is an error
7l. Optional explicit `when` on derived values must match the checker's inferred intersection of input `when` sets
7m. Tautological invariant (off by default, opt-in): an expression-bearing invariant whose assertion is provably true given lifecycle analysis from `when` clauses and transition reachability

**Expression validity:**
10. No circular dependencies in derived values
11. All variables are bound before use
12. Type consistency in comparisons and arithmetic
13. All lambdas are explicit (use `i => i.field` not `field`)
14. Inline enum fields cannot be compared with each other (whether on the same entity or across entities); use a named enum to share values across fields
14a. Dot-method calls on collections must use a recognised built-in name (`.count`, `.any()`, `.all()`, `.first`, `.last`, `.unique`, `.add()`, `.remove()`). Unrecognised dot-methods are errors. Domain-specific collection operations use free-standing black box function syntax

**Sum type validity:**
15. Sum type discriminators use the pipe syntax with capitalised variant names (`A | B | C`)
16. All names in a discriminator field must be declared as `variant X : BaseEntity`
17. All variants that extend a base entity must be listed in that entity's discriminator field
18. Variant-specific fields are only accessed within type guards (`requires:` or `if` branches)
19. Base entities with sum type discriminators cannot be instantiated directly
20. Discriminator field names are user-defined (e.g., `kind`, `node_type`), no reserved name
21. The `variant` keyword is required for variant declarations

**Given validity:**
22. `given` bindings must reference entity types declared in the module or imported via `use`
23. Each binding name must be unique within the `given` block
24. Unqualified instance references in rules must resolve to a `given` binding, a `let` binding, a trigger parameter or a default entity instance

**Config validity:**
25. Config parameters must have explicit types. Parameters with default values must declare them explicitly (literal, qualified reference or expression). Parameters without defaults are mandatory: consuming modules must supply a value
26. Config parameter names must be unique within the config block
27. References to `config.field` in rules must correspond to a declared parameter in the local config block or a qualified external config (`alias/config.field`)

**Surface validity:**
28. Types in `facing` clauses must be either a declared `actor` type or a valid entity type (internal, external or imported)
29. All fields referenced in `exposes` must be reachable from bindings declared in the surface (`facing`, `context`, `let`), via relationships, or be declared types from imported specifications
30. All triggers referenced in `provides` must be defined as external stimulus triggers in rules
31. All surfaces referenced in `related` must be defined, and the type of the parenthesised expression must match the target surface's `context` type
32. Bindings in `facing` and `context` clauses must be used consistently throughout the surface
33. `when` conditions must reference valid fields reachable from the party or context bindings
34. `for` iterations must iterate over collection-typed fields or bindings and are valid in block scopes that produce per-item content (`exposes`, `provides`, `related`)
35. Rule names referenced in `timeout` clauses must correspond to a defined rule with a temporal trigger. If a `when` condition is present, it must match the referenced rule's temporal trigger expression

**Contract clause validity:**
36. `contracts:` entries must use `demands` or `fulfils` followed by a PascalCase contract name
37. Each contract name appears at most once per surface
38. Referenced contract names must resolve to a `contract` declaration in scope (local or imported via `use`)
39. Same-named contracts from different modules on the same surface are a structural error

**Contract validity:**
40. `contract` declarations must have a PascalCase name followed by a brace-delimited block body
41. Contract bodies may contain only typed signatures and annotations (`@invariant`, `@guidance`)
42. Types in contract signatures must be declared at module level or imported via `use`
43. Contract names must be unique at module level
44. `@invariant` annotations within contracts must have a PascalCase name and be followed by at least one indented comment line
45. `@invariant` names must be unique within their contract

**Config reference validity:**
46. A qualified config reference in a default expression must resolve to a declared parameter in an imported module's config block
47. The declared type of a parameter with a qualified default must match the referenced parameter's type
48. The config reference graph must be acyclic

**Config expression validity:**
49. Expression-form config defaults must use only arithmetic operators (`+`, `-`, `*`, `/`), literal values, local config parameter references and qualified config references
50. Both sides of an arithmetic operator in a config default must resolve to type-compatible operands per the type compatibility table

**Invariant validity:**
51. Top-level `invariant` blocks must have a PascalCase name followed by a brace-delimited expression body
52. Entity-level `invariant` blocks must have a PascalCase name followed by a brace-delimited expression body
53. Invariant names must be unique within their scope (module-level for top-level invariants, entity declaration for entity-level invariants)
54. Invariant expressions must evaluate to a boolean type
55. Invariant expressions must not contain side-effecting operations (`.add()`, `.remove()`, `.created()`, trigger emissions)
56. Invariant expressions must not reference `now` (volatile; stored timestamp fields are permitted)
57. Entity collection references in top-level invariants must correspond to declared entity types

**Ordered collection validity:**
58. `.first` and `.last` on unordered collections (`Set`) produce a warning in the current version, becoming a hard error in the next version
59. Set arithmetic (`+`, `-`) on ordered collections produces unordered results. The checker reports an error if the result is used where an ordered collection is expected
60. `.unique` produces an unordered `Set` regardless of the source collection's ordering

**Enum literal validity:**
61. Backtick-quoted enum literals must contain only printable Unicode characters (categories L, M, N, P, S) excluding backtick and whitespace
62. Backtick-quoted literals are permitted only in enum declarations (named and inline), literal comparisons and `ensures` clauses; they are not permitted in identifier positions
63. Backtick-quoted literals cannot appear in arithmetic expressions

**Annotation validity:**
64. `@invariant` requires a PascalCase name; names must be unique within their containing construct (contract or surface)
65. `@guarantee` requires a PascalCase name; names must be unique within their surface
66. `@guidance` must not have a name; must appear after all structural clauses and after all other annotations in its containing construct
67. All annotations must be followed by at least one indented comment line; unindented comment lines after an annotation are not part of the annotation body
68. Within a construct, `@invariant` and `@guarantee` annotations may appear in any order relative to each other but must appear after all structural clauses; `@guidance` must appear last

The checker should warn (but not error) on:
- External entities without known governing specification
- Open questions
- Deferred specifications without location hints
- Unused entities or fields
- Rules that can never fire (preconditions always false)
- Temporal rules without guards against re-firing
- Surfaces that reference fields not used by any rule (may indicate dead code)
- Items in `provides` with `when` conditions that can never be true
- Actor declarations that are never used in any surface
- Rules whose ensures creates an entity for a parent, where sibling rules on the same parent don't guard against that entity's existence
- Surface `provides` when-guards weaker than the corresponding rule's requires
- Rules with the same trigger and overlapping preconditions (spec ambiguity)
- Parameterised derived values that reference fields outside the entity (scoping violation)
- Actor `identified_by` expressions that are trivially always-true or always-false
- Rules where all ensures clauses are conditional and at least one execution path produces no effects
- Temporal triggers on optional fields (trigger will not fire when the field is null)
- Surfaces that use a raw entity type in `facing` when actor declarations exist for that entity type (may indicate a missing access restriction)
- `transitions_to` triggers on values that entities can be created with (the rule will not fire on creation; consider `becomes` if the rule should also fire on creation)
- Multiple fields on the same entity with identical inline enum literals (suggests extraction to a named enum; will error if the fields are later compared)
- `@invariant` prose that resembles a formal expression (informational: promote to expression-bearing `invariant Name { expression }` when the assertion is machine-readable)
- Config reference chains deeper than two levels of indirection
- Diamond dependency conflicts in config overrides
- Tautological invariant (off by default, opt-in): an expression-bearing invariant whose assertion is provably true given lifecycle analysis from `when` clauses and transition reachability
- `.first` or `.last` on unordered collections (warning in current version, error in next)

---

## Anti-patterns

**Implementation leakage:**
```
-- Bad
let request = FeedbackRequest.find(interview_id, interviewer_id)

-- Good
let request = FeedbackRequest{interview, interviewer}
```

**UI/UX in spec:**
```
-- Bad
ensures: Button.displayed(label: "Confirm", onClick: ...)

-- Good
ensures: CandidateInformed(about: options_available, data: { slots: slots })
```

**Algorithm in rules:**
```
-- Bad
ensures: selected = filter(take(sortBy(interviewers, load), 3), available)

-- Good
ensures: Suggestion.created(
    interviewers: InterviewerMatching.suggest(considering: [...])
)
```

**Queries in rules:**
```
-- Bad
let pending = SlotConfirmation.where(slot: slot, status: pending)

-- Good
let pending = slot.pending_confirmations
```

**Implicit shorthand in lambdas:**
```
-- Bad
interviewers.any(can_solo)

-- Good
interviewers.any(i => i.can_solo)
```

**Missing temporal guards:**
```
-- Bad: can fire repeatedly
rule InvitationExpires {
    when: invitation: Invitation.expires_at <= now
    ensures: invitation.status = expired
}

-- Good: guard prevents re-firing
rule InvitationExpires {
    when: invitation: Invitation.expires_at <= now
    requires: invitation.status = pending
    ensures: invitation.status = expired
}
```

**Overly broad status enums:**
```
-- Bad
status: draft | pending | active | paused | resumed | completed |
        cancelled | expired | archived | deleted

-- Good
status: pending | active | completed | cancelled
is_archived: Boolean
```

**`transitions_to` doesn't fire on creation:**
```
-- Bad: won't fire when Interview is created with status = scheduled
rule NotifyOnScheduled {
    when: interview: Interview.status transitions_to scheduled
    ensures: Email.created(to: interview.candidate.email, template: interview_scheduled)
}

-- Good: use becomes when the rule should fire regardless of how the state was reached
rule NotifyOnScheduled {
    when: interview: Interview.status becomes scheduled
    ensures: Email.created(to: interview.candidate.email, template: interview_scheduled)
}

-- Also good: handle creation and transition separately when the response differs
rule NotifyOnRescheduled {
    when: interview: Interview.status transitions_to scheduled
    ensures: Email.created(to: interview.candidate.email, template: interview_rescheduled)
}

rule NotifyOnCreatedScheduled {
    when: interview: Interview.created
    requires: interview.status = scheduled
    ensures: Email.created(to: interview.candidate.email, template: interview_scheduled)
}
```

**Magic numbers in rules:**
```
-- Bad
requires: attempts < 3
ensures: deadline = now + 48.hours

-- Good
requires: attempts < config.max_attempts
ensures: deadline = now + config.confirmation_deadline
```

---

## Glossary

| Term | Definition |
|------|------------|
| **Given (module)** | Entity instances a module operates on, declared with `given { ... }`; inherited by all rules in the module. Binds singleton instances at module scope. Contrast with **Context**, which is parametric |
| **Context (surface)** | Parametric scope binding for a boundary contract, declared with `context` inside a surface. Creates one surface instance per matching entity. Contrast with **Given**, which binds singleton instances at module scope |
| **Entity** | A domain concept with identity and lifecycle |
| **Value** | Structured data without identity, compared by structure |
| **Sum Type** | Entity constrained to exactly one of several variants via a discriminator field |
| **Discriminator** | Field whose pipe-separated capitalised values name the variants |
| **Variant** | One alternative in a sum type, declared with `variant X : Base { ... }` |
| **Type Guard** | Condition (`requires:` or `if`) that narrows to a variant, unlocking its fields |
| **Field** | Data stored on an entity or value |
| **Relationship** | Navigation from one entity to related entities. Unordered relationships produce `Set`; ordered relationships produce `Sequence`. Declaration syntax for ordered relationships is pending |
| **Projection** | A filtered view of a relationship. Preserves the ordering of the source collection: a projection of a `Sequence` is a `Sequence` |
| **Sequence** | Ordered collection type that will be produced by ordered relationships and their projections when declaration syntax is introduced (pending follow-up ALP). A subtype of `Set`: assignable where an unordered collection is expected, but not the reverse. Distinct from `List<T>`, which is a compound field type declared explicitly. Ordering propagates through `where` and field extraction; set arithmetic (`+`, `-`) and `.unique` produce unordered results |
| **Ordered collection** | A collection whose elements have a meaningful sequence (intrinsic order). `Sequence` and `List<T>` are the two ordered collection types. `.first`, `.last` and deterministic `for` iteration are restricted to ordered collections |
| **Derived Value** | A computed value based on other fields |
| **Parameterised Derived Value** | A derived value that takes arguments, e.g. `can_use_feature(f): f in plan.features` |
| **Rule** | A specification of behaviour triggered by some condition |
| **Trigger** | The condition that causes a rule to fire |
| **Trigger Emission** | An ensures clause that emits a named event; other rules chain from it via their `when` clause |
| **Precondition** | A requirement that must be true for a rule to execute |
| **Postcondition** | An assertion about what becomes true after a rule executes |
| **`when` clause (field)** | Clause on a field declaration tying its presence to lifecycle state: `field: Type when status = value1 \| value2`. The field is present only in the listed states. The referenced status field must have a `transitions` block. Orthogonal to `?` (genuine optionality) |
| **Presence obligation** | When a rule transitions an entity into a field's `when` set (source state outside, target state inside), the rule must set the field |
| **Absence obligation** | When a rule transitions an entity out of a field's `when` set (source state inside, target state outside), the rule must clear the field |
| **Derived value `when` inference** | The checker infers `when` sets for derived values by intersecting the `when` sets of their inputs. Authors may optionally annotate with an explicit `when` clause, verified against the inference. The checker exports inferred `when` sets as structured data |
| **Black Box Function** | Domain logic referenced but not defined in the spec; pure and deterministic. Always use free-standing call syntax, never dot-method syntax. For collection-operating functions, the collection is the first argument: `filter(events, predicate)`. Common examples include `hash()`, `verify()`, `filter()`, `grouped_by()` |
| **External Entity** | An entity managed by another specification; referenced but not governed here |
| **Config** | Configurable parameters for a specification, referenced via `config.field` |
| **Default** | A named entity instance used as seed data or base configuration |
| **Deferred Specification** | Complex logic defined in a separate file |
| **Open Question** | An unresolved design decision |
| **Entity Collection** | Pluralised type name referring to all instances of that entity (e.g., `Users` for all `User` instances) |
| **Exists** | Keyword for checking entity existence (`exists x`) or asserting removal (`not exists x`) |
| **`within`** | Clause in actor declarations that names the required context type; also a keyword in `identified_by` expressions that resolves to the surface's context entity |
| **`this`** | The instance of the enclosing type; valid in entity declarations and actor `identified_by` expressions |
| **Enum** | A set of values. **Named enums** (`enum Recommendation { ... }`) have type identity and are reusable across fields and entities. **Inline enums** (`status: pending \| active`) are anonymous, scoped to a single field, and cannot be compared across fields. Enum literals referencing external standards may use backtick quoting (`` `de-CH-1996` ``) to preserve the standard's canonical form; quoted and unquoted literals are distinct values with no implicit normalisation |
| **Transition Graph** | An authoritative, opt-in declaration of valid lifecycle transitions for an enum status field. Declared inside the entity body with `transitions field_name { ... }` using `->` edge notation and a `terminal:` clause. When present, rules producing transitions not in the graph are validation errors. Entities without a graph derive transition validity from rules alone |
| **Discard Binding** | `_` used where a binding is syntactically required but the value is not needed |
| **Actor** | An entity type that can interact with surfaces, declared with explicit identity mapping |
| **`facing`** | Surface clause naming the external party on the other side of the boundary |
| **Surface** | A boundary contract between two parties specifying what each side exposes and provides, with optional `contracts:` clause for programmatic integration obligations |
| **Contract** | A named, direction-agnostic obligation declared at module level with `contract Name { ... }`. Surfaces reference contracts in a `contracts:` clause with `demands`/`fulfils` direction markers. Identity determined by module-qualified name |
| **`demands`** | Direction marker in a `contracts:` clause indicating the counterpart must implement this contract |
| **`fulfils`** | Direction marker in a `contracts:` clause indicating this surface supplies the contract's operations |
| **Invariant** | A named, scoped assertion about a property. Two syntactic forms: `@invariant Name` (prose annotation, in contracts) and `invariant Name { expression }` (expression-bearing, at top-level and entity-level). Expression-bearing invariants are logical assertions over entity state, not runtime checks. Distinct from `@guarantee`, which annotates properties of the boundary as a whole |
| **`@guarantee`** | Named prose annotation on a surface asserting a property of the boundary as a whole. PascalCase name required, unique within the surface. Structurally validated by the checker; prose content is not evaluated. Distinct from `@invariant` (scoped to a contract) and expression-bearing `invariant Name { }` (machine-readable) |
| **`@guidance`** | Unnamed prose annotation providing non-normative implementation advice. Permitted in contracts, rules and surfaces. Must appear after all structural clauses and after all other annotations in its containing construct. Structurally validated; prose content is not evaluated |
| **`implies`** | Boolean operator. `a implies b` is `not a or b`. Lowest boolean precedence, binding looser than `and` and `or`. Available in all expression contexts |
| **Config reference** | A qualified reference in a config default (`param: Type = other/config.param`) that aliases a parameter from an imported module. Supports expression-form defaults with arithmetic operators |
