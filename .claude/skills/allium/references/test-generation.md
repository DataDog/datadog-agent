# Test generation

This document describes categories of tests that a person, agent or skill should derive from an Allium specification. It is not a tool to invoke. The taxonomy maps spec constructs to test obligations; how those tests are expressed depends on the target language and test framework.

From an Allium specification, generate:

**Entity and value type tests** (per entity and value type):
- Verify all declared fields are present with correct types
- Verify optional fields accept null and non-null values
- Verify optional navigation (`?.`) short-circuits to null when the left side is absent
- Verify null coalescing (`??`) produces the default when the left side is null and the original value otherwise
- Verify relationships navigate to the correct related entities
- Verify join lookups (`Entity{field1, field2}`) resolve to the correct instance, or null when no match exists
- For value types, verify equality is structural (by field values, not reference)

**Enumeration tests** (per named enum):
- Verify fields typed with the same named enum are comparable
- Verify `in` and `not in` membership tests work against set literals of enum values

**Sum type tests** (per sum type):
- Verify each variant has its variant-specific fields accessible within a type guard
- Verify variant-specific fields are inaccessible outside type guards
- Verify all variants listed in the discriminator are handled in conditional logic
- Verify an entity cannot be multiple variants simultaneously
- Verify creation uses the variant name, not the base entity name
- Verify a `.created` trigger on the base entity fires for any variant and can be narrowed with a type guard

**Derived value and projection tests** (per derived value or projection):
- Verify derived values compute correctly for representative entity states
- Verify projections filter correctly against their `where` predicate
- Verify projections with `-> field` mapping extract the correct field and exclude nulls
- Verify parameterised derived values return correct results for representative arguments
- Verify derived values involving `now` are volatile (re-evaluate on each read)
- Verify built-in collection operations (`.any()`, `.all()`, `.count`, set `+`/`-`, `.first`, `.last`) produce correct results
- Verify black box collection functions (free-standing calls like `filter(collection, predicate)`) are treated as opaque with implementation-defined semantics

**Default instance tests** (per `default` declaration):
- Verify the named instance exists unconditionally
- Verify all specified field values match the declaration
- Verify cross-references between defaults resolve correctly (e.g. `inherits_from: viewer`)

**Config tests** (per config block):
- Verify each parameter has its declared default value when not overridden
- Verify overriding a parameter replaces the default
- Verify mandatory parameters (no default) cause an error when omitted
- Verify expression-form defaults evaluate correctly, including type compatibility of arithmetic operands
- Verify qualified references to imported config resolve through the override chain
- Verify config parameters that depend on other parameters resolve in the correct order

**Invariant tests** (per expression-bearing `invariant Name { expr }`):
- Verify the invariant holds after every state-changing rule that touches the constrained entities
- Verify the invariant holds for edge-case entity states (boundary values, empty collections)
- For entity-level invariants, verify they hold after any field mutation on the entity
- For invariants using `implies`, verify the consequent holds when the antecedent is true and that the invariant is trivially satisfied when the antecedent is false

**Rule tests** (per rule):
- Success case: all preconditions met, verify all postconditions hold
- Failure cases: verify the rule is rejected when each `requires` clause independently fails
- Edge cases: boundary values for numeric conditions
- For conditional ensures, verify each branch fires under the correct condition and that `if` guards read resulting state, not pre-rule state
- For entity-creating ensures, verify the created entity has the specified field values
- For `let` bindings, verify the bound value is correct and available to subsequent clauses
- For `not exists` in ensures, verify the entity is removed from the system
- For bulk updates (`for` in ensures), verify the postcondition applies to every element in the collection
- For rule-level `for` iteration, verify the rule body executes for each matching element
- For chained triggers (trigger emission in ensures), verify the downstream rule fires with the correct parameters

**State transition tests** (per entity with status enum):
- Valid transitions succeed via their rules
- Invalid transitions are rejected (no rule allows them)
- Terminal states have no outbound transitions
- For `transitions_to` triggers, verify the rule does not fire on entity creation
- For `becomes` triggers, verify the rule fires both on creation and on transition

**State-dependent field tests** (per field with a `when` clause):
- Verify the field is present (has a meaningful value) when the entity is in a qualifying state
- Verify the field is absent (has no meaningful value) when the entity is outside the qualifying states
- When a rule transitions into the `when` set, verify it sets the field (entering obligation)
- When a rule transitions out of the `when` set, verify it clears the field (leaving obligation)
- When a rule moves within the `when` set, verify no obligation fires (field is already present)
- When two rules converge on the same qualifying state, verify both set the field
- Verify accessing a `when`-qualified field without a state guard is rejected
- For derived values computed from `when`-qualified fields, verify the inferred `when` set matches the intersection of the inputs' `when` sets

How "present" and "absent" are tested depends on how the implementation represents the entity. When the entity is modelled as a sealed hierarchy or variant type (one class per lifecycle state), presence and absence are structural: the field exists on one variant and not another. The compiler enforces the `when` clause. When the entity is modelled as a single mutable class with nullable fields, test that the field is meaningfully populated in qualifying states and null or empty outside them. Both representations are valid for the same spec; the choice is an implementation concern. The spec-level concept is lifecycle-dependent presence; the test adapts to the representation.

**Temporal tests** (per time-based trigger):
- Before deadline: rule does not fire, state unchanged
- At deadline: rule fires, postconditions hold
- After deadline: rule has already fired, does not re-fire
- Verify `requires` guard prevents re-firing when entity remains in the qualifying state
- Verify temporal triggers on optional fields do not fire when the field is null

**Communication tests** (per Notification/Email/etc):
- Verify communication is triggered by the correct rule
- Verify recipient is correct
- Verify template and data are passed

**Surface tests** (per surface):
- Exposure tests: verify each item in `exposes` is accessible to the specified party
- For `for` iteration in `exposes`, verify each element in the collection is exposed
- Provides availability: verify provided operations appear when their `when` conditions are true
- Provides unavailability: verify provided operations are hidden when `when` conditions are false, including when the corresponding rule's `requires` clauses are not met
- Actor identification: verify only entities matching the actor's `identified_by` predicate can interact
- For actors with `within`, verify interaction is scoped to the declared context (e.g. actions in one workspace do not affect another)
- Party restriction: verify the surface is not accessible to other party types
- Context scoping: verify the surface instance is absent when no entity matches the `context` predicate
- Related surface navigation: verify navigation to related surfaces resolves to the correct context entity
- Guarantee tests: verify stated `@guarantee` annotations hold across the boundary
- Timeout tests: verify the referenced temporal rule fires within the surface's context

**Contract declaration tests** (per `contract` declaration):
- Verify the implementation satisfies each typed signature in the contract
- Verify `@invariant` annotations are honoured across the boundary
- For surfaces that `demand` a contract, verify the counterpart provides all signatures
- For surfaces that `fulfil` a contract, verify this surface supplies all signatures

**Cross-module tests** (per `use` declaration):
- Verify qualified entity references resolve to the imported module's entities
- Verify rules that respond to external triggers (state transitions, trigger emissions) from imported modules fire correctly
- Verify external entities referenced as type placeholders accept the consuming spec's concrete type

**Cross-rule interaction tests** (per rule with entity-creating ensures):
- Verify guards prevent duplicate entity creation when sibling rules re-trigger on the same parent
- Verify `provides` entries are unavailable when any `requires` conjunct of the corresponding rule is false

**Scenario tests** (per specification):
- Happy path through the main rule chain (follow chained triggers from entry point to terminal state)
- Edge cases and error paths at each decision point
- When two rules could fire on the same entity, verify the resulting state is consistent regardless of order

**Concurrency note:** The language reference does not formally define rule atomicity or evaluation order. Treat rules as atomic (completing entirely or not at all) as a reasonable default.

**Constructs without dedicated test categories:** `given` block bindings are exercised through rule tests that reference them. `deferred` specifications are tested in their own modules. `open question` declarations are checker warnings, not test obligations. `exists` as an `if`-condition is covered by rule conditional ensures tests. Discard bindings (`_`) have no observable effect to test.
