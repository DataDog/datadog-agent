# cluster-agent-with-load

The Cluster Agent alone on KinD, with 1,000 simulated Pods in the cluster.

## What it exercises

The minimal Cluster Agent with many Pods. The Cluster Agent collects Kubernetes tags for all Pods (`DD_CLUSTER_AGENT_COLLECT_KUBERNETES_TAGS=true`), and it collects the events of the Pods. The Kubernetes API also gets more load.

## Load

A `Deployment` of 1,000 Pods in `activity/workload.yaml`. The Pods do not run containers.

## Assertions

An upper bound on the peak working-set memory of the Cluster Agent container:

| Container | Upper bound |
|---|---:|
| `cluster-agent` (Cluster Agent) | 81 MiB |

The bound is the highest value from 10 CI replicates plus 10%, rounded to the nearest MiB.

## Special Considerations

- The case uses the fixed image `registry.datadoghq.com/cluster-agent:7.84.6`.
- The case runs on KinD. The same case without load is `cluster-agent-idle`.
- `target/cluster-agent.yaml` holds only the resources that the Cluster Agent needs to run. The Cluster Agent uses the default ServiceAccount of the namespace. It reads Pods, Nodes, Namespaces, Endpoints, ComponentStatuses and Events, and it keeps its leader election lock in a ConfigMap. Orchestrator Explorer is off, because it needs more RBAC. Cluster checks and the Admission Controller are off by default. So the Cluster Agent does less work than in a default Helm or Operator deployment.
- The ClusterRoleBinding names the namespace of the case. If you rename the case, change this namespace too.
- The Cluster Agent sends its payloads to the HTTP receiver in `experiment.yaml`. Remote Configuration and Agent telemetry are off, because the cluster has no internet access.
