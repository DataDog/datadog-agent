# Datadog Distribution of the OpenTelemetry Collector

## Overview

The Datadog Distribution of the OpenTelemetry Collector provides a streamlined and optimized way to collect, process, and export observability data (traces, metrics, and logs) from your applications and infrastructure directly to Datadog. Built upon the robust and extensible OpenTelemetry Collector, this distribution offers Datadog-specific configurations and components out-of-the-box, simplifying your telemetry pipeline setup.

This project empowers users to leverage the vendor-agnostic OpenTelemetry standard while benefiting from Datadog's powerful analytics, dashboards, and alerting capabilities. It's an ideal solution to support your OpenTelemetry observability needs while remaining fully integrated with the Datadog Agent and its facilities.

## Key Features

- Seamless Datadog Integration: Pre-configured with the Datadog Exporter for easy transmission of traces, metrics, and logs to your Datadog account.
- Optimized Defaults: Includes sensible default configurations for common processors like batch, memory_limiter, and resource detection, tailored for Datadog best practices.
- OpenTelemetry Protocol (OTLP) Support: Ready to receive telemetry data via OTLP over gRPC and HTTP, making it compatible with all OpenTelemetry SDKs.
- Extensible: As a distribution of the OpenTelemetry Collector, it retains full extensibility, allowing you to add any other OpenTelemetry receivers, processors, or exporters as needed.
- Kubernetes-Native: Specifically designed and optimized for deployment within Kubernetes environments.
- Complementary to Datadog Agent: Works effectively alongside the Datadog Agent, allowing you to choose the best collection strategy for different types of telemetry.

## Getting Started

### Prerequisites

At the time of this writing only Kubernetes and daemonset (ie. [agent](deployments) in OTel lingo) deployments are supported.

Before you begin, ensure you have:
- A Datadog API Key. You can find or generate one in your Datadog Organization Settings.
- Access to a Kubernetes cluster (version 1.18+ recommended).
- `kubectl` configured to interact with your Kubernetes cluster.

### Deployment

The Datadog Distribution of the OpenTelemetry Collector is deployed as an additional container within your Datadog Agent Kubernetes deployment. Both Helm and the Datadog Operator are officially supported to deploy the Datadog Agent with DDOT.

For detailed instructions on how to deploy DDOT in your environment please check our [official docs](https://docs.datadoghq.com/opentelemetry/setup/ddot_collector/install/?tab=datadogoperator).

### Configuration

The supported deployment methods, Datadog Helm and Operator, provide a sample OpenTelemetry Collector configuration that you can use as a starting point with some pre-defined defaults. This section walks you through the predefined pipelines and included OpenTelemetry components:

```
extensions:
  health_check:
    endpoint: localhost:13133
  pprof:
    endpoint: localhost:1777
  zpages:
    endpoint: localhost:55679
  ddflare:
    endpoint: localhost:7777


receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
    # Collect own metrics
  prometheus:
    config:
      scrape_configs:
      - job_name: 'otel-collector'
        fallback_scrape_protocol: PrometheusText0.0.4
        metric_name_validation_scheme: legacy
        scrape_interval: 60s
        scrape_protocols:
          - PrometheusText0.0.4
        static_configs:
        - targets: ['0.0.0.0:8888']
        metric_relabel_configs:
        - source_labels: [__name__]
          regex: ".*grpc_io.*"
          action: drop
exporters:
  datadog:
    hostname: "otelcol-docker"
    api:
      key: ${env:DD_API_KEY}
      site: ${env:DD_SITE}
processors:
  infraattributes:
  batch:
  # using the sampler
  probabilistic_sampler:
    sampling_percentage: 30
connectors:
  # Use datadog connector to compute stats for pre-sampled traces
  datadog/connector:
    traces:
      compute_stats_by_span_kind: true
      peer_tags_aggregation: true
service:
  extensions: [health_check, pprof, zpages, ddflare]
  pipelines:
    traces: # this pipeline computes APM stats
      receivers: [otlp]
      processors: [batch]
      exporters: [datadog/connector]
    traces/sampling: # this pipeline uses sampling and sends traces
      receivers: [otlp]
      processors: [probabilistic_sampler, infraattributes,batch]
      exporters: [datadog]
    metrics:
      receivers: [otlp, datadog/connector, prometheus]
      processors: [infraattributes,batch]
      exporters: [datadog]
    logs:
      receivers: [otlp]
      processors: [infraattributes, batch]
      exporters: [datadog]
```

Please note that you con also bring your OpenTelemetry Collector configuration with your predefined pipelines. DDOT will enrich the provided configuration to seamlessly integrate with the Datadog Agent and Fleet Automation, and enable relevant troubleshooting and debug options.

### BYOC

DDOT ships with a curated and pre-defined set of components defined in the [manifest](https://github.com/DataDog/datadog-agent/blob/main/comp/otelcol/collector-contrib/impl/manifest.yaml) file. We understand that customer's particular use-cases may require additional components not shipping by default. To address these cases we have introduced the BYOC (Bring Your Own Components) workflow.

To learn more about how to build DDOT with support for your custom set of components please read our [documentation](https://docs.datadoghq.com/opentelemetry/setup/ddot_collector/custom_components/).


## Contributing

We welcome and appreciate contributions to this project!

If you're interested in improving the Datadog Distribution of the OpenTelemetry Collector, please review our [CONTRIBUTING.md](https://github.com/DataDog/datadog-agent/blob/main/docs/public/guidelines/contributing.md) guide.

Of course, contributions can also be made upstream in both the [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) and [OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib) if changes are required to the underlying framework.

## Support

For any questions, issues, or assistance with the Datadog Distribution of the OpenTelemetry Collector:
- GitHub Issues: [Open](https://github.com/DataDog/datadog-agent/issues) an issue on this repository for bugs or feature requests.
- Datadog Documentation: Consult the [Datadog OpenTelemetry documentation]() for integration specifics.
- Datadog Support: For direct assistance, reach out to Datadog Support.

## License

This project is licensed under the Apache 2.0 License. See the [LICENSE](https://raw.githubusercontent.com/DataDog/datadog-agent/refs/heads/main/LICENSE) file for more details.
