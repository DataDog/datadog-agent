# operator-helm-with-load

The Operator, the Node Agent and the Cluster Agent on KinD, with 1,000 simulated Pods in the cluster.

## What it exercises

The Cluster Agent with many Pods. The Cluster Agent collects Kubernetes tags for all Pods (`DD_CLUSTER_AGENT_COLLECT_KUBERNETES_TAGS=true`). The Operator and the Kubernetes API also get more load.

## Load

A `Deployment` of 1,000 Pods in `activity/workload.yaml`. The Pods do not run containers, so they put no load on the Node Agent.

## Assertions

Upper bounds on the peak working-set memory of each container:

| Container | Upper bound |
|---|---:|
| `agent` (Node Agent) | 169 MiB |
| `cluster-agent` (Cluster Agent) | 199 MiB |

Each bound is the highest value from 10 CI replicates plus 10%, rounded to the nearest MiB.

## Special Considerations

- The case uses fixed images: the Helm chart `datadog-operator` `2.29.0` installs Operator `1.31.0`, which selects its default Agent images.
- The case runs on KinD. The same case without load is `operator-helm-idle`.
- The Agent sends its payloads to the HTTP receivers in `experiment.yaml`. Remote Configuration, Agent telemetry and the NTP check are off, because the cluster has no internet access.
