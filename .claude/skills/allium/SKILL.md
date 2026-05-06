---
name: allium
description: "Writes, reads, and validates .allium behavioural specification files. Use when the user asks to create an allium spec, edit a .allium file, write a behavioural specification, or understand allium syntax."
metadata:
  version: 3
  auto_trigger_file_patterns: "**/*.allium"
  auto_trigger_keywords: "allium, allium spec, allium specification, .allium file"
---

# Allium

Allium is a formal language for capturing software behaviour at the domain level. It describes observable behaviour, not implementation, and generates integration and end-to-end tests.

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

Capitalised pipe values are variant references (`kind: Branch | Leaf`), lowercase are enum literals (`status: pending | active`). See [language reference](./references/language-reference.md) for full variant syntax.

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

- **External stimulus**: `when: CandidateSelectsSlot(invitation, slot)`
- **State transition**: `when: interview: Interview.status transitions_to scheduled` (transition only, not creation)
- **State becomes**: `when: interview: Interview.status becomes scheduled` (creation or transition)
- **Temporal**: `when: invitation: Invitation.expires_at <= now` (always add `requires` guard)
- **Derived condition**: `when: interview: Interview.all_feedback_in`
- **Entity creation**: `when: batch: DigestBatch.created`
- **Chained**: `when: AllConfirmationsResolved(candidacy)` (subscribes to trigger emission)

### Ensures patterns

- **State changes**: `entity.field = value`
- **Entity creation**: `Entity.created(...)` (the only creation verb)
- **Trigger emission**: `TriggerName(params)`
- **Entity removal**: `not exists entity`

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

Surfaces define boundary contracts: `facing` names the external party, `context` scopes the entity, `exposes` lists visible data, `provides` lists available operations, `contracts:` references module-level contract declarations.

For full syntax of contracts, expressions, config, defaults, invariants, transition graphs, state-dependent fields, deferred specs, and open questions, see the [language reference](./references/language-reference.md).

## Authoring workflow

1. Define entities with fields, relationships, and derived values
2. Write rules with triggers, guards, and ensures clauses
3. Add surfaces for boundary contracts (UI, API, integration points)
4. Add config, invariants, and open questions as needed
5. Validate with the `allium` CLI (if installed) or against the language reference
6. Fix any reported issues and re-validate

## Verification

When the `allium` CLI is installed, a hook validates `.allium` files automatically after every write or edit. Fix any reported issues before presenting the result. If the CLI is not available, verify against the [language reference](./references/language-reference.md).

## References

- [Language reference](./references/language-reference.md) — full syntax for entities, rules, expressions, surfaces, contracts, invariants and validation
- [Test generation](./references/test-generation.md) — generating tests from specifications
- [Patterns](./references/patterns.md) — 9 worked patterns: auth, RBAC, invitations, soft delete, notifications, usage limits, comments, library spec integration, framework integration contract
