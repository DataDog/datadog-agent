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

## Provided and enhanced config

`GetProvidedConf` and `GetEnhancedConf` return the string representation of the user provided and autoconfigured conf respectively. Currently, these APIs have two limitations:
- They do not redact sensitive data
- They do not provide the effective config (including defaults...etc)
