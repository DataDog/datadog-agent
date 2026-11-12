# SMP experiment health report contract

This contract supports experiment owners who need evidence before they promote an advisory experiment to a quality gate. It preserves observed failures independently of CI policy. The first version covers container experiments only.

## Isolation and activation

The service is `datadog-agent-smp-experiment-health`. Every testcase has `smp.report_kind:experiment_health` and `smp.report_version:2`. Suites use `smp.health.<experiment>`. Names remain stable across branches, commits, and reruns.

The container job generates artifacts. A separate job uploads the new XML automatically for resolved PR runs and main, including when the source job fails. It does not add the XML to GitLab's `reports:junit` section. The existing explicit `outputs/junit.xml` upload remains unchanged.

The upload uses the new service, environment `ci`. It has no automatic retries and cannot fail the performance gate. Branches without resolved PR context do not upload.

## Outcomes

| Kind | Pass | Failure | Error |
| --- | --- | --- | --- |
| Bounds | All comparison replicas pass, with positive total | At least one replica fails | Missing result, zero replicas, or invalid counts |
| Optimization goal | No significant degradation | Significant change without improvement | Configured goal has no usable result |
| Analysis | No synthetic pass row | No performance failure row | Analysis error or statistical instability |
| Execution | No synthetic pass row | No performance failure row | Failed replica groups or absent selected experiment |

An ignored regression remains a failure. `smp.ignores_regressions` records the experiment policy. `smp.statistically_erratic` records the analysis flag. Statistical instability also creates an analysis error row. Recovered replica failures remain visible with their original `is_dead` value.

The inventory comes from the same selection manifest and inputs as submission. The renderer reads expected checks from the selected experiment definitions. It emits errors for missing experiments and checks. Bounds-only experiments have no synthetic optimization goal.

Bounds attributes describe comparison replicas only. Numeric properties remain strings in JUnit. The report preserves replica counts, limits, observed ranges, percent change, and confidence intervals. For a valid bounds result, failed replicas equal `smp.bounds.total_count` minus `smp.bounds.pass_count`. Display the total beside the failed count. Invalid or absent counts mean an error, not zero failures.

## Memory bounds statistics

For each comparison replica, take its maximum memory value. The dashboard shows the range of those peaks beside the configured limit:

| Label | Attribute | Calculation |
| --- | --- | --- |
| Configured limit | `smp.bounds.upper_bound` | Limit from the experiment definition used by the run |
| Lowest replicate peak | `smp.bounds.lowest_replicate_peak` | Minimum of the comparison replicas' maxima |
| Highest replicate peak | `smp.bounds.highest_replicate_peak` | Maximum of the comparison replicas' maxima |

The renderer reads replica maxima from `report.json`, alongside `report.v1.json`. Both files must identify the same SMP job. Replica counts and outcomes must match. The highest replica peak must match the version 1 source report's observed maximum. An inconsistent source stops rendering and leaves the CI gate unchanged.

`smp.bounds.peak_summary_status` is `complete` when every comparison replica has a maximum. Otherwise, it is `unavailable`, and the report omits both peak statistics. Missing values do not become zero. The original pass, failure, or error outcome remains visible.

The existing `min_observed` and `max_observed` attributes retain the overall sample range. Do not label the sample minimum as the lowest replicate peak. Version 2 dashboard queries must use the new peak attributes without a fallback to version 1 sample minima. Convert byte values to MiB for display.

For other checks, inspect the series expression before choosing labels. A terminal expression such as `avg(...)` produces a final value per replica. Its replica range describes those final values.

## Identity and attribution

Each evaluation ID hashes the CI job ID, SMP job ID, experiment, check kind, and check name. A CI retry has a distinct identity. A repeated copy of one artifact retains the same identity.

PR context uses the existing branch lookup. Exactly one match supplies the PR number and URL. Absent, ambiguous, and failed lookups remain visible and use context `other`. Main uses context `main`. A branch name alone does not identify a PR.

Context includes the actual baseline and comparison SHAs, parent pipeline ID, child job ID, job URL, and analysis end time. The context builder copies an explicit list of fields. It does not copy tokens or the full environment.

## Dashboard views

The local [dashboard design](../../test/regression/reporting/dashboard-design.json) defines an experiment template variable and optional check and PR filters. Main and PR panels remain separate. A PR filter applies only to PR panels.

The all-evaluations view uses CI Tests and retains reruns. Show separate bounds and optimization rates, daily outcome counts, a check table, and diagnostic errors. The pass rate is `100 * pass / (pass + fail)`. Errors remain visible beside the rate. A zero denominator means no data.

The latest-per-PR view first selects a whole experiment run. It orders parent pipeline ID, CI job ID, analysis end time, and SMP job ID in descending order. It then retains every check in that run. This clears an old diagnostic when a newer run succeeds. It also preserves missing-check errors from the newest run.

The [DDSQL query](../../test/regression/reporting/latest-pr.sql) implements this reduction over test events. The offline summary implements the same ordering. Tests compare the SQL reduction with independent fixtures. Live validation verified ingestion and both views. Verify template-variable bindings for each saved widget. The design file is not a Datadog API payload.

Latest means latest reported within the selected time window, per experiment. It does not mean current PR head, or that every check passed in a PR. Keep source commit and run links visible. Main can have no results for experiments outside the manifest's `always` group.

## Impact review

- Existing performance gates and the legacy upload keep their current behavior.
- The new container step runs one local selection and one local render. It also reads selected experiment definitions.
- The step retains source JSON, normalized JSON, XML, and summaries for the existing artifact retention period.
- A render failure creates a visible status artifact and warning. It does not change the gate result.
- Each automatic upload sends new test events. Standard Invoke task logging remains active.
- Each upload adds one event per bounds check and configured goal, plus diagnostic rows. Repeated uploads can distort event counts.
- A separate service protects service-filtered consumers. Unfiltered cross-service totals and monitors can include the new events.

When the report contract changes, verify ingestion, attribute types, native error status, test identity, both views, and empty selections. Verify CI Tests dataset access and saved-query support during dashboard setup. Do not infer these platform behaviors from local XML validation.

The SQL source follows the [CI Tests dataset](https://docs.datadoghq.com/ddsql_reference/data_directory/dd/dd.ci_tests.dataset/) and [DDSQL window functions](https://docs.datadoghq.com/ddsql_reference/).
