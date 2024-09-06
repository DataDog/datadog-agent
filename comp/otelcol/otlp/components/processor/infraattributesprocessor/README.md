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
* `cardinality: 0` - **LowCardinality**: in the host count order of magnitude
* `cardinality: 1` - **OrchestratorCardinality**: tags that change value for each pod or task
* `cardinality: 2` - **HighCardinality**: typically tags that change value for each web request, user agent, container, etc.

## Expected Attributes

The infra attributes processor looks up the following resource attributes in order to extract Kubernetes Tags. These resource attributes can be set in your SDK or in your otel-agent collector configuration:

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

### Collector Configuration

The expected resource attributes can be set by configuring the [Kubernetes attributes processor and resource detection processor](https://docs.datadoghq.com/opentelemetry/collector_exporter/hostname_tagging/?tab=kubernetesdaemonset), [Docker stats receiver](https://docs.datadoghq.com/opentelemetry/integrations/docker_metrics/?tab=host), and [host metrics receiver](https://docs.datadoghq.com/opentelemetry/integrations/host_metrics/?tab=host):
```
receivers:
  docker_stats:
    endpoint: unix:///var/run/docker.sock # (default)
    metrics:
      container.network.io.usage.rx_packets:
        enabled: true
      container.network.io.usage.tx_packets:
        enabled: true
      container.cpu.usage.system:
        enabled: true
      container.memory.rss:
        enabled: true
      container.blockio.io_serviced_recursive:
        enabled: true
      container.uptime:
        enabled: true
      container.memory.hierarchical_memory_limit:
        enabled: true
  hostmetrics:
    collection_interval: 10s
    scrapers:
      paging:
        metrics:
          system.paging.utilization:
            enabled: true
      cpu:
        metrics:
          system.cpu.utilization:
            enabled: true
      disk:
      filesystem:
        metrics:
          system.filesystem.utilization:
            enabled: true
      load:
      memory:
      network:
      processes:

processors:
  batch:
    send_batch_max_size: 1000
    send_batch_size: 100
    timeout: 10s
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

service:
  pipelines:
    metrics:
      receivers: [docker_stats, hostmetrics]
      processors: [batch, resourcedetection, k8sattributes]
      exporters: [datadog]
    traces:
      receivers: []
      processors: [batch, resourcedetection, k8sattributes]
      exporters: [datadog]
    logs:
      receivers: []
      processors: [batch, resourcedetection, k8sattributes]
      exporters: [datadog]
```

## List of Kubernetes Tags

For the full list of Kubernetes Tags added by the infra attributes processor, see [comp/core/tagger/tags/tags.go](../../../../../../comp/core/tagger/tags/tags.go).
