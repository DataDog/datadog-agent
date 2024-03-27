# package `custommetrics`

This package is a part of the Datadog Cluster Agent and is responsible for providing custom metrics to the Kubernetes apiserver for Horizontal Pod Autoscalers.

## DatadogProvider

The `DatadogProvider` currently only implements the External Metrics Provider interface by providing external metrics from Datadog.

There is no guarantee that every replica returns the exact same list of metrics for `ListAllExternalMetrics`. It is possible for the leader to mutate the store between calls to `ListAllExternalMetricValues` from different replicas. This is the tradeoff of using a `ConfigMap` for persistent storage instead of a transactional store.

## Store

The `Store` interface provides persistent storage of custom and external metrics. The default implementation stores metric values in a `ConfigMap`.

Only the leader replica should add or delete metrics from the store. Any replica is able to call `ListAllExternalMetricValues` to return the most up-to-date values for metrics.

### configMapStore

The `configMapStore` provides simple persistent storage of custom and external metrics. This allows any replica of the Datadog Cluster Agent to serve metrics to the apiserver but still only have the leader replica query Datadog.

The `configMapStore` always performs operations on a local copy of the configmap but `ListAllExternalMetricValues` will always get the most up-to-date configmap from the apiserver before listing the metrics.

#### Summary of apiserver calls

- `NewConfigMapStore`: between 1 and 2 calls
- `SetExternalMetricValues`: 1 call to update configmap
- `DeleteExternalMetricValues`: 1 call to update configmap
- `ListAllExternalMetricValues`: 1 call to get configmap
