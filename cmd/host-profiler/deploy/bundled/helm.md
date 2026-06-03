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

2. To enable AppArmor (optional), load [`../apparmor-profile`](../apparmor-profile) on each node, then add to `values.yaml`:

```yaml
datadog:
  hostProfiler:
    apparmor: localhost/dd-host-profiler
```

3. Upgrade:

```shell
helm upgrade --install datadog datadog/datadog \
  -f values.yaml \
  -n datadog
```
