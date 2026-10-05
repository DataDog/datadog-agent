# Quality Gate Private Action Runner

## Overview

This quality gate experiment measures the idle memory footprint of the
standalone Private Action Runner (PAR). It runs the `privateactionrunner`
binary on its own, not the full Agent, so `total_pss_bytes` reflects only the
runner's resident memory.

## Owners

- **Teams**: @DataDog/action-platform

## Scenario

Models a runner that has started successfully and is waiting for work:

- `private_action_runner.enabled: true` with self-enrollment disabled and a
  throwaway static identity (`urn` + `private_key`), so the runner gets past
  startup without a real Datadog backend.
- OPMS polling is routed to a local lading HTTP blackhole
  (`DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS`). No tasks are ever returned, so no
  actions run.
- `generator: []` in lading: no workload is generated.
- `DD_INTERNAL_PAR_ENABLE_TELEMETRY` exposes `/telemetry` so SMP also captures
  the runner's Go runtime metrics.

Anything registered or allocated at startup counts toward this gate, including
action bundles that are never invoked. Adding a bundle or a dependency to the
runner's startup path is the most common reason this gate moves.

## Enforcements

- Memory usage (`total_pss_bytes`) is below `upper_bound` in `experiment.yaml`.
  Every replicate must stay under the bound, so the bound needs headroom above
  typical runs, not just above the mean.

## Changing the bound

When raising `upper_bound`, also raise `memory_allotment` to ~30% above it
(see the comments in `experiment.yaml`). Bound increases need an approved
exception; record the measured impact and link it in the PR.

## Other Links

- [Performance Quality Gates](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4294836779/Performance+Quality+Gates)
- [PAR Authored Script Execution - Quality Gates Decision Records](https://datadoghq.atlassian.net/wiki/spaces/ABLD/pages/7259162694/Private+Action+Runner+Authored+Script+Execution+-+Quality+Gates+Decision+Records)
