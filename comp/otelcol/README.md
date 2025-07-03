# Datadog Distribution of the OpenTelemetry Collector

<p align="center">
  <img src="https://github.com/user-attachments/assets/e1397db0-343e-435b-8232-606adc270ed5" alt="Datadog Logo"/>
  <img src="https://github.com/user-attachments/assets/e6629ec0-2d19-44d1-b951-7af74088a257" alt="OpenTelemetry Logo"/>
</p>

## Overview

The Datadog Distribution of the OpenTelemetry (DDOT) Collector provides a streamlined and optimized way to collect, process, and export observability data (traces, metrics, and logs) from your applications and infrastructure directly to Datadog. Built upon the robust and extensible OpenTelemetry Collector, this distribution offers Datadog-specific configurations and components out-of-the-box, simplifying your telemetry pipeline setup.

This project empowers users to leverage the vendor-agnostic OpenTelemetry standard while benefiting from Datadog's powerful analytics, dashboards, and alerting capabilities. It's an ideal solution to support your OpenTelemetry observability needs while remaining fully integrated with the Datadog Agent and its facilities.

![ddot-collector](https://github.com/user-attachments/assets/e286805f-df95-42d9-8a26-16ac8fd44567)


## Key Features

- **Seamless Datadog Integration**: Pre-configured with the Datadog Exporter for easy transmission of traces, metrics, and logs to your Datadog account.
- **Optimized Defaults**: Includes sensible default configurations for common components like the DD Exporter, Batch Processor, and Datadog connector, tailored for Datadog best practices.
- **OpenTelemetry Protocol (OTLP) Support**: Ready to receive telemetry data via OTLP over gRPC and HTTP, making it compatible with all OpenTelemetry SDKs.
- **Extensible**: As a distribution of the OpenTelemetry Collector, it retains full extensibility, allowing you to add any other OpenTelemetry receivers, processors, or exporters as needed.
- **Kubernetes-Native**: Specifically designed and optimized for deployment within Kubernetes environments.
- **Complementary to Datadog Agent**: Works effectively alongside the Datadog Agent, allowing you to choose the best collection strategy for different types of telemetry.

## Getting Started

### Prerequisites

**Supported deployments** :
- Kubernetes (v1.18+ recommended)
- DaemonSet ([agent pattern](https://opentelemetry.io/docs/collector/deployment/agent/))

Before you begin, ensure you have:
- A Datadog API Key. You can find or generate one in your Datadog Organization Settings.
- `kubectl` configured to interact with your Kubernetes cluster.

### Deployment

The Datadog Distribution of the OpenTelemetry Collector is deployed as an additional container within your Datadog Agent Kubernetes deployment. Both Helm and the Datadog Operator are officially supported to deploy the Datadog Agent with DDOT.

For detailed instructions on how to deploy DDOT in your environment please check our [official docs](https://docs.datadoghq.com/opentelemetry/setup/ddot_collector/install/?tab=datadogoperator).

### Configuration

The supported deployment methods, Datadog Helm and Operator, provide a sample OpenTelemetry Collector configuration that you can use as a starting point with some pre-defined defaults. Below you may find this default, predefined, pipeline configuration provided with DDOT:

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

Please note that you can also bring your OpenTelemetry Collector configuration with your predefined pipelines. DDOT will enrich the provided configuration to seamlessly integrate with the Datadog Agent and Fleet Automation, and enable relevant troubleshooting and debug options.

### Custom Components

DDOT ships with a curated and pre-defined set of components defined in the [manifest](https://github.com/DataDog/datadog-agent/blob/main/comp/otelcol/collector-contrib/impl/manifest.yaml) file. We understand that customer's particular use-cases may require additional components not shipping by default. To address these cases, we support a workflow to use custom OpenTelemetry components with the Datadog Agent.

To learn more about how to build the DDOT Collector with support for your custom set of components please read our [documentation](https://docs.datadoghq.com/opentelemetry/setup/ddot_collector/custom_components/).


### Development

For developers, building and running the DDOT Collector locally is straightforward once you have the Datadog Agent development environment set up. You can use our [development documentation](https://datadoghq.dev/datadog-agent/) to learn some more about the development guidelines, and instructions on the environment and tooling setup are available in this [guide](https://datadoghq.dev/datadog-agent/setup/).

Once your development environment is set up, you can build your DDOT Collector as follows:
```
inv otel-agent.build
```

The resulting binary is dropped in the `./bin/otel-agent` directory. The default OTel configuration is dropped in `./bin/otel-agent/dist/otel-config.yaml`.

The DDOT executable takes in two arguments `--core-config <agent configuration>` and `--config <otel configuration>`:
- `--core-config`: This is your typical Datadog Agent configuration that defines some settings that are leveraged by the DDOT Collector.
- `--config`: This is the OpenTelemetry Collector configuration, with the relevant components (receivers, processors, exporters, connectors, and extensions) that define your OTel observability pipelines. This file is in the same format as the upstream Collector configuration files.


You can run your DDOT local build simply by executing the resulting binary as follows:
```
./bin/otel-agent/otel-agent run --core-config <your_datadog_agent_config>.yaml --config ./bin/otel-agent/dist/otel-config.yaml
```

Of course, feel free to provide and tweak the provided configurations as required.


## Contributing

We welcome and appreciate contributions to this project!

If you're interested in improving the Datadog Distribution of the OpenTelemetry Collector, please review our [CONTRIBUTING.md](https://github.com/DataDog/datadog-agent/blob/main/docs/public/guidelines/contributing.md) guide.

Of course, contributions can also be made upstream in both the [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) and [OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib) if changes are required to the underlying framework.

## Support

For any questions, issues, or assistance with the Datadog Distribution of the OpenTelemetry Collector:
- GitHub Issues: [Open](https://github.com/DataDog/datadog-agent/issues) an issue on this repository for bugs or feature requests.
- Datadog Documentation: Consult the [Datadog OpenTelemetry documentation](https://docs.datadoghq.com/opentelemetry/setup/ddot_collector) for integration specifics.
- Datadog Support: For direct assistance, reach out to Datadog Support.

## License

This project is licensed under the Apache 2.0 License. See the [LICENSE](https://raw.githubusercontent.com/DataDog/datadog-agent/refs/heads/main/LICENSE) file for more details.
