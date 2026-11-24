# cluster-agent-idle

The Cluster Agent alone on KinD, with no added load.

## What it exercises

A minimal Deployment of the Cluster Agent, with no Node Agent and no Operator. The Cluster Agent runs its leader election, its tagger and the default `kubernetes_apiserver` check, with event collection on. This case gives the memory baseline of an idle Cluster Agent.

## Load

None. The case adds no Pods and no traffic.

## Assertions

None yet. A first CI run gives the memory data for the bounds.

## Special Considerations

- The case uses the fixed image `registry.datadoghq.com/cluster-agent:7.84.6`.
- The case runs on KinD. The same case with load is `cluster-agent-with-load`.
- `target/cluster-agent.yaml` holds only the resources that the Cluster Agent needs to run. The Cluster Agent uses the default ServiceAccount of the namespace. It reads Pods, Nodes, Namespaces, Endpoints, ComponentStatuses and Events, and it keeps its leader election lock in a ConfigMap. Orchestrator Explorer is off, because it needs more RBAC. Cluster checks and the Admission Controller are off by default. So the Cluster Agent does less work than in a default Helm or Operator deployment.
- The ClusterRoleBinding names the namespace of the case. If you rename the case, change this namespace too.
- The Cluster Agent sends its payloads to the HTTP receiver in `experiment.yaml`. Remote Configuration and Agent telemetry are off, because the cluster has no internet access.
