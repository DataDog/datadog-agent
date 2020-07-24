# Serverless Agent

## Build

```
GOOS=linux go build -tags serverless -o datadog-agent
```

## Serverless environment configuration

  - `DD_API_KEY` should be set to provide the API key to use.
  - `DD_SITE` (optional)
  - `DD_LOG_LEVEL` (optional)

## Limitations

  - Doesn't support the APIkey provided to the runtime for now (using the lambda library),
    it must be set in the DD_API_KEY environment var.
