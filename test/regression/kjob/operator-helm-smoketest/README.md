# Operator Helm smoketest

This fixture is the smallest Operator case for KJob. It comes from `experiments/kjob/operator-helm-smoketest` in the SMP repository, at commit `334662d00` on the KJob branch, with all optional settings removed.

## What it exercises

The SMP CLI renders the Datadog Operator Helm chart at submission. KJob then runs the rendered resources. A minimal `DatadogAgent` resource makes the Operator start the Node Agent and the Cluster Agent with their default configuration.

## Load

One Operator, one Cluster Agent, and a Node Agent DaemonSet. The fixture adds no activity Pods and no application traffic.

## Assertions

None. The KJob report has no checks and always passes. A good result shows only that the run completed.

## Special Considerations

- Helm `v4.3.1` renders chart `datadog-operator` `2.29.0`. The SMP CLI needs exactly this Helm version on `PATH`.
- The images are fixed. The chart default sets the Operator to `1.31.0`, and that Operator selects its default Node Agent and Cluster Agent images. The fixture has no parameters, so `--override` flags do not apply.
- The values file only disables the Datadog CSI Driver controller. All other chart values use their defaults.
- One HTTP blackhole, `intake`, receives the payloads for the main Datadog endpoint. Other products, such as APM, processes, Remote Configuration and telemetry, try to reach public Datadog endpoints. KJob targets have no internet access, so these calls fail.
- The target namespace is `operator-helm-smoketest`.
- See the [KJob user guide](https://github.com/ddoghq/single-machine-performance/blob/paul.reinlein/krjo-poc/docs/kjob/user-guide.md) for the case format and the `smp kjob` commands.
