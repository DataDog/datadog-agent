---
name: e2e-audit
description: Judge whether Agent behavior belongs in a new-e2e, integration, or unit test
allowed-tools: Read, Glob, Grep, Bash
argument-hint: "<behavior description> | <path-to-test-file-or-dir> [more paths...]"
model: sonnet
---

Decide whether assertions about proposed or existing Agent behavior belong in
a full `new-e2e` test, an integration test, or a unit test. Produce only a
verdict and rationale. Never edit, move, or delete tests.

## Principle

Analyze observable claims and failure modes, not entire test files or individual
`assert` or `require` calls. Group checks that validate the same contract as one
assertion, including implicit claims such as successful installation, startup,
or command execution.

Choose the cheapest test that preserves the boundary and failure mode that
matter. Do not ask only whether an assertion *can* be made with mocks; ask what
the assertion would stop validating if its dependencies were replaced.

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

### Layered coverage and residual E2E value

Duplicating an E2E assertion in a unit or integration test is useful when the
lower-level test preserves its failure mode: it provides faster PR feedback,
more deterministic failures, and easier debugging. Do not treat this useful
duplication as waste by itself.

After identifying lower-level coverage, reevaluate what the E2E test uniquely
validates. Repeating the same assertion through the deployed Agent can still be
valuable when it catches assembly, configuration, packaging, lifecycle, or
forwarding failures that the lower-level test cannot. If nearly all material
assertions have equivalent lower-level coverage and the E2E test preserves no
meaningful additional boundary, its feedback no longer justifies its
provisioning, runtime, and maintenance cost; recommend removing it. Base this
decision on residual failure coverage, not only the number of duplicated
assertions.

## Procedure

### Proposed behavior

1. List the observable claims that the proposed test would assert.
2. For each assertion, state the failure it is intended to catch.
3. Identify the smallest environment that preserves each failure mode.
4. Classify each assertion. If E2E is justified, name the real boundary that
   would be lost in a lower-level test.
5. Give an overall verdict based on the assertion requiring the broadest real
   boundary.

If the description does not establish the relevant assertions or boundaries,
ask for the missing information or return an explicitly uncertain verdict.

### Existing tests

1. Resolve every concrete suite. For directories, find all `*_test.go` files.
   Follow shared suites, setup code, helpers, and provisioners rather than
   judging files in isolation.
2. Read each test and subtest, including setup, gating, environment updates,
   and cleanup. Inspect the production code it exercises before deciding that
   an assertion can be tested at a lower level.
3. Inventory the observable claims in each test. Include implicit claims from
   setup and lifecycle operations, and group checks that cover the same
   contract.
4. For each assertion, state the failure it catches, identify the smallest
   environment that preserves that failure, note equivalent lower-level
   coverage, and classify it using the principle above.
5. Give the suite-level verdict:
   - If any assertion needs the deployed Agent boundary, the suite remains E2E;
     name the assertion and boundary that determine this verdict.
   - If none does, recommend an integration or unit test as appropriate.
6. Recommend duplicating suitable assertions at a lower level when that would
   provide faster PR feedback, more deterministic failures, or easier
   debugging, even if the suite initially remains E2E.
7. After accounting for that lower-level coverage, identify the failure modes
   and real boundaries that only the E2E test preserves. If nearly all material
   assertions are duplicated and no meaningful E2E-only boundary remains,
   recommend removing the E2E test. Weigh its residual value against runtime,
   provisioning, flakiness, and maintenance cost; do not assume all cost is
   paid only once at suite setup.
8. When reviewing several suites, briefly note redundant provisioners or
   equivalent coverage that could be consolidated.

For large reviews, inspect files in parallel if possible, then verify and
synthesize the results.

## Output

Use one of these verdicts:

- **E2E justified**
- **Should be an integration test**
- **Should be a unit test**

For proposed behavior, classify each expected assertion and return an overall
verdict with a short reason. For existing tests, classify each material
assertion and return one verdict per concrete suite, naming the assertion and
boundary that determine it. State what the E2E test uniquely validates after
accounting for lower-level coverage, or say that no material E2E-only boundary
remains. When several assertions differ, use a concise table with these
columns: assertion, failure caught, smallest environment, and classification.
Add only material uncertainty, lower-level candidates, or consolidation
opportunities.

### Examples

**Input:** Verify that installing the Agent package creates a running service
with the expected permissions and that data reaches fakeintake after a reboot.

**Output:** **E2E justified** — The package installation, service lifecycle,
permissions, reboot, and forwarding path are the behavior under test; a
lower-level test would not preserve those deployed-system boundaries.

**Input:** Verify that the assembled Agent components transform a payload and
send it to a local fakeintake.

**Output:** **Should be an integration test** — A locally assembled component
graph and fakeintake preserve the component wiring and payload boundary without
provisioning remote infrastructure.

**Input:** Verify that the configuration parser rejects a negative timeout and
applies the default when the field is absent.

**Output:** **Should be a unit test** — This is isolated parsing and validation
logic that does not require a real component graph, process, or external
dependency.

Do not propose implementation changes unless the user asks for them.
