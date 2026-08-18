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

The PR comment is rendered **client-side** by the `smp` CLI from the job's
`report.json`, using the minijinja template `report_template.md.j2` in this
directory. Nothing about the report is computed by CI or by the SMP backend, so
the template can be changed in a PR and takes effect on that PR's own run.

The CI job (`.gitlab/childs/smp-regression-child-pipeline.yml`) renders three
views after `smp job sync`:

| output | template | consumed by |
|---|---|---|
| `outputs/report.md` | `report_template.md.j2` (this directory) | the PR comment |
| `outputs/severity_report.md` | SMP built-in | job artifact, debugging |
| `outputs/junit.xml` | SMP built-in | `datadog-ci junit upload` |

`report_template.md.j2` deliberately collapses everything that passed behind
`<details>`, so a clean run is a handful of lines and a reviewer only sees what
needs action. It also renders the **CI Pass/Fail Decision** section; the job's
exit code itself comes from `smp job result --signal bounds-check`.

### Iterating on the template locally

Rendering is offline, so no AWS credentials or SMP job are needed -- only a
`report.json`. Take one from a recent job's artifacts, then:

```sh
smp report render \
  --template-file test/regression/report_template.md.j2 \
  --report-json report.json \
  --target-config-dir test/regression \
  --output-file /tmp/report.md
```

Useful companions:

- `smp report list` -- the built-in template names.
- `smp report export report.md --output-file /tmp/builtin.md.j2` -- the built-in
  source, handy as a diff base or to copy a table idiom.
- Omitting `--target-config-dir` renders with no links, which is what the
  config-only job does; worth checking a template still renders that way.

The render context (`optimization_goals`, `checks`, `missing_data`, `errors`,
`failed_replicates`, `config`, `experiments`, and the `interpolate`/`table`/`fmt2`
filters) is documented in SMP's `docs/guides/reports.md`.

### Two gotchas

- **`--target-config-dir` parsing is strict.** `config.yaml` and every
  `experiment.yaml` the report references are parsed with unknown fields
  rejected. A field that the pinned `SMP_VERSION` does not know about fails the
  render rather than being ignored. Good for catching typos, but it means adding
  a new experiment field can require bumping `SMP_VERSION` in the child pipeline.
- **Config-only suites have no `experiment.yaml`.** `ebpf/config-only/` uses
  `config-only-experiment.yaml`, which `--target-config-dir` cannot read, so that
  job renders without a config dir and its reports carry no links.

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
