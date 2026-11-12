# Inspect SMP experiment health reports

Use this guide to inspect experiment health reports in CI and Datadog. It applies to the container experiment lane. Metal and eBPF experiments keep their existing reports.

The report uses service `datadog-agent-smp-experiment-health`. CI generates artifacts automatically. A separate job uploads the new XML for resolved PR runs and main. Existing uploads and CI gates remain active.

## Inspect a CI run

Download these artifacts from the SMP child job:

- `outputs/report.v1.json`: source analysis results.
- `outputs/report.json`: comparison replica maxima used by the peak summary.
- `outputs/experiment-health/selected-experiments.json`: selected experiment inventory.
- `outputs/experiment-health/context.json`: branch, PR, and CI identifiers.
- `outputs/experiment-health/pr-context.json`: resolved, absent, or unavailable PR context.
- `outputs/experiment-health/selection-inputs.json`: selection labels and involved teams.
- `outputs/experiment-health/evaluations.json`: normalized checks and their source details.
- `outputs/experiment-health/experiment-health.junit.xml`: rendered JUnit report.
- `outputs/experiment-health/summary.json`: all evaluations and latest results per PR.
- `outputs/experiment-health/render-status.json`: render success or failure.
- `outputs/experiment-health/upload-plan.json`: local render plan and separate service name. The local renderer never uploads.

Inspect `render-status.json` first. A renderer error does not change the performance gate result. A failure before result download can prevent artifact generation entirely.

Trace each XML testcase through its `smp.experiment`, `smp.check_kind`, and `smp.check_name` properties. Compare its outcome with the corresponding source result. An advisory regression must remain a failure in the new report.

Missing checks must appear as errors. A check with zero comparison replicas must also appear as an error. Keep these errors separate from performance failures.

For memory bounds, compare the configured limit with the lowest and highest replicate peaks. Calculate each replica's peak first, then take the minimum and maximum of those peaks. Do not use the lowest sample across all replicas as the lowest replicate peak. An incomplete peak summary must omit its values.

## Render a downloaded report

Use an Agent checkout from the same commit as the original run. Install SMP v0.29.1. Keep the original selection labels, involved teams, and PR context when available.

```shell
dda inv smp.experiment-health \
  --smp /path/to/smp \
  --report /path/to/report.v1.json \
  --replicate-report /path/to/report.json \
  --pr-context /path/to/pr-context.json \
  --output /path/to/health-artifacts
```

Both source reports must come from the same SMP job. Without `--replicate-report`, the renderer reads `report.json` beside the version 1 source report.

The command reads CI identifiers from the environment. Set `CI_COMMIT_BRANCH`, `CI_DEFAULT_BRANCH`, `PARENT_PIPELINE_ID`, `CI_JOB_ID`, and `CI_JOB_URL` to reproduce the original context. Set `SMP_LABELS` and `SMP_INVOLVED` to reproduce experiment selection. The task calls only local SMP selection and rendering commands.

Without resolved PR context, the summary classifies a development branch as `other`. It does not invent a PR identity. Without CI identifiers, the report uses zero placeholders for local review. Do not publish those local review results.

## Inspect results in Datadog

`single-machine-performance-experiment-health-upload` runs automatically after the container job, including when its quality gate fails. It uploads only the new report, under service `datadog-agent-smp-experiment-health` and environment `ci`. It skips branches without resolved PR context.

The job disables report-log forwarding and has no automatic retries. An upload failure does not change the performance gate result. A separate service does not exclude the data from monitors that query all services. Repeated uploads can add duplicate test events.

In Test Optimization Explorer, use this query:

```text
service:datadog-agent-smp-experiment-health env:ci @smp.report_kind:experiment_health @smp.report_version:2
```

Group by `smp.experiment`, `smp.check_kind`, and `smp.outcome`. Inspect a passing check, a failing check, and a diagnostic error when the source run contains them. Use `smp.upload_job_id` to isolate one upload.


## Compare several runs

```shell
dda inv smp.summarize-health \
  --input /path/to/first/evaluations.json \
  --input /path/to/second/evaluations.json \
  --experiment quality_gate_idle \
  --output /path/to/summary.json
```

`all_evaluations` retains reruns. `latest_per_pr` retains only the newest reported run for each PR and experiment. Both views contain check rows and separate pass, fail, and error counts. Identical evaluation IDs count once. Conflicting duplicates stop the summary.

Latest reported results can belong to an older PR commit. Always inspect `smp.comparison_sha`. An experiment absent from a newer run does not prove that its previous result applies to the current PR head.

## Verify a new CI run

Use one PR run and one main run for artifact review. Compare check counts, advisory failures, missing results, and PR identity. Verify that the new step leaves existing gate outcomes unchanged.

The [report contract](../../reference/smp-experiment-health.md) defines both dashboard views and their impact checks. Verify numeric attribute types, error mapping, query filters, and event volume in Datadog.
