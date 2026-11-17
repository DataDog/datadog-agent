# Operator Helm smoketest

This fixture is the smallest Operator case for KJob.

## What it exercises

The SMP CLI renders the Datadog Operator Helm chart at submission. KJob then runs the rendered resources. A minimal `DatadogAgent` resource makes the Operator start the Node Agent and the Cluster Agent with their default configuration.

## Load

One Operator, one Cluster Agent, and a Node Agent DaemonSet. The fixture adds no activity Pods and no application traffic.

## Assertions

None. The KJob report has no checks and always passes. A good result shows only that the run completed without failures.

## Special Considerations

- The images are fixed. The chart default sets the Operator to `1.31.0`, and that Operator selects its default Node Agent and Cluster Agent images.
- The values file disables the Datadog CSI Driver controller and the Operator metrics forwarder, which sends to the Datadog API. All other chart values use their defaults.
- Four HTTP blackholes, `intake`, `process`, `orchestrator` and `apm`, receive the Agent payloads. The `DatadogAgent` sends the main, process, orchestrator, APM, container image and container lifecycle payloads to them.
- The fixture turns off Remote Configuration and Agent telemetry, and it excludes the NTP check. These features otherwise call public endpoints.
- KJob targets have no internet access. Cluster DNS still resolves public names, but a managed NetworkPolicy blocks the connections. So a call that this fixture does not cover fails with a timeout.
- The target namespace is `operator-helm-smoketest`.
- See the [KJob user guide](https://github.com/ddoghq/single-machine-performance/blob/paul.reinlein/krjo-poc/docs/kjob/user-guide.md) for the case format and the `smp kjob` commands.
  - *Note: This will be updated later to an actual public confluence d
