# Serverless Agent

## Build

```
GOOS=linux go build -tags serverless -o datadog-agent
```

Or using a script from the root of this repo

```bash
./tasks/serverless/build_layers.sh
```

## Deploy

Make sure you have aws-credentials installed in your local environment, and the
aws-cli installed.

```bash
# Publish to all runtimes/regions
./tasks/serverless/publish_layers.sh
# Publish to specific runtime/region
./tasks/serverless/publish_layers.sh us-east-1 Datadog-Extension-Python
./tasks/serverless/publish_layers.sh us-east-1
# Get all layer versions
./tasks/serverless/list_layers.sh
# Get layer for specific runtime/region
./tasks/serverless/list_layers.sh us-east-1 Datadog-Extension-Python
# Shortcut to build/publish to us-east-1, (use different aws-creds for staging).
./tasks/serverless/publish_staging.sh
```

## Serverless environment configuration

- `DD_API_KEY` should be set to provide the API key to use.
- `DD_SITE` (optional)
- `DD_LOG_LEVEL` (optional)

## Limitations

- Doesn't support the APIkey provided to the runtime for now (using the lambda library),
  it must be set in the DD_API_KEY environment var.
=======
## Serverless environment configuration

  - `DD_API_KEY` should be set to provide the API key to use.
  - `DD_SITE` (optional)
  - `DD_LOG_LEVEL` (optional)

## Limitations

  - Doesn't support the APIkey provided to the runtime for now (using the lambda library),
    it must be set in the DD_API_KEY environment var.