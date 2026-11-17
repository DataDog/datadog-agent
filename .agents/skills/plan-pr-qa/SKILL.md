---
name: plan-pr-qa
description: Generate independently assignable QA cards from a pull request, grounded in its intended behavior, code changes, and existing validation.
---

**Ownership:** maintained by `@DataDog/agent-devx`.

Create a QA plan that another engineer or agent can execute without the author
explaining the change. Generate the cards only. Creating tickets, provisioning
environments, and running QA are separate actions.

## Establish the change

Read the PR description, diff, relevant tests, and material review discussion.
Inspect surrounding code and direct consumers where necessary to understand
the externally observable behavior.

Identify what should change and remain unchanged; which environments,
configurations, inputs, and lifecycle transitions matter; and what existing
validation establishes or leaves unverified.

Distinguish documented intent from your inference. Do not silently turn the
implementation's current behavior into the expected result. Treat review concerns
as hypotheses until reconciled with the intended contract and final diff.

Record the PR URL and analyzed head commit. If a release/build is supplied,
verify that it contains the change and inspect relevant differences. Otherwise,
target the PR head and leave executable artifact selection explicit.

## Decide whether QA is worthwhile

Use a low inclusion threshold: include any plausible effect on installation,
runtime behavior, configuration, collected data, compatibility, permissions,
or resource use. Include internal refactors when they could affect these.

Existing tests and labels such as qa/done inform the plan; they do not
automatically exclude a change. For clearly nonfunctional changes, return a
short exclusion rationale. When uncertain, explain the uncertainty and propose
a focused check.

## Divide the work

Generate as many cards as the behavioral boundaries warrant. Split cards when
they require materially different environments, credentials, fixtures, or
observations—for example, Agent output versus real-backend interpretation, or
standalone Windows versus domain-account upgrades.

Each card must have a coherent setup and an independently reportable result.
Avoid splitting one necessary lifecycle sequence into disconnected tasks. If a
card needs different credential or topology fixtures, split it or give each
variant its own assignable preconditions and verdict.
Prefer a small, meaningful set of scenarios over an exhaustive Cartesian
product. Cover each distinct changed contract; mark costly extra variants separately
without turning them into catch-all cards. Explain significant omissions.
Missing tooling can block a variant, but does not make a changed path optional.

Prefer checks against an executable Agent and observable output. A new
instrumented harness or a set of new unit tests is not a prerequisite for
ordinary QA. If a transition cannot be induced through a supported runtime
control, record that coverage gap or propose a separate diagnostic follow-up.

## Write each card

Include:

- Title and source: PR, analyzed commit, and relevant evidence links.
- Purpose: behavioral claim and plausible failure being checked.
- Target: candidate build/version and any required upgrade baseline.
- Prerequisites: platform, architecture, privileges, credentials, existing
  state, and required observation points.
- Procedure: concrete actions paired with expected observable results.
- Evidence: what the executor must retain to substantiate the result.
- Existing coverage: what this card repeats, extends, or independently checks.
- Unknowns: unresolved assumptions or requirements that prevent execution.

Describe the required environment without prescribing a provisioning system
unless the scenario depends on it. Name concrete inputs and the means of
performing each material action; avoid instructions such as "submit a valid
request" without an identified fixture or interface. When a sender, command,
or fixture is unavailable, identify exactly what the executor must resolve.
Where inputs have independent meanings or precedence, vary them separately or
use distinguishable values so accidental copying cannot pass. Reuse concrete
versions, inputs, and fixture definitions supplied by the sources rather than
replacing them with an unspecified "valid fixture" or approximate baseline.
A compact case table can state inputs and expected observations without repeating procedures.

Include negative controls, failure conditions, or lifecycle transitions when
they address the change. For absence assertions, establish that the workload
ran and the observation path worked.

Use documented timing and tolerances where available. Label proposed bounds
as proposals rather than inventing product guarantees. Assert the contract at
the actual observation boundary: helper behavior may be changed by later
normalization, transport, or ingestion. Do not freeze unrelated fields or
internal retry details without evidence they are part of the contract.
Uncertainty about one output field should not discard a useful scenario:
retain the known assertions and identify only the unresolved expectation.

Use the least costly observation that establishes the claim. Local payload
assertions need not depend on real backend access. A mock or fake intake
establishes only the behavior it actually observes; keep downstream semantics
that require a real service in a separate card.

## Check the plan

Before returning it, ask:

- Would this detect the plausible regression, or merely show healthy startup?
- Can a fresh executor determine success without consulting the author?
- Are the fixtures and credentials compatible with every step?
- Could missing data accidentally count as a pass?
- Are inferred expectations and blocked prerequisites visible?

Return the inclusion decision, a brief behavior summary, and the cards. Keep
shared target/source metadata once; avoid repeating uncertainty boilerplate.
Cards may reference that header, but must not depend on another card being run.
All cards begin as NOT RUN. Do not imply existing tests were independently
executed or that the plan has already validated the change.
