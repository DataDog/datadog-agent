# KJob jobs

This directory holds KJob jobs for Agent CI. KJob is a job type in SMP that runs Kubernetes performance jobs on EKS or KinD clusters. A KJob job checks absolute bounds and does not compare against a baseline image.

These jobs are not part of the regression detector. `container/selection.yaml` does not select them, and `smp job submit-regression` does not read this directory.

Each subdirectory is one KJob job directory. It contains one or more cases under `cases/<name>/`. Each case has an `experiment.yaml` file and its target manifests, and it can have Helm values files.

The `single_machine_performance-kjob` job in `.gitlab/test/functional_test/kjob.yml` submits `operator-helm-smoketest` to the `agent-beta` SMP team. Then `single_machine_performance-kjob-pr-comment` posts the result on the PR, with the header `K8s Regression Detector Results`. `smp kjob report` renders the comment with `report_template.md.j2`.

See the [KJob user guide](https://github.com/ddoghq/single-machine-performance/blob/paul.reinlein/krjo-poc/docs/kjob/user-guide.md) for the case format.
