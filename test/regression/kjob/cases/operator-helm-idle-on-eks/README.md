# operator-helm-idle-on-eks

The Operator, the Node Agent and the Cluster Agent on EKS, with no added load.

## What it exercises

The Operator, installed from its Helm chart, starts the Node Agent and the Cluster Agent with their default configuration. This case gives the memory baseline of an idle deployment.

## Load

None. The case adds no Pods and no traffic.

## Assertions

Upper bounds on the peak working-set memory of each container:

| Container | Upper bound |
|---|---:|
| `agent` (Node Agent) | 259 MiB |
| `cluster-agent` (Cluster Agent) | 110 MiB |

Each bound is the highest value from 10 CI replicates plus 10%, rounded to the nearest MiB.

## Special Considerations

- The case uses fixed images: the Helm chart `datadog-operator` `2.29.0` installs Operator `1.31.0`, which selects its default Agent images.
- Results on EKS are not comparable with `operator-helm-idle-on-kind`. The same case with load is `operator-helm-with-load-on-eks`.
- The Agent sends its payloads to the HTTP receivers in `experiment.yaml`. Remote Configuration, Agent telemetry and the NTP check are off, because the cluster has no internet access.
