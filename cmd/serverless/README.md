# Serverless Agent

## Build

```
GOOS=linux go build -tags serverless -o datadog-agent
```

## Serverless environment configuration

  - `DD_API_KEY` should be set to provide the API key to use, or `DD_KMS_API_KEY` to use a KMS encrypted API key or `DD_API_KEY_SECRET_ARN` to use a secret set in Secrets Manager for the API key.
  - `DD_SITE` (optional)
  - `DD_LOG_LEVEL` (optional)
  - `DD_LOGS_ENABLED` (optional) to enable the logs collection if supported in the serverless environment

## Limitations

  - Doesn't support the API key provided to the runtime for now (using the Lambda library),
    it must be set in the DD_API_KEY environment var.