# Regression Detection

The Regression Detector, owned by Single Machine Performance, is a tool that
detects if there are more-than-random performance changes to a target program --
here, the Agent -- across a variety of experiments and goals. This directory
contains the experiments for Agent. A similar one exists in [Vector]. Please do
add your own experiments, instructions below. If you have any questions do
contact #single-machine-performance; we'll be glad to help.

## Quality Gate Experiments
Experiments prefixed with `quality_gate_` represent the strongest claims made
about the Agent and its performance. These are discussed in more detail on
[this
page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4294836779/Performance+Quality+Gates)

## Adding an Experiment

In order for SMP's tooling to properly read a experiment directory please
adhere to the following structure. Starting at the root:

* `config.yaml` -- __Required__ Configuration that applies to all experiments.
* `cases/` -- __Required__ The directory that contains each experiment.
  Each sub-directory is a separate experiment and the name of the
  directory is the name of the experiment, for instance
  `tcp_syslog_to_blackhole`. We call these sub-directories 'cases'.

The structure of each case is as follows:

* `lading/lading.yaml` -- __Required__ The [lading] configuration inside its own
  directory.
* `datadog-agent/` -- __Required__ This is the configuration directory of your
  program. Will be mounted read-only in the container build from `Dockerfile`
  above at `/etc/datadog-agent`.
* `experiment.yaml` -- __Required__ Set any experiment-specific configuration.
  The "optimization goal" determines what metric the Regression Detector
  will analyze at the conclusion of the experiment.

  Eg:
  ```yaml
  optimization_goal: ingress_throughput
  ```

  Supported values of `optimization_goal` are `ingress_throughput` and
  `egress_throughput`.

[Vector]: https://github.com/vectordotdev/vector/tree/master/regression
[lading]: https://github.com/DataDog/lading

## Reports

The SMP backend computes analysis data. After `smp job sync`, CI uses
`report.v1.json` for local rendering and policy decisions. When the backend
only supplies legacy `report.json`, `convert_old_report_to_v1.py` converts it
first. The wrapper assumes the v1 input conforms to its report schema; it does
not duplicate schema validation.

`test/regression/smp_ci.py` computes the three decisions, builds Agent links,
and invokes `smp report render` with `report_template.md.j2`. The Markdown keeps
the parity-report layout and the **CI Pass/Fail Decision** section used by the
existing PR comment. All teams use the Agent link configuration for now.

The CI job (`.gitlab/childs/smp-regression-child-pipeline.yml`) produces:

| output | purpose |
|---|---|
| `outputs/report.v1.json` | native or converted source for all local reporting |
| `outputs/report.md` | rendered Markdown, including the Agent CI decision |
| `outputs/decision.json` | three `passed` / `failed` values for tags and job control |
| `outputs/pr_report_comment.json` | complete PR-comment JSON payload; its message equals the Markdown exactly |
| `outputs/smp-failed-flag` | zero-byte marker when `decision.job` is `failed`; not uploaded |
| `outputs/junit.xml` | direct SMP built-in render, uploaded with `datadog-ci junit upload` |

`outputs/extra.json` is the wrapper's template context, not a policy artifact.
The comment job sends the generated JSON payload directly with
`curl --data-binary`; no shell escaping of Markdown is needed.

### Who decides pass/fail

`smp_ci.py` computes these fields once from the v1 input:

| field | policy | Datadog job tag |
|---|---|---|
| `regressions` | fail on missing optimization analysis or a significant worsening in a non-erratic experiment | `smp_optimization_goal` |
| `all_bounds_checks` | fail when any comparison replicate fails a bounds check in a non-erratic experiment | `smp_bounds_check` |
| `job` | fail when a `quality_gate_*` bounds check has no comparison data or any comparison replicate fails | `smp_quality_gates` |

Configured `erratic: true` is not an exemption from an Agent quality gate.
Runtime optimization-result `is_erratic` does not change the backend-signal
predicates. Regressions and non-gated bounds failures are reported and tagged,
but do not themselves fail the Agent job. A suite with no quality gates passes.

The wrapper returns **zero for both valid policy outcomes**. Input, rendering,
or output-write errors return nonzero and stop CI immediately. Outputs are
written directly in the fresh CI workspace; there is no atomic-publication or
stale-output cleanup layer. CI checks decision values and marker consistency,
uploads JUnit, sets tags, and finally exits 1 if the failure marker exists.
The comment job runs even after a policy failure, but requires the locally
generated payload rather than a downloaded server report.

### Legacy conversion limits

Legacy JSON does not carry metric series names, quantile results, or analysis
errors. The converter uses check names as series placeholders, guesses units
(with a `memory_usage` override), and emits empty quantile/error lists. These
fields do not drive the three decisions. Experiments absent from legacy JSON
cannot be recovered. Empty legacy bounds checks that are present are retained
with zero counts so missing quality-gate data cannot turn into a pass.

### Iterating locally

Rendering and decisions are offline: no AWS credentials or submitted SMP job
are needed. Use an SMP CLI with v1 `report render` support and a saved report.
From the repository root:

```sh
mkdir -p /tmp/smp-report

# Skip this conversion step when you already have report.v1.json.
python3 test/regression/convert_old_report_to_v1.py report.json \
  --output /tmp/smp-report/report.v1.json --force

python3 test/regression/smp_ci.py \
  --smp /path/to/smp \
  --report /tmp/smp-report/report.v1.json \
  --commit "<comparison-commit-sha>" \
  --template test/regression/report_template.md.j2 \
  --output-report-md /tmp/smp-report/report.md \
  --output-decision /tmp/smp-report/decision.json \
  --output-pr-comment /tmp/smp-report/pr_report_comment.json \
  --output-failure-mark /tmp/smp-report/smp-failed-flag

/path/to/smp report render \
  --report /tmp/smp-report/report.v1.json \
  --builtin junit.xml > /tmp/smp-report/junit.xml
```

To iterate on the template after generating its context:

```sh
/path/to/smp report render \
  --report /tmp/smp-report/report.v1.json \
  --template-file test/regression/report_template.md.j2 \
  --extra /tmp/smp-report/extra.json > /tmp/smp-report/report.md
```

The template expects both `report` and `extra`, including the Agent links and
CI decision section. It does not read experiment configuration directories.
Run the focused Python tests without SMP or cloud access:

```sh
python3 -m unittest discover -s test/regression -p 'test_smp_ci.py'
```

## Local Run
In order to run a regression experiment locally, you need two CLI utilities
available:
- `smp` -- build from source [repo](https://github.com/DataDog/single-machine-performance/)
- `lading` -- See the notes in the below documentation about architecture,
  `lading` needs to be compatible with the architecture of the image being run.

See full docs [here](https://github.com/DataDog/single-machine-performance/blob/main/smp/README.md#running-replicates-locally)

An example command may look like this:
```
smp local-run --experiment-dir ~/dev/datadog-agent/test/regression/ --case uds_dogstatsd_to_api --target-image datadog/agent-dev:nightly-main-fe13dead-py3
```
