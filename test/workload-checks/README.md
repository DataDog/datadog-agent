# Workload Checks

The Single Machine Performance 'Workload Checks' tool performs a nightly run of
the Agent and compares that run to a series of 'checks' to determine if the
Agent is fit for purpose: did we consume too much memory, did we achieve our
throughput goals etc. This is done relative to a 'machine class', with the
intention of simulating whether we are fit-for-purpose on a given type of
machine, for a given type of workload that our customers would run.

## Adding an Experiment

In order for SMP's tooling to properly read a experiment directory please
adhere to the following structure. Starting at the root:

* `MACHINE_CLASS/` -- __Required__ The directory that contains experiments for
  each class of machine. Please note that `MACHINE_CLASS` metasyntactic
  variable. For instance, we have `typical` for a "typical" machine class.

The structure of each machine class is as follows:

* `machine.yaml` -- __Required__ The definition of the machine class, lays out
  resource limitations.
* `cases/` -- __Required__ The directory that contains each workload check
   experiment for the parent machine class.  Each sub-directory is a separate
   experiment and the name of the directory is the name of the experiment, for
   instance `tcp_syslog_to_blackhole`. We call these sub-directories 'cases'.

The structure of each case is as follows:

* `lading/lading.yaml` -- __Required__ The [lading] configuration inside its own
  directory. Directory will be mount read-only in the container built from
  `Dockerfile` above at `/etc/lading`.
* `datadog-agent/` -- __Required__ This is the configuration directory of your
  program. Will be mounted read-only in the container build from `Dockerfile`
  above at `/etc/datadog-agent`.
* `experiment.yaml` -- __Optional__ This file can be used to set a
  single optimization goal for each experiment.
