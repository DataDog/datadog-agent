---
name: e2e-audit
description: Judge whether Agent behavior belongs in a new-e2e, integration, or unit test
allowed-tools: Read, Glob, Grep, Bash
argument-hint: "<behavior description> | <path-to-test-file-or-dir> [more paths...]"
model: sonnet
---

Decide whether proposed or existing Agent behavior belongs in a full
`new-e2e` test, an integration test, or a unit test. Produce only a verdict
and rationale. Never edit, move, or delete tests.

## Principle

Choose the cheapest test that preserves the boundary and failure mode that
matter. Do not ask only whether the behavior *can* be mocked; ask what the test
would stop validating if it were mocked.

### E2E justified

Use `new-e2e` when the deployed Agent or its real environment is part of the
behavior being validated. This includes boundaries such as:

- installation, packaging, permissions, or merged deployed configuration;
- process or service lifecycle, cross-process communication, or CLI behavior
  that depends on the running Agent;
- kernel, operating-system, cloud, container-orchestrator, or network behavior
  that cannot be represented faithfully by a local dependency;
- user-visible data reaching fakeintake when the Agent's real assembly,
  configuration, encoding, or forwarding path is material to the test.

A behavior is not E2E-worthy merely because the current test reaches it through
SSH, a CLI, or remote infrastructure. If those layers add no relevant coverage,
use a lower-level test.

### Should be an integration test

Use an integration test when the important boundary can be preserved locally,
for example by wiring components with `fx.Test`, using a local fakeintake, or
using a real local daemon, driver, or hardware dependency. A real local
dependency does not by itself require `new-e2e`.

### Should be a unit test

Use a unit test when the behavior is isolated logic and does not require a real
component graph, process, or external dependency.

## Procedure

### Proposed behavior

1. State the failure the test is intended to catch.
2. Identify the smallest environment that preserves that failure mode.
3. Choose the verdict above. If E2E is justified, name the real boundary that
   would be lost in a lower-level test.

If the description does not establish the relevant boundary, ask for the
missing information or return an explicitly uncertain verdict.

### Existing tests

1. Resolve every concrete suite. For directories, find all `*_test.go` files.
   Follow shared suites, setup code, helpers, and provisioners rather than
   judging files in isolation.
2. Read each test and subtest, including setup, gating, environment updates,
   and cleanup. Inspect the production code it exercises before deciding that
   the behavior can be tested at a lower level.
3. Classify each subtest using the principle above.
4. Give the suite-level verdict:
   - If any subtest needs the deployed Agent boundary, the suite remains E2E.
   - If none does, recommend an integration or unit test as appropriate.
5. Even when the suite remains E2E, mention lower-level candidates when moving
   them would materially reduce runtime, reprovisioning, flakiness, or
   duplication. Do not assume all cost is paid only once at suite setup.
6. When reviewing several suites, briefly note redundant provisioners or
   equivalent coverage that could be consolidated.

For large reviews, inspect files in parallel if possible, then verify and
synthesize the results.

## Output

Use one of these verdicts:

- **E2E justified**
- **Should be an integration test**
- **Should be a unit test**

For proposed behavior, return one verdict and a short reason. For existing
tests, return one verdict per concrete suite, naming the subtest or boundary
that determines it. Add only material uncertainty, lower-level candidates, or
consolidation opportunities.

Do not propose implementation changes unless the user asks for them.
