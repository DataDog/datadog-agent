# GPU job metadata prototype

This is a small prototype for carrying job identity from a training workload into GPU telemetry without changing the GPU check itself.

## Summary

A training process sends a reserved DogStatsD event when a GPU job starts, updates, heartbeats, or ends. The event carries a job id and optional tags such as `team:ml` or `phase:train`.

The Agent uses existing DogStatsD origin detection to determine which container sent the event, then publishes those tags into the existing Agent tagger for that container. The GPU check already looks up container tags when it emits GPU metrics, so the same tags are picked up through the normal tag path.

The prototype tag name is currently `gpu_job_id`. The mechanism is independent of the final spelling; it could be renamed or aliased to `ml_job_id` if that is the desired product tag.

## Data flow

```text
training process
  -> DogStatsD reserved event over the existing metrics socket
  -> DogStatsD parses lifecycle + tags
  -> Agent origin detection maps sender to container id
  -> Agent tagger stores tags for container_id://<container>
  -> GPU check emits metrics with normal container/tagger tags
```

For local demos that do not use a Unix socket, the sender can provide the container id with DogStatsD client-origin syntax (`c:ci-<container_id>`). In the intended socket path, the Agent can use the socket sender identity it already relies on for origin detection.

## Control event shape

Start/update:

```text
_e{15,5}:datadog.gpu.job|start|s:datadog_gpu_job|#gpu_job_id:job-123,team:ml
```

End/clear:

```text
_e{15,3}:datadog.gpu.job|end|s:datadog_gpu_job
```

Supported publish actions: `start`, `update`, `heartbeat`.

Supported clear actions: `end`, `stop`, `complete`, `completed`.

The Agent consumes these reserved events as control-plane input; they are not meant to be forwarded as normal Datadog events.

## What was built

- `pkg/gpu/jobmetadata`: validates and normalizes the reserved events.
- DogStatsD server integration: consumes those events when `gpu.job_metadata.enabled` is true.
- Tagger bridge: publishes tags on `container_id://<container>` from source `dogstatsd-gpu-job-metadata`.
- Clear path: end events replace that source with an empty tag set for the container.
- Optional fallback TTL: default is `0s`, meaning tags remain until an explicit end event.
- Demo check for non-Linux/macOS validation: emits a placeholder metric using the same tagger lookup.
- Linux demo support: runs the real `gpu` check with a minimal fake NVML library and a small Docker workload.

## What did not change

- The GPU check does not contain custom job-metadata logic.
- There is no aggregator-level metric-name enrichment.
- There is no workloadmeta schema change in this prototype.
- There is no Slurm/job-to-PID correlation logic in this prototype.
- Container-deletion cleanup is not implemented as a separate path yet; explicit end events are the primary lifecycle signal, with TTL available as a stale-data fallback.

## Demo status

Local macOS validation uses the placeholder check only, because the real GPU check is Linux/NVML-specific.

On Linux, the intended demo command is:

```bash
tools/gpu_job_metadata_demo_cmux.py --build
```

That path builds the Agent and fake NVML library, starts a small Docker container, runs the real `gpu` check, and sends start/end events so `gpu.*` series should show the dynamic job tags.

## Open questions before productionizing

- Final tag spelling: `gpu_job_id`, `ml_job_id`, or both.
- Which extra user-supplied tags should be allowed.
- Source priority/conflict behavior for tags from `dogstatsd-gpu-job-metadata`.
- Whether container lifecycle cleanup should be explicit in workloadmeta/tagger in addition to end events and TTL.
