# Converter Component

The converter enhances the user provided configuration.

## Autoconfigure logic

The autoconfigure logic is applied within the `Convert` function. It takes in a `*confmap.Conf` and will modify it based on the logic described below.

## Feature Configuration

The converter supports selective enabling and disabling of features through the `otelcollector.converter.features` configuration option.

### Available Features

- **`infraattributes`**: Adds the infra attributes processor to pipelines with Datadog exporters
- **`prometheus`**: Adds a prometheus receiver to collect internal telemetry metrics
- **`pprof`**: Adds the pprof extension for profiling capabilities
- **`zpages`**: Adds the zpages extension for debugging information
- **`health_check`**: Adds the health check extension for health monitoring
- **`ddflare`**: Adds the Datadog flare extension for diagnostic information
- **`datadog`**: Adds the OSS Datadog extension to send Fleet Automation data
- **`opamp`**: Adds the OpAMP extension so the collector reports to an OpAMP server (standalone mode only; mutually exclusive with `datadog`)

### Configuration Examples

```yaml
# Default: all features enabled
otelcollector:
  converter:
    enabled: true
    features: ["infraattributes", "prometheus", "pprof", "zpages", "health_check", "ddflare"]

# Enable only specific features
otelcollector:
  converter:
    enabled: true
    features: ["infraattributes", "prometheus"]

# Disable all features
otelcollector:
  converter:
    enabled: true
    features: []
```

### Environment Variable
You may also set these features using an environment variable:
```bash
export DD_OTELCOLLECTOR_CONVERTER_FEATURES="infraattributes,prometheus,pprof"
```

### Extensions

The converter looks for the `pprof`, `health_check`, `zpages`, `ddflare`, `datadog` and `opamp` extensions. If these are already defined in the service pipeline, it makes no changes. If any of these extensions are not defined, it will add the extensions config (name: `<extension_name>/dd-autoconfigured`) and add the component in the services extension pipeline.

### Infra Attributes Processor

The converter will check for any pipelines which have the dd exporter without the infraattributes processor. If it finds any matches, it will add the infra attributes config (name: `infraattributes/dd-autoconfigured`) and add the processor to the pipeline. It adds the processor in last place in the processors slice.

### Prometheus Receiver

The converter will check to see if a prometheus receiver is defined which points to the service internal telemetry metrics address. It then checks that this receiver is used in the same pipeline as *all* configured datadog exporters.

If it finds datadogexporters which are not defined in a pipeline with the prometheus receiver, it adds the prometheus config (name: `prometheus/dd-autoconfigured`), and then create it's own pipeline `metrics/dd-autoconfigured/<dd exporter name>` which contains the prometheus receiver and the datadog exporter.

For any prometheus receiver collecting collector health metrics, and sending these to Datadog, it will update the job name to `datadog-agent`. This ensures the health metrics are tagged by `service:datadog-agent` and differentiable from collector health metrics.

### OpAMP Extension

The `datadog` and `opamp` extensions are mutually exclusive, and which one is injected is decided by the deployment mode:

- **Connected mode** (`otel_standalone: false`, the default): the `datadog` extension is injected. The core Datadog Agent is present and already reports to the backend.
- **Standalone mode** (`otel_standalone: true`): the `opamp` extension is injected instead, and the `datadog` extension is never injected even when `datadog` is listed in `otelcollector.converter.features`.

Both require an `api_key`; without one neither extension is injected. A user-defined `opamp` extension is reused rather than duplicated, as with the other extensions above.

The injected configuration is derived from the agent config:

```yaml
extensions:
  opamp/dd-autoconfigured:
    server:
      http:
        endpoint: https://opamp.<site>/v1/opamp
        headers:
          DD-API-KEY: <api_key>
    reports_raw_config: true
```

Two settings adjust it:

| Setting | Default | Effect |
| --- | --- | --- |
| `otelcollector.opamp.endpoint` | `""` | When set, used verbatim instead of the site-derived default. |
| `otelcollector.opamp.reports_raw_config` | `true` | Reports the unexpanded configuration, so references such as `${env:FOO}` are preserved instead of being resolved. Fields typed as `configopaque` are redacted either way. |

### API Key and API Site

If `api_key` is unset, set to an empty string or set to a secret, the converter will fetch the api key from the agent configuration. It will also fetch the the site from the agent config if unset in collector.

## Opting out of converter

It is possible to opt out of the converter by setting env var `DD_OTELCOLLECTOR_CONVERTER_ENABLED` or agent config `otelcollector.converter.enabled` to `false` (`true` by default). Please note that by doing so, you are removing functionality including flare collection from otel-agent, health metrics from collector, or infra level tagging on your telemetry data. If you want to opt out of some components, you can disable all and add the components that you require manually:

### Extensions

Please refer to the following example in order to manually set the `pprof`, `health_check`, `zpages` and `datadog` extensions: [extensions.yaml](examples/extensions.yaml). Please refer to the extensions README.md for additional information about the components:
- [pprof](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/extension/pprofextension/README.md): Enables collecting collector profiles at a defined endpoint.
- [health_check](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/extension/healthcheckextension/README.md): Enables an HTTP url that can be probed to check the status of the OpenTelemetry Collector.
- [zpages](https://github.com/open-telemetry/opentelemetry-collector/blob/main/extension/zpagesextension/README.md): Enables an extension that serves zPages, an HTTP endpoint that provides live data for debugging different components
- [ddflare](../ddflareextension/README.md): Enables otel-agent information to be collected in the datadog-agent flare.
- [datadog](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/extension/datadogextension): Enables collector configuration and build info to be viewed in both Datadog Infrastructure Monitoring and Fleet Automation.
- [opamp](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/extension/opampextension): Enables the collector to report its status and configuration to an OpAMP server.

### Prometheus Receiver

The Prometheus receiver scrapes prometheus endpoints. This is used to collect the collectors internal health metrics, by adding a job that scrapes the service telemetry metrics endpoint (configurable via `service::telemetry::metrics`). Please refer to the following example in order to manually set the prometheus receiver: [prometheus.yaml](examples/prometheus.yaml). Please refer to the receivers [README.md](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver) for additional information about the component.

### Infra Attributes Processor

The infraattributes processor is used to add infra level tags collected by the datadog-agent to your telemetry data. Please refer to the following example in order to manually set the infraattributes processor: [infraattributes.yaml](examples/infraattributes.yaml). Please refer to the processors [README.md](../otlp/components/processor/infraattributesprocessor/README.md) for additional information about the component.
