# Resource requests and limits

| QoS class | Requests | Limits | Trade-off |
|---|---|---|---|
| Best-effort | none | none | No reservation, no cap; first to be evicted under node memory pressure |
| Burstable (default) | none | set | No reservation; usage capped to protect the node |
| Guaranteed | = limits | set | Reserves capacity on every node; best eviction stability and resource visibility |

The provided manifests set limits of 1 CPU and 1 GiB memory. These values fit most deployments but can be tuned:

- **Large clusters or dense nodes**: consider adjusting limits based on observed usage, as overhead scales with the number of running processes.
- **Large native binaries**: increase the memory limit when running workloads with large debug symbols, as symbol processing requires additional working memory.
- **Guaranteed QoS**: set requests equal to limits if your cluster's observability tools assume they are equal, or if you need predictable eviction behavior.
