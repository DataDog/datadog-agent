# Regression Detection

The Regression Detector, owned by Single Machine Performance, is a tool that
detects if there are more-than-random performance changes to a target program --
here, the Agent -- across a variety of experiments and goals. This directory
contains the experiments for Agent. A similar one exists in [Vector]. Please do
add your own experiments, instructions below. If you have any questions do
contact #single-machine-performance; we'll be glad to help.

## Adding an Experiment

In order for SMP's tooling to properly read a experiment directory please
adhere to the following structure. Starting at the root:

* `cases/` -- __Required__ The directory that contains each regression
  experiment. Each sub-directory is a separate experiment and the name of the
  directory is the name of the experiment, for instance
  `tcp_syslog_to_blackhole`. We call these sub-directories 'cases'.

The structure of each case is as follows:

* `lading/lading.yaml` -- __Required__ The [lading] configuration inside its own
  directory. Directory will be mount read-only in the container built from
  `Dockerfile` above at `/etc/lading`.
* `datadog-agent/` -- __Required__ This is the configuration directory of your
  program. Will be mounted read-only in the container build from `Dockerfile`
  above at `/etc/datadog-agent`.
* `experiment.yaml` -- __Optional__ This file can be used to set a
  single optimization goal for each experiment.

  In this file, the optimization goal is set via a snippet like

  ```yaml
  optimization_goal: ingress_throughput
  ```

  Supported values of `optimization_goal` are `ingress_throughput` and
  `egress_throughput`.

  If an experiment omits this file, the optimization goal defaults to
  `ingress_throughput`.

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
smp local-run --experiment-dir ~/dev/datadog-agent/test/regression/ --case uds_to_blackhole --target-image datadog/agent-dev:nightly-main-1bf80594-py3 --lading-path ~/lading/target/release/lading --target-command "/bin/entrypoint.sh" --target datadog-agent
```
