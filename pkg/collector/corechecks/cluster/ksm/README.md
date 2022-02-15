# Kubernetes State Core Check

The Kubernetes State Core Check is an alternative to [Kubernetes State Python Check](https://github.com/DataDog/integrations-core/tree/master/kubernetes_state). It leverages the `kube-state-metrics` project and extends it to generate Datadog metrics.

[Public documentation page.](https://docs.datadoghq.com/integrations/kubernetes_state_core)

**Notes:**

- The data collected by the checks is documented in `kubernetes_state.md`. The file shares the same format used for the public documentation to ease the synchronization.
- If a code change updates/removes/adds metrics or service checks, please update `kubernetes_state.md` accordingly.
- The public documentation page lives in the [DataDog/documentation repo](https://github.com/DataDog/documentation/blob/master/content/en/integrations/kubernetes_state_core.md). It needs to be synchronized with `kubernetes_state.md` on every new release of the Agent.
