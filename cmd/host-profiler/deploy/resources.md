# Resource requests and limits

| Requests | Limits | QoS class | Trade-off |
|---|---|---|---|
| none | none | Best-effort | No reservation, no cap; first to be evicted under node memory pressure |
| none | CPU: `1`, Memory: `1Gi` | Burstable | No reservation; usage capped to protect the node |
| CPU: `1`, Memory: `1Gi` | CPU: `1`, Memory: `1Gi` | Guaranteed | Reserves capacity on every node; best eviction stability and resource visibility |

The provided manifests use the Burstable configuration. Increase the memory limit on nodes that run large native binaries, as debug symbol processing requires additional memory.

Use Guaranteed if your cluster's observability tools assume requests and limits are equal, or if you need predictable eviction behavior.
