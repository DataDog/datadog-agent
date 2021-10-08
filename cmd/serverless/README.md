# Serverless Agent

## Build

```
GOOS=linux go build -tags serverless -o datadog-agent
```

## Serverless environment configuration

  - `DD_API_KEY` - should be set to provide the API key to use, or `DD_KMS_API_KEY` to use a KMS encrypted API key or `DD_API_KEY_SECRET_ARN` to use a secret set in Secrets Manager for the API key.
  - `DD_SITE` (optional)
  - `DD_LOG_LEVEL` (optional)
  - `DD_LOGS_ENABLED` (optional) - send function logs to Datadog. If false, logs will still be collected to generate metrics, but will not be sent.

## Configuration file

Using a configuration file is supported (note that environment variable settings
override the value from the configuration file): you only have to create a `datadog.yaml`
file in the root of your lambda function.

### Logs filtering and scrubbing support

The Serverless Agent support using the different logs filtering / scrubbing features of the
Datadog Agent. All you have to do is to set `processing_rules` in the `logs_config`
field of your configuration.

Please refer to the public documentation for all filtering and scrubbing features:

* https://docs.datadoghq.com/agent/logs/advanced_log_collection/?tab=configurationfile#global-processing-rules
* https://docs.datadoghq.com/agent/logs/advanced_log_collection/?tab=configurationfile#filter-logs

## Limitations

- Doesn't support the API key provided to the runtime for now (using the Lambda library),
  it must be set in the DD_API_KEY environment var or in the configuration file.
