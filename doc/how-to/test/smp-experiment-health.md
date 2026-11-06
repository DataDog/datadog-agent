# Inspect SMP experiment health reports

Use this guide to inspect the proposed experiment health report before platform ingestion. It applies to the container experiment lane. Metal and eBPF experiments keep their existing reports.

The report uses service `datadog-agent-smp-experiment-health`. This change generates local artifacts only. It adds no Datadog upload or dashboard publication. Existing uploads and CI gates remain active.

## Inspect a CI run

Download these artifacts from the SMP child job:

- `outputs/report.v1.json`: source analysis results.
- `outputs/experiment-health/selected-experiments.json`: selected experiment inventory.
- `outputs/experiment-health/context.json`: branch, PR, and CI identifiers.
- `outputs/experiment-health/pr-context.json`: resolved, absent, or unavailable PR context.
- `outputs/experiment-health/selection-inputs.json`: selection labels and involved teams.
- `outputs/experiment-health/evaluations.json`: normalized checks and their source details.
- `outputs/experiment-health/experiment-health.junit.xml`: rendered JUnit report.
- `outputs/experiment-health/summary.json`: all evaluations and latest results per PR.
- `outputs/experiment-health/render-status.json`: render success or failure.
- `outputs/experiment-health/upload-plan.json`: disabled upload plan and separate service name.

Inspect `render-status.json` first. A renderer error does not change the performance gate result. A failure before result download can prevent artifact generation entirely.

Trace each XML testcase through its `smp.experiment`, `smp.check_kind`, and `smp.check_name` properties. Compare its outcome with the corresponding source result. An advisory regression must remain a failure in the new report.

Missing checks must appear as errors. A check with zero comparison replicas must also appear as an error. Keep these errors separate from performance failures.

## Render a downloaded report

Use an Agent checkout from the same commit as the original run. Install SMP v0.29.1. Keep the original selection labels, involved teams, and PR context when available.

```shell
dda inv smp.experiment-health \
  --smp /path/to/smp \
  --report /path/to/report.v1.json \
  --pr-context /path/to/pr-context.json \
  --output /path/to/health-artifacts
```

The command reads CI identifiers from the environment. Set `CI_COMMIT_BRANCH`, `CI_DEFAULT_BRANCH`, `PARENT_PIPELINE_ID`, `CI_JOB_ID`, and `CI_JOB_URL` to reproduce the original context. Set `SMP_LABELS` and `SMP_INVOLVED` to reproduce experiment selection. The task calls only local SMP selection and rendering commands.

Without resolved PR context, the summary classifies a development branch as `other`. It does not invent a PR identity. Without CI identifiers, the report uses zero placeholders for local review. Do not publish those local review results.

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

## Review before platform activation

Use one PR run and one main run for artifact review. Compare check counts, advisory failures, missing results, and PR identity. Verify that the new step leaves existing gate outcomes unchanged.

The [report contract](../../reference/smp-experiment-health.md) defines both dashboard views and their impact checks. A later change must explicitly enable uploads and publish the dashboard. That pilot must verify numeric attribute types, error mapping, service isolation, query filters, and event volume in Datadog.
