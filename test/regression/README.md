# Regression Detection

The Regression Detector finds Agent performance changes with controlled experiments. The Single Machine Performance team owns this system.

## Common tasks

* [Add an experiment and choose when it runs](experiment-selection-guide.md)
* Install the SMP CLI with `dogbrew install smp`, then [run an experiment locally][smp-local]
* [Understand the core Agent performance quality gates](container/quality_gates/README.md)
* Ask `#single-machine-performance` for help

## How Agent CI selects experiments

Most experiments are under `container/` and use `container/selection.yaml`. Metal eBPF experiments use the separate `ebpf/` lane.

The selection manifest supports three modes:

* The `always` bucket runs on every pull request and every `main` commit. This selection does not control the nightly schedule.
* The `codeowners` bucket maps a team slug to experiments. On pull requests, the experiments run when the team owns a changed file.
* The `labels` bucket maps an exact label to experiments. On pull requests, the experiments run when the pull request has that label.

On `main`, Agent CI selects only the `always` bucket. Label and CODEOWNERS selection needs pull request context.

The [experiment selection guide](experiment-selection-guide.md) covers experiment structure, manifest syntax, optimization goals, and verification.

[smp-local]: https://github.com/ddoghq/single-machine-performance/blob/main/docs/guides/local-run.md

## CI Reports

The SMP backend computes analysis data. After `smp job sync`, CI uses
`report.v1.json` for local rendering and policy decisions.

`test/regression/smp_ci.py` computes the CI pass/fail decisions
and invokes `smp report render` with `report_template.md.j2`.
Report links are selected in `smp_ci.py`; their URLs are defined in
`report_template.md.j2`. The legacy `report` and `report_links` YAML sections
are not used for local rendering.

The CI job (`.gitlab/childs/smp-regression-child-pipeline.yml`) produces:

| output | purpose |
|---|---|
| `outputs/report.v1.json` | server artifact, used for all local reporting |
| `outputs/report.md` | rendered Markdown, including the Agent CI decision |
| `outputs/decision.json` | three `passed` / `failed` values for tags and job control |
| `outputs/pr_report_comment.json` | complete PR-comment JSON payload; its message equals the Markdown exactly |
| `outputs/smp-failed-flag` | zero-byte marker when `decision.job` is `failed`; not uploaded |
| `outputs/junit.xml` | direct SMP built-in render, uploaded with `datadog-ci junit upload` |
