---
name: propagate
description: "Generate tests from Allium specifications. Use when the user wants to propagate tests, generate test files from a spec, write tests for a specification, create property-based tests, produce state machine tests, check test coverage against spec obligations, or understand what tests a specification requires."
---

# Propagation

This skill generates tests from Allium specifications. Propagation is how plants reproduce from cuttings of the parent: the spec is the parent, the tests are the offspring.

Deterministic tools guarantee completeness (every spec construct maps to a test obligation). You handle the implementation bridge: correlating spec constructs with code, generating tests in the project's conventions.

## Prerequisites

Before propagating tests, you need:

1. **An Allium spec** — the `.allium` file describing the system's behaviour
2. **A target codebase** — the implementation to test
3. **Test obligations** — from `allium plan <spec>` (JSON listing every required test)
4. **Domain model** — from `allium model <spec>` (JSON describing entity shapes, constraints, state machines)

If the CLI tools are not available, derive test obligations manually from the spec using the test-generation taxonomy in `references/test-generation.md`.

## Modes

### Surface mode

Generates boundary tests from surface declarations. Use when the user wants to test an API, UI contract or integration boundary.

For each surface in the spec:

1. **Exposure tests** — verify each item in `exposes` is accessible to the specified actor, including `for` iteration over collections
2. **Provides tests** — verify operations appear when their `when` conditions are true and are hidden otherwise, including when the corresponding rule's `requires` clauses are not met
3. **Actor restriction tests** — verify the surface is not accessible to other actor types
4. **Actor identification tests** — verify only entities matching the actor's `identified_by` predicate can interact; for actors with `within`, verify interaction is scoped to the declared context
5. **Context scoping tests** — verify the surface instance is absent when no entity matches the `context` predicate
6. **Contract obligation tests** — verify `demands` are satisfied by the counterpart, `fulfils` are supplied by this surface, including all typed signatures
7. **Guarantee tests** — verify `@guarantee` annotations hold across the boundary
8. **Timeout tests** — verify referenced temporal rules fire within the surface's context
9. **Related navigation tests** — verify navigation to related surfaces resolves to the correct context entity

### Spec mode

Walks the full test obligations document. Use when the user wants comprehensive test coverage for the entire specification.

Categories from the test-generation taxonomy:

- **Entity and value type tests** — fields, types, optional (`?`) null handling, `when`-clause state-dependent presence, relationships, join lookups, equality
- **Enum tests** — comparability across named enums, membership tests, inline enum isolation
- **Sum type tests** — variant fields, type guards, exhaustiveness, creation via variant name, base `.created` trigger narrowing
- **Derived value and projection tests** — computation, filtering, `-> field` extraction, parameterised derived values, `now` volatility, collection operations
- **Default instance tests** — unconditional existence, field values, cross-references between defaults
- **Config tests** — defaults, overrides, mandatory parameters, expression-form defaults, qualified references, config chains
- **Invariant tests** — post-rule verification, edge cases, implication logic, entity-level invariants
- **Rule tests** — success/failure/edge cases, conditionals (ensuring `if` guards read resulting state), entity creation, removal, bulk updates, rule-level `for` iteration, `let` bindings, chained triggers
- **State transition tests** — valid/invalid transitions, terminal states, `transitions_to` vs `becomes` semantics
- **Temporal tests** — deadline boundaries, re-firing prevention, optional field null behaviour
- **Surface tests** — exposure, availability, actor identification with `within` scoping, context scoping, related navigation
- **Contract tests** — signature satisfaction, `@invariant` honouring, `demands`/`fulfils` direction
- **Cross-module tests** — qualified entity references, external trigger responses, type placeholder substitution
- **Cross-rule interaction tests** — duplicate creation guards, provides availability
- **Transition graph tests** — every declared edge is reachable via its witnessing rule, undeclared transitions are rejected, terminal states have no outbound rules, non-terminal states have at least one exit, exact correspondence between enum values and graph edges
- **State-dependent field tests** — presence when in qualifying state, absence when outside, presence obligations on entering the `when` set, absence obligations on leaving, no obligation when moving within or outside, convergent transitions all set the field, guard required to access `when`-qualified fields, derived value `when` inference via input intersection
- **Scenario tests** — happy path, edge cases, order independence

## Test output kinds

### 1. Assertion-based tests

For deterministic obligations: field presence, enum membership, transition validity, surface exposure, state-dependent field presence and absence. These are standard unit/integration tests.

### 2. Property-based tests

For invariants and rule properties. Each expression-bearing invariant becomes a PBT property:
- Generate a valid entity state using the generator spec
- Apply a sequence of rules (following the transition graph when declared, or deriving valid sequences from rules alone)
- Check the invariant holds at every step

Use the project's PBT framework:

| Language | Framework | Discovery |
|----------|-----------|-----------|
| TypeScript | fast-check | `package.json` |
| Python | Hypothesis | `pyproject.toml` |
| Rust | proptest | `Cargo.toml` |
| Go | rapid | `go.mod` |
| Elixir | StreamData | `mix.exs` |

Fall back to assertion-based tests if no PBT framework is present.

### 3. State machine tests

For entities with status enums. When a transition graph is declared, walk every path through the graph. When no graph is declared, derive valid transitions from rules.
- Verify transitions succeed via witnessing rules
- Verify rejected transitions fail
- Verify state-dependent fields are present or absent at each state per their `when` clauses
- Verify invariants hold at each state

State machine tests require an **action map**: a function per transition edge that takes the entity in the source state and produces it in the target state by calling the actual implementation code. Without this map, the test framework can describe valid paths through the graph but cannot execute them.

To build the action map:
1. For each edge in the transition graph, find the witnessing rule in the spec
2. Find the code implementing that rule (the implementation bridge)
3. Write a test action that sets up the preconditions (`requires` clauses), invokes the code, and returns the entity in the target state
4. Register the action under the `(from_state, to_state)` key

Once the map is built, the PBT framework can walk random valid paths: start at any non-terminal state, pick a random outbound edge, apply its action, check all entity-level invariants, repeat. The path length and starting state are generated randomly. This is the fullest expression of the spec's transition graph as a test.

## The implementation bridge

You correlate spec constructs with implementation code, the same way the weed agent correlates for divergence checking.

### For surface tests

Map surfaces to their implementation:
- API surfaces map to endpoints (REST routes, GraphQL resolvers, gRPC services)
- UI surfaces map to components or pages
- Integration surfaces map to message handlers or SDK methods

Discover the mapping by reading the codebase. Look for naming patterns, route definitions and handler registrations.

### For internal tests

For each rule in the spec:
1. Find the code implementing the rule (service method, event handler, state machine transition)
2. Determine how to instantiate the entities involved (factories, builders, fixtures)
3. Determine how to invoke the rule (API call, method call, event dispatch)
4. Determine how to assert postconditions (database queries, return values, event assertions)

### For temporal tests

Temporal triggers (deadline-based rules) need a controllable time source in the test. If the implementation uses wall-clock time (`Instant.now()`, `System.currentTimeMillis()`), the test cannot reliably position itself before, at or after a deadline.

Before attempting temporal tests, check whether the component accepts an injected clock or time parameter. Common patterns: a `Clock` parameter on the constructor, an epoch-millisecond argument on the method, a `TimeProvider` interface. If the seam exists, inject a controllable time source. If it does not, flag this as a test infrastructure gap: the temporal tests cannot be generated until the component supports time injection. Do not attempt to test temporal behaviour by sleeping or racing against wall-clock time.

### For cross-module trigger chains

When a rule emits a trigger that another spec's rule receives (e.g. the Arbiter emits `ClerkReceivesEvent`, the Clerk handles it), testing the chain requires multiple components wired together.

Before generating cross-module tests:
1. Trace the trigger emission graph from the plan output: which rules emit triggers, and which rules in other specs receive them
2. Check whether the codebase has an existing integration test fixture that wires the participating components (a pipeline test, an end-to-end test helper, a test harness class)
3. If a fixture exists, reuse it. Cross-module tests should compose existing wiring, not rebuild it
4. If no fixture exists, generate only the test skeleton with TODOs marking where component wiring is needed

Cross-module tests are integration tests by nature. They verify that the spec's trigger chains are faithfully implemented across component boundaries, but the setup cost is high. Prioritise them after single-component tests are passing.

### Reusing existing tests

When exploring the codebase, note which spec obligations are already covered by existing tests. An existing integration test that exercises the happy path from event submission through to acknowledged output already covers multiple `rule_success` obligations and the end-to-end scenario.

When an existing test covers a spec obligation, reference it rather than generating a duplicate. The propagate skill's value at the integration level is verifying that coverage is complete against the spec's obligation list, identifying gaps, and generating tests to fill them. Replacing working hand-written tests with generated equivalents adds no value.

### For deferred specs

Deferred specifications are fully specified in separate files. When the target codebase doesn't include the deferred spec's module, generate a test stub with a placeholder:

```typescript
// TODO: deferred spec — InterviewerMatching.suggest
// This behaviour is specified as deferred. Provide a mock or skip.
```

## Process

1. **Read the spec** — understand entities, rules, surfaces, invariants, transition graphs, state-dependent fields, contracts, config, defaults
2. **Read test obligations** — from `allium plan` output or manual derivation
3. **Read domain model** — from `allium model` output or manual derivation
4. **Explore the codebase** — find existing tests, test framework, entity implementations, rule implementations
5. **Map constructs to code** — correlate spec entities/rules/surfaces with implementation classes/functions/endpoints
6. **Generate tests** — produce test files following the project's conventions
7. **Verify tests compile/run** — ensure generated tests are syntactically valid

### Discovery checklist

Before generating tests, establish:

- [ ] Test framework and runner (Jest, pytest, cargo test, etc.)
- [ ] PBT framework if present (fast-check, Hypothesis, proptest, etc.)
- [ ] Test file location conventions (co-located, `__tests__/`, `tests/`, etc.)
- [ ] Entity/model location and patterns (classes, interfaces, structs)
- [ ] Factory/fixture patterns for test data
- [ ] How state transitions are implemented (methods, events, state machines)
- [ ] How surfaces are implemented (routes, controllers, resolvers)
- [ ] Existing test helpers or utilities
- [ ] Whether components accept injected time sources for temporal tests
- [ ] Whether an integration test fixture exists for cross-module trigger chains
- [ ] Which spec obligations are already covered by existing tests

### Generator awareness

When generator specs are available, use them to produce valid test data:

- Respect field types and constraints
- For entities with transition graphs, generate entities at specific lifecycle states with correct field presence per `when` clauses (e.g. a `shipped` Order has `tracking_number` and `shipped_at` populated; a `pending` Order does not)
- For invariants, generate states that exercise boundary conditions
- For config parameters, use declared defaults unless testing overrides

## Interaction with other tools

- **distill** produces specs from code. Those specs feed propagate.
- **weed** checks alignment. After propagating tests, weed verifies spec-code match.
- **tend** evolves specs. After spec changes, run propagate again to update tests.
- **elicit** builds specs through conversation. Once a spec is ready, propagate generates tests.

## Limitations

- Generated tests are a starting point. They may need adjustment for project-specific patterns.
- The implementation bridge is LLM-mediated. Complex or unusual codebases may need manual guidance on the mapping.
- Cross-module test generation is not yet supported. Each spec generates tests independently.
- Runtime trace validation and model checking are separate workstreams.
