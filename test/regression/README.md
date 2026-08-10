# Regression Detection

The Regression Detector, owned by Single Machine Performance, is a tool that
detects if there are more-than-random performance changes to a target program --
here, the Agent -- across a variety of experiments and goals. This directory
contains the experiments for Agent. A similar one exists in [Vector]. Please do
add your own experiments, instructions below. If you have any questions do
contact #single-machine-performance; we'll be glad to help.

## Experiment groups and selection

Experiments are organized into **groups**. A group is either:

* a **leaf** — a directory that directly contains a `cases/` directory of
  experiments, plus a `README.md` declaring how the group runs; or
* a **dir** — an organizational directory that only contains other groups (no
  `cases/` of its own, e.g. `logs/`).

Each leaf's `README.md` front-matter declares a `mode` that controls when the
group runs:

* `quality_gates` — always runs, on every PR and in scheduled SMP runs; not
  user-selectable. These are the strongest claims made about the Agent's
  performance (see [Performance Quality Gates](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4294836779/Performance+Quality+Gates)).
* `codeowners` — runs automatically on a PR when the group's owning team (per
  `.github/CODEOWNERS`) has changed a file. It can also be run on any PR by
  applying the group's label (below).
* `optional` — runs only when the group's label is applied to the PR.

**To run a `codeowners` or `optional` group on a PR, add its label.** A group's
label is declared in its `README.md` front-matter and always mirrors the group
path: `smp/<group-path>` (e.g. `smp/logs/syslog`). Quality-gate groups have no
label — they always run.

## Adding an Experiment

In order for SMP's tooling to properly read the suite please adhere to the
following structure. Starting at the root:

* `config.yaml` -- __Required__ Configuration that applies to all experiments.
* One or more **groups**, each a directory containing:
  * `README.md` -- __Required for leaves__ Front-matter with `mode`
    (`quality_gates` | `codeowners` | `optional`), a `label` of
    `smp/<group-path>` (required for `codeowners`/`optional`; omit for
    `quality_gates`), and a one-line `description`.
  * `cases/` -- __Required for leaves__ The directory that contains each
    experiment. Each sub-directory is a separate experiment and the name of the
    directory is the name of the experiment. Experiment names must be unique
    across the whole suite. We call these sub-directories 'cases'.

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

## Local Run
In order to run a regression experiment locally, you need two CLI utilities
available:
- `smp` -- build from source [repo](https://github.com/DataDog/single-machine-performance/)
- `lading` -- See the notes in the below documentation about architecture,
  `lading` needs to be compatible with the architecture of the image being run.

See full docs [here](https://github.com/DataDog/single-machine-performance/blob/main/smp/README.md#running-replicates-locally)

An example command may look like this:
```
smp local-run --experiment-dir ~/dev/datadog-agent/test/regression/ --case quality_gate_logs --target-image datadog/agent-dev:nightly-main-fe13dead-py3
```
