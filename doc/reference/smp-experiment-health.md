# SMP experiment health report contract

This contract supports experiment owners who need evidence before they promote an advisory experiment to a quality gate. It preserves observed failures independently of CI policy. The first version covers container experiments only.

## Isolation and activation

The proposed service is `datadog-agent-smp-experiment-health`. Every testcase has `smp.report_kind:experiment_health` and `smp.report_version:1`. Suites use `smp.health.<experiment>`. Names remain stable across branches, commits, and reruns.

The CI change generates artifacts and never uploads the new XML. It does not add the XML to GitLab's `reports:junit` section. The existing explicit `outputs/junit.xml` upload remains unchanged. Platform activation needs a separate change with the new service and explicit XML path.

## Outcomes

| Kind | Pass | Failure | Error |
| --- | --- | --- | --- |
| Bounds | All comparison replicas pass, with positive total | At least one replica fails | Missing result, zero replicas, or invalid counts |
| Optimization goal | No significant degradation | Significant change without improvement | Configured goal has no usable result |
| Analysis | No synthetic pass row | No performance failure row | Analysis error or statistical instability |
| Execution | No synthetic pass row | No performance failure row | Failed replica groups or absent selected experiment |

An ignored regression remains a failure. `smp.ignores_regressions` records the experiment policy. `smp.statistically_erratic` records the analysis flag. Statistical instability also creates an analysis error row. Recovered replica failures remain visible with their original `is_dead` value.

The inventory comes from the same selection manifest and inputs as submission. The renderer reads expected checks from the selected experiment definitions. It emits errors for missing experiments and checks. Bounds-only experiments have no synthetic optimization goal.

Bounds attributes describe comparison replicas only. Numeric properties remain strings in JUnit. The report preserves replica counts, limits, observed ranges, percent change, and confidence intervals for inspection. It does not promise numeric aggregation in Datadog before an ingestion test.

## Identity and attribution

Each evaluation ID hashes the CI job ID, SMP job ID, experiment, check kind, and check name. A CI retry has a distinct identity. A repeated copy of one artifact retains the same identity.

PR context uses the existing branch lookup. Exactly one match supplies the PR number and URL. Absent, ambiguous, and failed lookups remain visible and use context `other`. Main uses context `main`. A branch name alone does not identify a PR.

Context includes the actual baseline and comparison SHAs, parent pipeline ID, child job ID, job URL, and analysis end time. The context builder copies an explicit list of fields. It does not copy tokens or the full environment.

## Dashboard views

The local [dashboard design](../../test/regression/reporting/dashboard-design.json) defines an experiment template variable and optional check and PR filters. Main and PR panels remain separate. A PR filter applies only to PR panels.

The all-evaluations view uses CI Tests and retains reruns. Show separate bounds and optimization rates, daily outcome counts, a check table, and diagnostic errors. The pass rate is `100 * pass / (pass + fail)`. Errors remain visible beside the rate. A zero denominator means no data.

The latest-per-PR view first selects a whole experiment run. It orders parent pipeline ID, CI job ID, analysis end time, and SMP job ID in descending order. It then retains every check in that run. This clears an old diagnostic when a newer run succeeds. It also preserves missing-check errors from the newest run.

The [DDSQL query](../../test/regression/reporting/latest-pr.sql) implements this reduction over test events. The offline summary implements the same ordering. Tests compare the SQL reduction with independent fixtures. Platform publication and template-variable filters still need a live pilot. The design file is not a Datadog API payload.

Latest means latest reported within the selected time window, per experiment. It does not mean current PR head, or that every check passed in a PR. Keep source commit and run links visible. Main can have no results for experiments outside the manifest's `always` group.

## Impact review

- Existing performance gates and the legacy upload keep their current behavior.
- The new container step runs one local selection and one local render. It also reads selected experiment definitions.
- The step retains source JSON, normalized JSON, XML, and summaries for the existing artifact retention period.
- A render failure creates a visible status artifact and warning. It does not change the gate result.
- This change sends no new test events and publishes no dashboards or monitors. Standard Invoke task logging remains active.
- Later ingestion adds one event per bounds check and configured goal, plus diagnostic rows. Review this volume before activation.
- A separate service protects service-filtered consumers. Unfiltered cross-service totals can still change after activation.

The platform pilot must test ingestion, attribute types, native error status, test identity, both views, and empty selections. Verify CI Tests dataset access and saved-query support before dashboard publication. Do not infer these platform behaviors from local XML validation.

The SQL source follows the [CI Tests dataset](https://docs.datadoghq.com/ddsql_reference/data_directory/dd/dd.ci_tests.dataset/) and [DDSQL window functions](https://docs.datadoghq.com/ddsql_reference/).
