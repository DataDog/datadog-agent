# Regression Detection

The Regression Detector, owned by Single Machine Performance, is a tool that
detects if there are more-than-random performance changes to a target program --
here, the Agent -- across a variety of experiments and goals. This directory
contains the experiments for Agent. A similar one exists in [Vector]. Please do
add your own experiments, instructions below. If you have any questions do
contact #single-machine-performance; we'll be glad to help.

## Experiment selection

An experiment is a directory under any `cases/` directory, at any depth (e.g.
`quality_gates/cases/…`, `logs/general/cases/…`). Which experiments run on a
given PR is governed by a single central manifest, `selection.yaml`, which maps
**trigger buckets → experiments** (by glob, exact path, or experiment name):

* `always` — runs unconditionally, on every PR and in scheduled SMP runs. These
  are the strongest claims made about the Agent's performance (see
  [Performance Quality Gates](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4294836779/Performance+Quality+Gates)).
* `codeowners` — runs automatically on a PR when the experiment's owning team
  (per `.github/CODEOWNERS`) has changed a file.
* `labels` — runs when the named `smp/<label>` is applied to the PR.

The buckets are **unioned**: an experiment runs if any of its triggers fire, and
an experiment may appear in several buckets. **To run a labelled suite on a PR,
apply its `smp/<label>` label.** `selection.yaml` is also the label registry —
see it for the available labels.

For step-by-step recipes (adding a label suite, an ownership-driven suite, and
the CODEOWNERS delegation that ownership-driven suites require), see
[`experiment-selection-guide.md`](experiment-selection-guide.md).

## Adding an Experiment

In order for SMP's tooling to properly read the suite please adhere to the
following structure. Starting at the root:

* `config.yaml` -- __Required__ Configuration that applies to all experiments.
* One or more directories that (at any depth) contain a `cases/` directory:
  * `cases/` -- The directory that contains each experiment. Each sub-directory
    is a separate experiment and the name of the directory is the name of the
    experiment. Experiment names must be unique across the whole suite. We call
    these sub-directories 'cases'.
  * `README.md` -- __Optional__ Prose docs for the folder (a one-line
    `description` plus any notes). No selection front-matter — selection lives
    in `selection.yaml`.
* Assign the new experiment to a bucket in `selection.yaml` (or ensure it falls
  under an existing glob such as `logs/**`). The validation gate
  (`smp experiments validate --in-ci`) blocks merge until every experiment is
  bucketed.

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
