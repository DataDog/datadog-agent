# Regression Detection

The Regression Detector, owned by Single Machine Performance, is a tool that
detects if there are more-than-random performance changes to a target program --
here, the Agent -- across a variety of experiments and goals. This directory
contains the experiments for Agent. A similar one exists in [Vector]. Please do
add your own experiments, instructions below. If you have any questions do
contact #single-machine-performance; we'll be glad to help.

## Experiment selection

Experiments run on two lanes: **container** experiments live under `container/`
(manifest-driven, described here) and the **metal** ebpf experiments live under
`ebpf/` (run separately on the old SMP path). A container experiment is any
directory containing an `experiment.yaml`, discovered by a recursive walk of
`container/` at any depth (e.g. `container/quality_gates/quality_gate_idle/`,
`container/metrics/dsd_uds_…/`). Which experiments run on a given PR is governed by
a single central manifest, `container/selection.yaml`, which maps
**trigger buckets → experiments** (by glob, exact path, or experiment name):

* `always` — a flat list; runs on **every PR**, unconditionally. This is PR
  selection, not the nightly SMP schedule (which runs only the quality gates).
  Includes the core performance
  [quality gates](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4294836779/Performance+Quality+Gates).
* `codeowners` — a `team → experiments` map; a team's experiments run
  automatically when that team has a file changed on the PR (involvement is
  computed from changed files ∩ `.github/CODEOWNERS`).
* `labels` — a `smp/<label> → experiments` map; runs when the label is applied
  to the PR.

The buckets are **unioned**: an experiment runs if any of its triggers fire, and
an experiment may appear in several buckets. Every experiment must be in at least
one bucket to merge.

For step-by-step recipes (adding a label suite or an ownership-driven suite),
see [`experiment-selection-guide.md`](experiment-selection-guide.md).

## Adding an Experiment

In order for SMP's tooling to properly read the suite please adhere to the
following structure. Container experiments live under `container/`:

* `container/config.yaml` -- __Required__ Configuration that applies to all
  experiments in the lane.
* One directory per experiment, nested at any depth for organization — the
  directory name is the experiment name, and any directory containing an
  `experiment.yaml` is discovered as an experiment. Experiment names must be
  unique across the lane (compared case-insensitively). Intermediate directories
  are just grouping; a directory with no `experiment.yaml` is ignored, not an
  error.
  * `README.md` -- __Optional__ Prose docs for the folder (a one-line
    `description` plus any notes). No selection front-matter — selection lives
    in `selection.yaml`.
* Assign the new experiment to a bucket in `container/selection.yaml` (or ensure it
  falls under an existing glob such as `logs/**`). The lint gate (`smp
  experiments resolve`) blocks merge until every experiment is bucketed.

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
smp local-run --experiment-dir ~/dev/datadog-agent/test/regression/container/ --case quality_gate_logs --target-image datadog/agent-dev:nightly-main-fe13dead-py3
```
