# worktickets/

Local task tracking for the OpenMetrics core check branch
(`sopell/openmetrics-core-check-go`). This branch adds the Go
OpenMetrics scraper from scratch, so these tickets aren't filed against
an upstream system — they're the work-in-flight to-do list specific to
this branch.

## What's tracked here

Divergences against the production Python OpenMetrics check
(`datadog_checks.base.checks.openmetrics.v2.OpenMetricsBaseCheckV2`)
discovered by the differential-testing harness at
`pkg/collector/corechecks/openmetrics/differential/`. Each ticket has:

- The behavior gap, with a minimal repro
- A pointer to the failing subtest in the adversarial catalog
- A suggested fix shape
- A severity rationale
- Verification criteria for the fix

## Tooling

This directory uses [taskmd](https://github.com/DataDog/taskmd) for
markdown-native task files. To work with it from inside the worktree:

```bash
taskmd list --tasks-dir worktickets
taskmd status 07001 in-progress --tasks-dir worktickets
echo 'task body' | taskmd new --slug some-slug --priority p2 \
    --tasks-dir worktickets
```

## Current backlog

| ID | Priority | Status | Slug |
|----|----------|--------|------|
| 07001 | p0 | ready | per-line-error-recovery |
| 07002 | p1 | ready | openmetrics-type-keywords |
| 07003 | p1 | ready | openmetrics-exemplar-trailers |
| 07004 | p1 | ready | float64-overflow-aborts-scrape |
| 07005 | p2 | ready | conflicting-type-declaration |
| 07006 | p3 | ready | spec-strictness-divergences-informational |
| 07007 | p1 | done | share-labels-divergence |
| 07008 | p2 | done | transformer-knob-divergences |

07001-07006 came from the payload-axis harness (parser-level bugs).
07007-07008 came from the config-axis harness (transformer/matcher
pipeline bugs).
They were closed after correcting an inverted `share_labels` generator and
rerunning 2,000 fixture-aware comparisons with no divergences.

Post-rebase parser triage confirmed 07003-07005 remain reproducible. In
particular, 07003 is now scoped to content-type parser selection: Ali's
OpenMetrics parser accepts exemplars, while the Prometheus parser selected for
`text/plain; version=0.0.4` still rejects them.

## Lifecycle

When the Go scraper merges upstream and replaces the Python check in
production, this directory either:

- Migrates to wherever the team's permanent OpenMetrics issue tracker
  lives (a Datadog Jira project, say); or
- Gets archived/deleted along with the differential harness once the
  Go check has been stable in prod for one full release cycle.
