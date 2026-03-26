# comp/languagedetection/client

**Team:** container-platform

## Purpose

This component runs inside the node Agent (process-agent) and is responsible for detecting the programming languages used by processes running in Kubernetes pods, then streaming that information to the Cluster Agent. The Cluster Agent uses this data to annotate pod owners (Deployments, DaemonSets, etc.) with language metadata, enabling APM library auto-injection.

The component is optional: it is only instantiated when all three of the following configuration keys are `true`:
- `language_detection.enabled`
- `language_detection.reporting.enabled`
- `cluster_agent.enabled`

## Key elements

### Component interface

`comp/languagedetection/client/def/component.go`

```go
type Component interface{}
```

The interface itself carries no exported methods. The component's behavior is entirely driven internally: it subscribes to workloadmeta events and runs a background loop. The `Provides` struct wraps the component in an `option.Option` so callers can check whether it is actually enabled at runtime.

### Constructor

`NewComponent(reqs Requires) (Provides, error)` — `comp/languagedetection/client/impl/client.go`

Reads configuration, builds a `languageDetectionClientImpl`, and registers `OnStart`/`OnStop` lifecycle hooks. Returns `option.None` when the feature is disabled.

### Internal types

| Type | Description |
|------|-------------|
| `languageDetectionClientImpl` | Main struct. Holds the current batch, the set of freshly updated pods, the retry map, and timing configuration. |
| `batch` (`map[string]*podInfo`) | Accumulates per-pod language data keyed by pod name. Converted to a `pbgo.ParentLanguageAnnotationRequest` proto before sending. |
| `podInfo` | Tracks namespace, owner reference, container-to-language mapping, and the expected set of containers for a pod. |
| `eventsToRetry` | Wraps process events that arrived before their pod was available in workloadmeta, with an expiration timestamp. |

### Two-tier flush strategy

The component runs two independent timers:

1. **Fresh-data timer** (`language_detection.reporting.buffer_period`, default ~10 s) — sends only the pods that were updated since the last flush. Limits the rate of messages to the Cluster Agent.
2. **Periodic flush timer** (`language_detection.reporting.refresh_period`, default ~10 m) — sends the entire current batch, acting as a built-in retry for any previously missed data.

### Race-condition handling

Because the kubelet collector and process collector run independently, process events can arrive before the corresponding pod is stored in workloadmeta. Events in that state are held in `processesWithoutPod` (keyed by container ID) and retried when the pod's `EventTypeSet` event is later received. Stale entries are pruned every hour (TTL: 5 min per entry).

### Telemetry

Metrics are emitted under the `language_detection_dca_client` subsystem:

| Metric | Description |
|--------|-------------|
| `processed_events` | Counter — events processed, tagged by namespace, pod, container, and language |
| `latency` | Histogram — time (seconds) to `PostLanguageMetadata` to the Cluster Agent |
| `process_without_pod` | Counter — process events queued for retry due to missing pod |
| `requests` | Counter — HTTP requests to the Cluster Agent, tagged by `status` (`success`/`error`) |

## Usage

The component is wired into the node Agent's fx graph in `cmd/agent/subcommands/run/command.go`. It depends on:

- `comp/core/config` — reads timing and feature-flag configuration.
- `comp/core/workloadmeta` — subscribes to `KindProcess` and `KindKubernetesPod` events.
- `comp/core/log` and `comp/core/telemetry` — observability.
- `pkg/util/clusteragent` — lazily obtains the Cluster Agent HTTP client used to call `PostLanguageMetadata`.

The fx module is provided by `comp/languagedetection/client/fx/fx.go`. A no-op mock (`comp/languagedetection/client/mock/mock.go`) is available for tests that need to inject the option without real Cluster Agent connectivity.

### Configuration keys

| Key | Default | Description |
|-----|---------|-------------|
| `language_detection.enabled` | `false` | Master switch for language detection |
| `language_detection.reporting.enabled` | `false` | Enables reporting to the Cluster Agent |
| `language_detection.reporting.buffer_period` | `10s` | How long to buffer fresh updates before sending |
| `language_detection.reporting.refresh_period` | `10m` | Interval for the full periodic flush |
| `cluster_agent.enabled` | `false` | Must be true for the component to start |
