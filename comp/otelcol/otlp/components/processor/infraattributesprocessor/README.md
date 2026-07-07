# Infra Attributes Processor

The infra attributes processor extracts [Kubernetes tags](https://docs.datadoghq.com/containers/kubernetes/tag/?tab=datadogoperator#out-of-the-box-tags) based on labels or annotations and assigns these tags as resource attributes on traces, metrics, and logs.

When telemetry is exported from the otel-agent, these infra attributes will be converted into Datadog tags and used as metadata in [Container Monitoring](https://docs.datadoghq.com/containers/).

## Configuration

The infra attributes processor will be added automatically by the [converter component](../../../../converter/README.md). If you opted out of the converter, or you want to change the defaults, you are able to configure the processor as so:
```
processors:
  infraattributes:
    cardinality: 0
```

The infra attributes processor also needs to be included in the pipelines in order to take effect:
```
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [infraattributes]
      exporters: [datadog/connector, datadog]
    metrics:
      receivers: [otlp, datadog/connector]
      processors: [infraattributes]
      exporters: [datadog]
    logs:
      receivers: [otlp]
      processors: [infraattributes]
      exporters: [datadog]
```

### Cardinality
The cardinality option sets the [TagCardinality](../../../../../../comp/core/tagger/README.md#tagcardinality) in the Datadog Agent tagger component. Possible values for this option include:
* `cardinality: 0` - **LowCardinality**: in the host count order of magnitude *(default)*
* `cardinality: 1` - **OrchestratorCardinality**: tags that change value for each pod or task
* `cardinality: 2` - **HighCardinality**: typically tags that change value for each web request, user agent, container, etc.

### Container tag promotion

**This option only affects the traces pipeline.** `_dd.tags.container` promotion is a trace-agent-specific mechanism; logs, metrics, and profiles never go through it, so the logs/metrics/profiles processors always behave as `off` regardless of this setting. (Metrics in particular already recognize DD-format keys directly, so prefixing them would only risk stranding data under `rename`.)

Downstream (trace-agent / Datadog exporter) only promotes a resource attribute into `_dd.tags.container` (visible in the Infrastructure tab of a span) if its key matches a known DD or OTel container-tag convention, or if it carries the `datadog.container.tag.` prefix. Custom tags emitted by this processor — for example tags produced by `podLabelsAsTags` — fall into neither category and are therefore silently dropped from container tags.

The `trace_container_tag_promotion` option opts into rewriting these custom tags so the downstream promotion path picks them up:

* `trace_container_tag_promotion: off` *(default)* — tags are written as-is. Existing behavior.
* `trace_container_tag_promotion: duplicate` — each non-exempt tag is written twice: once under its non-prefixed key **and** once under the `datadog.container.tag.<key>` prefixed key. The non-prefixed tag survives for any downstream consumer that reads the raw key; the prefixed copy reaches `_dd.tags.container`.
* `trace_container_tag_promotion: rename` — each non-exempt tag is written **only** under the `datadog.container.tag.<key>` prefixed key. Smaller resource payload, but consumers that read the non-prefixed key lose access to the value.

In `duplicate` mode the non-prefixed and prefixed forms are written independently, so the two can coexist. If a `datadog.container.tag.<key>` prefixed attribute is already present on the incoming resource (see exemptions below), the processor keeps that value and still writes the tagger-derived non-prefixed key alongside it — the result is both the pre-existing prefixed tag and the non-prefixed tag.

**Exemptions (never prefixed, regardless of mode):**
* Keys recognized by trace-agent's container-tag promotion path (`ConsumeContainerTagsFromResource`) — the union of `attributes.ContainerMappings` keys (OTel semantic conventions: `k8s.pod.name`, `container.id`, `container.image.name`, ...) and its values (DD-format names produced by the OTel→DD mapping: `pod_name`, `kube_namespace`, `container_id`, `runtime`, `cloud_provider`, ...). These already reach `_dd.tags.container` under their canonical key.
* USM keys (`service`, `env`, `version`) — flow through their own path to `service.name` / `deployment.environment` / `service.version`.
* `datadog.host.name` (when `allow_hostname_override: true`) — reserved host attribute.
* Keys already starting with `datadog.container.tag.` — idempotent, never re-prefixed.
* A `datadog.container.tag.<X>` attribute already present on the incoming resource (typically set by the sender as a manual workaround) — preserved as-is. The processor only writes its prefixed copy when the key is absent, so a user-supplied value is never overwritten by the tagger-derived one.

Note that DD-format keys emitted by the tagger that are **not** in `ContainerMappings` (`kube_service`, `pod_phase`, `kube_qos`, `kube_priority_class`, `kube_app_*`, `image_id`, `docker_image`, `git.commit.sha`, ...) are treated as custom for this feature and **are** prefixed by `duplicate` / `rename` — trace-agent does not recognize them for container-tag promotion on their own.

Example:
```
processors:
  infraattributes:
    cardinality: 2
    trace_container_tag_promotion: duplicate
```

## Expected Attributes

The infra attributes processor [looks up the following resource attributes](https://github.com/DataDog/datadog-agent/blob/a7e58c617398e40e4d9f730f855b5bda963f3d42/comp/otelcol/otlp/components/processor/infraattributesprocessor/common.go#L90-L125) in order to extract Kubernetes Tags. These resource attributes can be set in your SDK or in your otel-agent collector configuration:

| *[Entity](../../../../../../comp/core/tagger/README.md#entity-ids)*  | *Resource Attributes*                       |
|----------------------------------------------------------------------|---------------------------------------------|
| workloadmeta.KindContainer                                           | `container.id`                              |
| workloadmeta.KindContainerImageMetadata                              | `oci.manifest.digest`                        |
| workloadmeta.KindECSTask                                             | `aws.ecs.task.arn`                          |
| workloadmeta.KindKubernetesDeployment                                | `k8s.deployment.name`, `k8s.namespace.name` |
| workloadmeta.KindKubernetesMetadata                                  | `k8s.namespace.name`, `k8s.node.name`       |
| workloadmeta.KindKubernetesPod                                       | `k8s.pod.uid`                               |
| workloadmeta.KindProcess                                             | `process.pid`                               |

## Container ID detection

If the `container.id` resource attribute is not present on the input, the infra attributes processor will attempt to automatically
detect it. This follows the same logic as the Agent's native Origin detection, but using resource attributes as the data source.
The following methods are tried, in order from highest to lowest precedence:

| *Resource attributes*                            | *Detection method*                |
|--------------------------------------------------|-----------------------------------|
| `process.pid` (int)                              | Based on container's external PID |
| `datadog.container.cgroup_inode` (int)           | Based on container's cgroup inode |
| `k8s.pod.uid` (str) + `k8s.container.name` (str) | Based on container's pod and name |

For the method based on the container name, the `datadog.container.is_init` (boolean) resource attribute may be set to `true` to
direct the processor's search towards init-containers rather than regular ones.

### SDK Configuration

The expected resource attributes can be set by using the `OTEL_RESOURCE_ATTRIBUTES` environment variable. For example, this can be set in your Kubernetes deployment yaml:
```
env:
  ...
  - name: OTEL_SERVICE_NAME
    value: {{ include "calendar.fullname" . }}
  - name: OTEL_K8S_NAMESPACE
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.namespace
  - name: OTEL_K8S_NODE_NAME
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: spec.nodeName
  - name: OTEL_K8S_POD_NAME
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.name
  - name: OTEL_K8S_POD_ID
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.uid
  - name: OTEL_RESOURCE_ATTRIBUTES
    value: >-
      service.name=$(OTEL_SERVICE_NAME),
      k8s.namespace.name=$(OTEL_K8S_NAMESPACE),
      k8s.node.name=$(OTEL_K8S_NODE_NAME),
      k8s.pod.name=$(OTEL_K8S_POD_NAME),
      k8s.pod.uid=$(OTEL_K8S_POD_ID),
      k8s.container.name={{ .Chart.Name }},
      host.name=$(OTEL_K8S_NODE_NAME),
      deployment.environment=$(OTEL_K8S_NAMESPACE)
```

When using OTel SDK auto-instrumentation, some SDKs automatically set `container.id` and `process.pid`, while others may require manual configuration..

### Collector Configuration

The expected resource attributes can be set by configuring the [Kubernetes attributes processor and resource detection processor](https://docs.datadoghq.com/opentelemetry/collector_exporter/hostname_tagging/?tab=kubernetesdaemonset). This is demonstrated in the [k8s-values.yaml](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/datadogexporter/examples/k8s-chart/k8s-values.yaml) example:
```
mode: daemonset
presets:
  kubernetesAttributes:
    enabled: true
extraEnvs:
  - name: POD_IP
    valueFrom:
      fieldRef:
        fieldPath: status.podIP
  - name: OTEL_RESOURCE_ATTRIBUTES
    value: "k8s.pod.ip=$(POD_IP)"
config:
  processors:
    k8sattributes:
      passthrough: false
      auth_type: "serviceAccount"
      pod_association:
        - sources:
            - from: resource_attribute
              name: k8s.pod.ip
      extract:
        metadata:
          - k8s.pod.name
          - k8s.pod.uid
          - k8s.deployment.name
          - k8s.node.name
          - k8s.namespace.name
          - k8s.pod.start_time
          - k8s.replicaset.name
          - k8s.replicaset.uid
          - k8s.daemonset.name
          - k8s.daemonset.uid
          - k8s.job.name
          - k8s.job.uid
          - k8s.cronjob.name
          - k8s.statefulset.name
          - k8s.statefulset.uid
          - container.image.name
          - container.image.tag
          - container.id
          - k8s.container.name
          - container.image.name
          - container.image.tag
          - container.id
        labels:
          - tag_name: kube_app_name
            key: app.kubernetes.io/name
            from: pod
          - tag_name: kube_app_instance
            key: app.kubernetes.io/instance
            from: pod
          - tag_name: kube_app_version
            key: app.kubernetes.io/version
            from: pod
          - tag_name: kube_app_component
            key: app.kubernetes.io/component
            from: pod
          - tag_name: kube_app_part_of
            key: app.kubernetes.io/part-of
            from: pod
          - tag_name: kube_app_managed_by
            key: app.kubernetes.io/managed-by
            from: pod
    resourcedetection:
      detectors: [env, eks, ec2, system]
      timeout: 2s
      override: false
  exporters:
    datadog:
      api:
        site: ${env:DD_SITE}
        key: ${env:DD_API_KEY}
      traces:
        trace_buffer: 500
      sending_queue:
        batch:
  service:
    pipelines:
      metrics:
        receivers: [otlp]
        processors: [resourcedetection, k8sattributes]
        exporters: [datadog]
      traces:
        receivers: [otlp]
        processors: [resourcedetection, k8sattributes]
        exporters: [datadog]
      logs:
        receivers: [otlp]
        processors: [resourcedetection, k8sattributes]
        exporters: [datadog]
```

## List of Kubernetes Tags

For the full list of Kubernetes Tags added by the infra attributes processor, see [comp/core/tagger/tags/tags.go](../../../../../../comp/core/tagger/tags/tags.go).
