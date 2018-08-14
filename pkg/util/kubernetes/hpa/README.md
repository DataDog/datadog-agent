# package `hpa`

This package is a part of the Datadog Cluster Agent and is responsible for watching the `HorizontalPodAutoscaler` resource and querying Datadog for external metrics specified by HPAs.

## HPAWatcherClient

The watcher starts a single loop to perform the following tasks:

- Start a watch for changes to HPAs and process the changes
- Query Datadog to update external metric values
- Garbage collect external metrics values in the store that reference deleted HPAs
