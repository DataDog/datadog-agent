---
name: allium
description: Give your AI agents something more useful than a prompt. Velocity through clarity.
version: 3
auto_trigger:
  - file_patterns: ["**/*.allium"]
  - keywords: ["allium", "allium spec", "allium specification", ".allium file"]
---

# Allium

Allium is a formal language for capturing software behaviour at the domain level. It sits between informal feature descriptions and implementation, providing a precise way to specify what software does without prescribing how it's built.

The name comes from the botanical family containing onions and shallots, continuing a tradition in behaviour specification tooling established by Cucumber and Gherkin.

Key principles:

- Describes observable behaviour, not implementation
- Captures domain logic that matters at the behavioural level
- Generates integration and end-to-end tests (not unit tests)
- Forces ambiguities into the open before implementation
- Implementation-agnostic: the same spec could be implemented in any language

Allium does NOT specify programming language or framework choices, database schemas or storage mechanisms, API designs or UI layouts, or internal algorithms (unless they are domain-level concerns).

## Routing table

| Task | Tool | When |
|------|------|------|
| Writing or reading `.allium` files | this skill | You need language syntax and structure |
| Building a spec through conversation | `elicit` skill | User describes a feature or behaviour they want to build |
| Extracting a spec from existing code | `distill` skill | User has implementation code and wants a spec from it |
| Modifying an existing spec | `tend` agent | User wants targeted changes to `.allium` files |
| Checking spec-to-code alignment | `weed` agent | User wants to find or fix divergences between spec and implementation |
| Generating tests from a spec | `propagate` skill | User wants to generate tests, PBT properties or state machine tests from a specification |

## Quick syntax summary

### Entity

```
entity Candidacy {
    -- Fields
    candidate: Candidate
    role: Role
    status: pending | active | completed | cancelled   -- inline enum
    retry_count: Integer

    -- Relationships
    invitation: Invitation with candidacy = this         -- one-to-one
    slots: InterviewSlot with candidacy = this           -- one-to-many

    -- Projections
    confirmed_slots: slots where status = confirmed
    pending_slots: slots where status = pending

    -- Derived
    is_ready: confirmed_slots.count >= 3
    has_expired: invitation.expires_at <= now
}
```

### External entity

```
external entity Role { title: String, required_skills: Set<Skill>, location: Location }
```

### Value type

```
value TimeRange { start: Timestamp, end: Timestamp, duration: end - start }
```

### Sum type

A base entity declares a discriminator field whose capitalised values name the variants. Variants use the `variant` keyword.

```
entity Node {
    path: Path
    kind: Branch | Leaf              -- discriminator field
}

variant Branch : Node {
    children: List<Node?>
}

variant Leaf : Node {
    data: List<Integer>
    log: List<Integer>
}
```

Lowercase pipe values are enum literals (`status: pending | active`). Capitalised values are variant references (`kind: Branch | Leaf`). Type guards (`requires:` or `if` branches) narrow to a variant and unlock its fields.

### Module given

Declares the entity instances a module's rules operate on. All rules inherit these bindings. Not every module needs one: rules scoped by triggers on domain entities get their entities from the trigger. `given` is for specs where rules operate on shared instances that exist once per module scope.

```
given {
    pipeline: HiringPipeline
    calendar: InterviewCalendar
}
```

Imported module instances are accessed via qualified names (`scheduling/calendar`) and do not appear in the local `given` block. Distinct from surface `context`, which binds a parametric scope for a boundary contract.

### Rule

```
rule InvitationExpires {
    when: invitation: Invitation.expires_at <= now
    requires: invitation.status = pending
    let remaining = invitation.proposed_slots where status != cancelled
    ensures: invitation.status = expired
    ensures:
        for s in remaining:
            s.status = cancelled
    @guidance
        -- Non-normative implementation advice.
}
```

### Trigger types

- **External stimulus**: `when: CandidateSelectsSlot(invitation, slot)` — action from outside the system
- **State transition**: `when: interview: Interview.status transitions_to scheduled` — entity changed state (transition only, not creation)
- **State becomes**: `when: interview: Interview.status becomes scheduled` — entity has this value, whether by creation or transition
- **Temporal**: `when: invitation: Invitation.expires_at <= now` — time-based condition (always add a `requires` guard against re-firing)
- **Derived condition**: `when: interview: Interview.all_feedback_in` — derived value becomes true
- **Entity creation**: `when: batch: DigestBatch.created` — fires when a new entity is created
- **Chained**: `when: AllConfirmationsResolved(candidacy)` — subscribes to a trigger emission from another rule's ensures clause

All entity-scoped triggers use explicit `var: Type` binding. Use `_` as a discard binding where the name is not needed: `when: _: Invitation.expires_at <= now`, `when: SomeEvent(_, slot)`.

### Rule-level iteration

A `for` clause applies the rule body once per element in a collection:

```
rule ProcessDigests {
    when: schedule: DigestSchedule.next_run_at <= now
    for user in Users where notification_setting.digest_enabled:
        let settings = user.notification_setting
        ensures: DigestBatch.created(user: user, ...)
}
```

### Ensures patterns

Ensures clauses have four outcome forms:

- **State changes**: `entity.field = value`
- **Entity creation**: `Entity.created(...)` — the single canonical creation verb
- **Trigger emission**: `TriggerName(params)` — emits an event for other rules to chain from
- **Entity removal**: `not exists entity` — asserts the entity no longer exists

These forms compose with `for` iteration (`for x in collection: ...`), `if`/`else` conditionals and `let` bindings.

Entity creation uses `.created()` exclusively. Domain meaning lives in entity names and rule names, not in creation verbs.

In state change assignments, the right-hand expression references pre-rule field values. Conditions within ensures blocks (`if` guards, creation parameters, trigger emission parameters) reference the resulting state.

### Surface

```
surface InterviewerDashboard {
    facing viewer: Interviewer

    context assignment: SlotConfirmation where interviewer = viewer

    exposes:
        assignment.slot.time
        assignment.status

    provides:
        InterviewerConfirmsSlot(viewer, assignment.slot)
            when assignment.status = pending

    related:
        InterviewDetail(assignment.slot.interview)
            when assignment.slot.interview != null
}
```

Surfaces define contracts at boundaries. The `facing` clause names the external party, `context` scopes the entity. The remaining clauses use a single vocabulary regardless of whether the boundary is user-facing or code-to-code: `exposes` (visible data, supports `for` iteration over collections), `provides` (available operations with optional when-guards), `contracts:` (references module-level `contract` declarations with `demands`/`fulfils` direction markers), `@guarantee` (named prose assertions about the boundary), `@guidance` (non-normative advice), `related` (associated surfaces reachable from this one), `timeout` (references to temporal rules that apply within the surface's context).

The `facing` clause accepts either an actor type (with a corresponding `actor` declaration and `identified_by` mapping) or an entity type directly. Use actor declarations when the boundary has specific identity requirements; use entity types when any instance can interact (e.g., `facing visitor: User`). For integration surfaces where the external party is code, declare an actor type with a minimal `identified_by` expression. Actors that reference `within` in their `identified_by` expression must declare the expected context type: `within: Workspace`.

### Surface-to-implementation contract

The `exposes` block is the field-level contract: the implementation returns exactly these fields, the consumer uses exactly these fields. Do not add fields not listed. Do not omit fields that are listed.

### Contract

```allium
contract Codec {
    serialize: (value: Any) -> ByteArray
    deserialize: (bytes: ByteArray) -> Any

    @invariant Roundtrip
        -- deserialize(serialize(value)) produces a value
        -- equivalent to the original for all supported types.
}
```

Contracts are module-level declarations referenced by name in surface `contracts:` clauses (`demands Codec`, `fulfils EventSubmitter`). See [Contracts](./references/language-reference.md#contracts) for declaration syntax and referencing rules.

### Expressions

Navigation: `interview.candidacy.candidate.email`, `reply_to?.author` (optional), `timezone ?? "UTC"` (null coalescing). Collections: `slots.count`, `slot in invitation.slots`, `interviewers.any(i => i.can_solo)`, `for item in collection: item.status = cancelled`, `permissions + inherited` (set union), `old - new` (set difference). Comparisons: `status = pending`, `count >= 2`, `status in {confirmed, declined}`, `provider not in providers`. Boolean logic: `a and b`, `a or b`, `not a`, `a implies b`.

### Modular specs

```
use "github.com/allium-specs/google-oauth/abc123def" as oauth
```

Qualified names reference entities across specs: `oauth/Session`. Coordinates are immutable (git SHAs or content hashes). Local specs use relative paths: `use "./candidacy.allium" as candidacy`.

### Config

```
config {
    invitation_expiry: Duration = 7.days
    max_login_attempts: Integer = 5
    extended_expiry: Duration = invitation_expiry * 2              -- expression-form default
    sync_timeout: Duration = core/config.default_timeout           -- config parameter reference
}
```

Rules reference config values as `config.invitation_expiry`. For default entity instances, use `default`.

### Defaults

```
default Role viewer = { name: "viewer", permissions: { "documents.read" } }
```

### Invariant

```allium
invariant NonNegativeBalance {
    for account in Accounts:
        account.balance >= 0
}
```

Expression-bearing invariants (`invariant Name { expression }`) assert properties over entity state. They are logical assertions, not runtime checks. Distinct from prose annotations (`@invariant Name`) in contracts, which use the `@` sigil to mark content the checker does not evaluate. See [Invariants](./references/language-reference.md#invariants).

### Transition graph (v3)

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

### State-dependent field presence (v3)

```
entity Order {
    status: pending | confirmed | shipped | delivered | cancelled
    customer: Customer
    total: Money
    tracking_number: String when status = shipped | delivered
    shipped_at: Timestamp when status = shipped | delivered

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

### Deferred specs

```
deferred InterviewerMatching.suggest    -- see: detailed/interviewer-matching.allium
```

### Open questions

```
open question "Admin ownership - should admins be assigned to specific roles?"
```

## Verification

When the `allium` CLI is installed, a hook validates `.allium` files automatically after every write or edit. Fix any reported issues before presenting the result. If the CLI is not available, verify against the [language reference](./references/language-reference.md).

## References

- [Language reference](./references/language-reference.md) — full syntax for entities, rules, expressions, surfaces, contracts, invariants and validation
- [Test generation](./references/test-generation.md) — generating tests from specifications
- [Patterns](./references/patterns.md) — 9 worked patterns: auth, RBAC, invitations, soft delete, notifications, usage limits, comments, library spec integration, framework integration contract
