# Tag GPU hardware metrics with training identifiers

With GPU Monitoring enabled, add these annotations to each training worker pod:

```yaml
metadata:
  annotations:
    training.datadoghq.com/run-id: "run-42"
    training.datadoghq.com/group-id: "experiment-7"
```

Use the same run ID on every worker in a distributed training run. The optional
group ID groups related runs. Put the annotations on the pod template when a
controller creates the workers, for example under `spec.template.metadata` for
a Job.

The Agent extracts these annotations as `training_run_id` and `training_group_id`
and attaches the tags to hardware metrics for GPUs allocated to the pod's
containers. No `gpu.jobs` or Kubernetes metadata extraction configuration is
needed for these default annotation keys. Pods without the annotations receive
no training identifiers from these defaults.

To verify newly collected metrics, query:

```text
avg:gpu.temperature{training_run_id:run-42} by {gpu_uuid}
```

## Existing identifiers and overrides

To read a different pod label, annotation, or process environment variable,
configure the corresponding identifier in `datadog.yaml`. For example:

```yaml
gpu:
  jobs:
    run:
      key: existing-run-id
      type: label
```

This replaces the default run annotation source; the group annotation default
still applies. The default source type is `annotation`, so set `type: label` or
`type: env` explicitly for other sources. An empty `gpu.jobs.run.key` or
`gpu.jobs.group.key` disables that identifier. Existing explicit metadata
extraction mappings retain their precedence.

## GPU tracing

When GPU tracing is enabled and a pod is selected for GPU tracer injection, the
Cluster Agent also reads these default annotations through the Kubernetes
Downward API to populate `DD_TRAINING_RUN_ID` and `DD_TRAINING_GROUP_ID`.
Traces use `training.run_id` and `training.group_id` for the same identifiers.
Adding the annotations alone does not enable tracing or opt a pod into injection.
