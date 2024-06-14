# Converter Component

The converter contains two main responsibilities:
- Taking in a user provided configuration, and enhancing the config
- Providing an API which returns the provided and enhanced config

## Autoconfigure logic

The autoconfigure logic takes is applied within the `Convert` function. It takes in a `*confmap.Conf` and will modify it based on the logic described below.

### Extensions

The converter looks for the `pprof`, `health_check` and `zpages` extensions. If these are defined in the service pipeline, it does nothing. If any of these extensions are not defined, then it will add it's own extension config (name: `<extension_name>/dd-autoconfigured`) and add this in the services extension pipeline.  


### Infra Attributes Processor

The converter will check for any pipelines which have the dd exporter without the infraattributes processor. If it finds any matches, it will add it's own infra attributes config (name: `infraattributes/dd-autoconfigured`) and add this to each pipeline with the dd exporter. It adds this processor in last place in the processors slice.

### Prometheus Receiver

The converter will check to see if a prometheus receiver is defined which points to the service internal telemetry metrics address. It will also check that this receiver is used in the same pipeline as *all* configured datadog exporters. 

If it finds datadogexporters which are not defined in a pipeline with the prometheus receiver, it will add it's own prometheus config (name: `prometheus/dd-autoconfigured`), and then create it's own pipeline `metrics/dd-autoconfigured/<dd exporter name>` which contains the prometheus receiver and the datadog exporter.

## Provided and enhanced config

`GetProvidedConf` and `GetEnhancedConf` return the string representation of the user provided and autoconfigured conf respectively. Currently, these APIs have two limitations:
- They do not redact sensitive data
- They do not provide the effective config (including defaults...etc)
