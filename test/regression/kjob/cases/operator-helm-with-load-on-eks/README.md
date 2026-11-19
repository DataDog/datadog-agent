# operator-helm-with-load-on-eks

The Operator, the Node Agent and the Cluster Agent on EKS, with 1,000 simulated Pods in the cluster.

## What it exercises

The Cluster Agent with many Pods. The Cluster Agent collects Kubernetes tags for all Pods (`DD_CLUSTER_AGENT_COLLECT_KUBERNETES_TAGS=true`). The Operator and the Kubernetes API also get more load.

## Load

A `Deployment` of 1,000 Pods in `activity/workload.yaml`. The Pods do not run containers, so they put no load on the Node Agent.

## Assertions

None yet. Bounds for the `agent` and `cluster-agent` containers come after CI runs give data.

## Special Considerations

- The case uses fixed images: the Helm chart `datadog-operator` `2.29.0` installs Operator `1.31.0`, which selects its default Agent images.
- Results on EKS are not comparable with `operator-helm-with-load-on-kind`. The same case without load is `operator-helm-idle-on-eks`.
- The Agent sends its payloads to the HTTP receivers in `experiment.yaml`. Remote Configuration, Agent telemetry and the NTP check are off, because the cluster has no internet access.
