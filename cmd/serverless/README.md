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

For example, if you want to redact some parts of the logs collected by the Datadog Lambda
Extension / Serverless Agent, you can use this syntax:

```
logs_config:
    processing_rules:
        # replace sequences in the logs
        - type: mask_sequences
          name: mask_user_email
          replace_placeholder: "MASKED_EMAIL@datadoghq.com"
          pattern: \w+@datadoghq.com
        - type: mask_sequences
          name: mask_credit_cards
          replace_placeholder: "[masked_credit_card]"
          pattern: (?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})
```

Another example, if you want to exclude some logs line containing a pattern:

```
logs_config:
    processing_rules:
    - type: exclude_at_match
      name: exclude_datadoghq_users
      # regexp can be anything
      pattern: \w+@datadoghq.com
```

Please refer to the public documentation for all filtering and scrubbing features:

* https://docs.datadoghq.com/agent/logs/advanced_log_collection/?tab=configurationfile#global-processing-rules
* https://docs.datadoghq.com/agent/logs/advanced_log_collection/?tab=configurationfile#filter-logs

## Limitations

- Doesn't support the API key provided to the runtime for now (using the Lambda library),
  it must be set in the DD_API_KEY environment var or in the configuration file.
