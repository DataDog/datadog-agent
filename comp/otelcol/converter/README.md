# Converter Component

The converter:
- Enhances the user provided configuration
- Provides an API which returns the provided and enhanced configurations

## Autoconfigure logic

The autoconfigure logic is applied within the `Convert` function. It takes in a `*confmap.Conf` and will modify it based on the logic described below.

### Extensions

The converter looks for the `pprof`, `health_check`, `zpages` and `datadog` extensions. If these are already defined in the service pipeline, it makes no changes. If any of these extensions are not defined, it will add the extensions config (name: `<extension_name>/dd-autoconfigured`) and add the component in the services extension pipeline.  

### Infra Attributes Processor

The converter will check for any pipelines which have the dd exporter without the infraattributes processor. If it finds any matches, it will add the infra attributes config (name: `infraattributes/dd-autoconfigured`) and add the processor to the pipeline. It adds the processor in last place in the processors slice.

### Prometheus Receiver

The converter will check to see if a prometheus receiver is defined which points to the service internal telemetry metrics address. It then checks that this receiver is used in the same pipeline as *all* configured datadog exporters. 

If it finds datadogexporters which are not defined in a pipeline with the prometheus receiver, it adds the prometheus config (name: `prometheus/dd-autoconfigured`), and then create it's own pipeline `metrics/dd-autoconfigured/<dd exporter name>` which contains the prometheus receiver and the datadog exporter.

For any prometheus receiver collecting collector health metrics, and sending these to Datadog, it will update the job name to `datadog-agent`. This ensures the health metrics are tagged by `service:datadog-agent` and differentiable from collector health metrics.

### API Key and API Site

If `api_key` is unset, set to an empty string or set to a secret, the converter will fetch the api key from the agent configuration. It will also fetch the the site from the agent config if unset in collector.

### Datadog Connector

The converter will automatically set `datadogconnector` config `trace.span_name_as_resource_name` to true in any datadog connectors in your configuration.

## Provided and enhanced config

`GetProvidedConf` and `GetEnhancedConf` return the string representation of the user provided and autoconfigured conf respectively. Currently, these APIs have two limitations:
- They do not redact sensitive data
- They do not provide the effective config (including defaults...etc)

## Opting out of converter

It is possible to opt out of the converter by setting env var `DD_OTELCOLLECTOR_CONVERTER_ENABLED` or agent config `otelcollector.converter.enabled` to `false` (`true` by default). Please note that by doing so, you are removing functionality including flare collection from otel-agent, health metrics from collector, or infra level tagging on your telemetry data. If you want to opt out of some components, you can disable all and add the components that you require manually:

### Extensions

Please refer to the following example in order to manually set the `pprof`, `health_check`, `zpages` and `datadog` extensions: [extensions.yaml](examples/extensions.yaml). Please refer to the extensions README.md for additional information about the components:
- [pprof](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/extension/pprofextension/README.md): Enables collecting collector profiles at a defined endpoint.
- [health_check](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/extension/healthcheckextension/README.md): Enables an HTTP url that can be probed to check the status of the OpenTelemetry Collector.
- [zpages](https://github.com/open-telemetry/opentelemetry-collector/blob/main/extension/zpagesextension/README.md): Enables an extension that serves zPages, an HTTP endpoint that provides live data for debugging different components
- [datadog](../extension/README.md): Enables otel-agent information to be collected in the datadog-agent flare.

### Prometheus Receiver

The Prometheus receiver scrapes prometheus endpoints. This is used to collect the collectors internal health metrics, by adding a job that scrapes the service telemetry metrics endpoint (configurable via `service::telemetry::metrics`). Please refer to the following example in order to manually set the prometheus receiver: [prometheus.yaml](examples/prometheus.yaml). Please refer to the receivers [README.md](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver) for additional information about the component.

### Infra Attributes Processor

The infraattributes processor is used to add infra level tags collected by the datadog-agent to your telemetry data. Please refer to the following example in order to manually set the infraattributes processor: [infraattributes.yaml](examples/infraattributes.yaml). Please refer to the processors [README.md](../otlp/components/processor/infraattributesprocessor/README.md) for additional information about the component.
