---
name: e2e-audit
description: Judge whether Agent behavior belongs in a new-e2e, integration, or unit test
allowed-tools: Read, Glob, Grep, Bash
argument-hint: "<behavior description> | <path-to-test-file-or-dir> [more paths...]"
model: sonnet
---

Decide whether assertions about proposed or existing Agent behavior belong in
a full `new-e2e` test, an integration test, or a unit test. Produce only
verdicts, or the check that owns a question, with their rationale. Never edit,
move, or delete tests.

## Criteria

Read `doc/guidelines/testing/test-categories.md` before classifying anything.
It defines what each test category owns, the questions that decide between
them, and when E2E coverage duplicated at a lower level is still worth keeping.
Put each assertion in the cheapest category that still catches its failure.

The doc has four categories and this skill three verdicts: an assertion in the
doc's **Binary test** category is classified **E2E justified** and marked as a
binary-test candidate (see Output): there is no agreed way to run binary tests
yet, even for a binary that has a harness. A question the doc assigns to its
other checks, such as package size, gets no verdict: name the check that owns
it.

## Procedure

### Proposed behavior

1. List the observable claims that the proposed test would assert.
2. For each assertion, state the failure it is intended to catch.
3. Identify the smallest environment that preserves each failure mode: the
   components it must keep real, and whether they depend on the deployed
   platform.
4. Classify each assertion. If E2E is justified, name the real boundary that
   would be lost in a lower-level test.
5. Give an overall verdict based on the assertion requiring the broadest real
   boundary, counting binary-test candidates as E2E justified.

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
   environment that preserves that failure (the components it must keep real,
   and whether they depend on the deployed platform), note equivalent
   lower-level coverage, and classify it using the criteria doc.
5. Give the suite-level verdict:
   - If any assertion is E2E justified, binary-test candidates included, the
     suite remains E2E; name the assertion and boundary that determine this
     verdict.
   - If none is, recommend an integration or unit test as appropriate.
6. Recommend duplicating suitable assertions at a lower level when that would
   provide faster PR feedback, more deterministic failures, or easier
   debugging, even if the suite initially remains E2E.
7. After accounting for that lower-level coverage, identify the failure modes
   and real boundaries that only the E2E test preserves; a binary-test
   candidate's boundary counts as one. If nearly all material assertions are
   duplicated and no meaningful E2E-only boundary remains, recommend removing
   the E2E test. Weigh its residual value against runtime, provisioning,
   flakiness, and maintenance cost; do not assume all cost is paid only once at
   suite setup.
8. When reviewing several suites, briefly note redundant provisioners or
   equivalent coverage that could be consolidated.

For large reviews, inspect files in parallel if possible, then verify and
synthesize the results.

## Output

Use one of these verdicts:

- **E2E justified**
- **Should be an integration test**
- **Should be a unit test**

or no verdict, naming the check that owns the question.

For proposed behavior, classify each expected assertion and return an overall
verdict with a short reason. For existing tests, classify each material
assertion and return one verdict per concrete suite, naming the assertion and
boundary that determine it. State what the E2E test uniquely validates after
accounting for lower-level coverage, or say that no material E2E-only boundary
remains. When several assertions differ, use a concise table with these
columns: assertion, failure caught, smallest environment, and classification.
Add only material uncertainty, lower-level candidates, or consolidation
opportunities.

Mark an **E2E justified** assertion as a **binary-test candidate** when the
only boundary it needs is the built binaries running on one machine: no
installed package, service manager, specific kernel, cloud, or orchestrator.
Write it as `E2E justified (binary-test candidate)` in the classification and
name the binaries involved. When the criteria doc lists a binary test harness
for those binaries, name it as a lower-level candidate.

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

**Input:** Verify that `agent config` prints the configuration the running
Agent merged from `datadog.yaml` and `DD_*` environment variables.

**Output:** **E2E justified (binary-test candidate)** — The CLI and the running
`agent` binary must both be real, talking over the Agent's IPC, but nothing
needs the installed package, service manager, or a cloud host: the built
`agent` binary started with a local configuration on one machine preserves the
failure.

**Input:** Verify that the configuration parser rejects a negative timeout and
applies the default when the field is absent.

**Output:** **Should be a unit test** — This is isolated parsing and validation
logic that does not require a real component graph, process, or external
dependency.

Do not propose implementation changes unless the user asks for them.
