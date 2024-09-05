# Infra Attributes Processor

The infra attributes processor extracts [Kubernetes Datadog tags](https://docs.datadoghq.com/containers/kubernetes/tag/?tab=datadogoperator#out-of-the-box-tags) based on labels or annotations and assigns these tags as resource attributes on traces, metrics, and logs.

When telemetry is exported from the otel-agent, these infra attributes will be converted into Kubernetes Datadog tags and used as metadata in [Container Monitoring](https://docs.datadoghq.com/containers/).

## Configuration

The infra attributes processor will be added automatically by the [converter component](../../../../converter/README.md). If you opted out of the converter, or you want to change the defaults, you are able to configure the processor as so:
```
processors:
  infraattributes:
    cardinality: 0
```

The infra attributes processor also needs to be included in the pipelines section in order to be enabled:
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

The infra attributes processor looks up the following resource attributes in order to extract Kubernetes Datadog tags:

| *[Entity](../../../../../../comp/core/tagger/README.md#entity-ids)*  | *Resource Attributes*                       |
|----------------------------------------------------------------------|---------------------------------------------|
| workloadmeta.KindContainer                                           | `container.id`                              |
| workloadmeta.KindContainerImageMetadata                              | `container.image.id`                        |
| workloadmeta.KindECSTask                                             | `aws.ecs.task.arn`                          |
| workloadmeta.KindKubernetesDeployment                                | `k8s.deployment.name`, `k8s.namespace.name` |
| workloadmeta.KindKubernetesMetadata                                  | `k8s.namespace.name`, `k8s.node.name`       |
| workloadmeta.KindKubernetesPod                                       | `k8s.pod.uid`                               |
| workloadmeta.KindProcess                                             | `process.pid`                               |

## List of Kubernetes Tags

For the full list of Kubernetes Datadog Tags added by the infra attributes processor, see [comp/core/tagger/tags/tags.go](../../../../../../comp/core/tagger/tags/tags.go).
