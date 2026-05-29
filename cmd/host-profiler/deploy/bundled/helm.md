# Deploying Bundled: Using Helm Charts

## Prerequisite

Deploy the Datadog Agent using Helm. See the [installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

1. Add to your `values.yaml`:

```yaml
datadog:
  hostProfiler:
    enabled: true
    image: "<IMAGE_REPOSITORY>:<IMAGE_TAG>"
```

2. Upgrade:

```shell
helm upgrade --install datadog datadog/datadog \
  -f values.yaml \
  -n datadog
```
