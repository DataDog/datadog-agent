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

## Expected Attributes

The infra attributes processor [looks up the following resource attributes](https://github.com/DataDog/datadog-agent/blob/7d51e9e0dc9fb52aab468b372a5724eece97538c/comp/otelcol/otlp/components/processor/infraattributesprocessor/metrics.go#L42-L77) in order to extract Kubernetes Tags. These resource attributes can be set in your SDK or in your otel-agent collector configuration:

| *[Entity](../../../../../../comp/core/tagger/README.md#entity-ids)*  | *Resource Attributes*                       |
|----------------------------------------------------------------------|---------------------------------------------|
| workloadmeta.KindContainer                                           | `container.id`                              |
| workloadmeta.KindContainerImageMetadata                              | `container.image.id`                        |
| workloadmeta.KindECSTask                                             | `aws.ecs.task.arn`                          |
| workloadmeta.KindKubernetesDeployment                                | `k8s.deployment.name`, `k8s.namespace.name` |
| workloadmeta.KindKubernetesMetadata                                  | `k8s.namespace.name`, `k8s.node.name`       |
| workloadmeta.KindKubernetesPod                                       | `k8s.pod.uid`                               |
| workloadmeta.KindProcess                                             | `process.pid`                               |

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

If you are using OTel SDK auto-instrumentation, `container.id` and `process.pid` will be automatically set by your SDK.

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
    batch:
      send_batch_max_size: 1000
      send_batch_size: 100
      timeout: 10s
  exporters:
    datadog:
      api:
        site: ${env:DD_SITE}
        key: ${env:DD_API_KEY}
      traces:
        trace_buffer: 500
  service:
    pipelines:
      metrics:
        receivers: [otlp]
        processors: [batch, resourcedetection, k8sattributes]
        exporters: [datadog]
      traces:
        receivers: [otlp]
        processors: [batch, resourcedetection, k8sattributes]
        exporters: [datadog]
      logs:
        receivers: [otlp]
        processors: [batch, resourcedetection, k8sattributes]
        exporters: [datadog]
```

## List of Kubernetes Tags

For the full list of Kubernetes Tags added by the infra attributes processor, see [comp/core/tagger/tags/tags.go](../../../../../../comp/core/tagger/tags/tags.go).
