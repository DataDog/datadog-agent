# Deploying Bundled: Using the Datadog Operator

## Prerequisite

Deploy the Datadog Agent using the Operator. See the [installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

Add the annotations to your existing `DatadogAgent` CR:

```shell
kubectl annotate datadogagent datadog \
  agent.datadoghq.com/host-profiler-enabled="true" \
  'experimental.agent.datadoghq.com/image-override-config={"host-profiler":{"name":"<IMAGE_REPOSITORY>:<IMAGE_TAG>"}}' \
  -n <namespace>
```

Or add them directly to your manifest and re-apply:

```yaml
metadata:
  annotations:
    agent.datadoghq.com/host-profiler-enabled: "true"
    experimental.agent.datadoghq.com/image-override-config: |
      {
        "host-profiler": {
          "name": "<IMAGE_REPOSITORY>:<IMAGE_TAG>"
        }
      }
```

The Operator rolls out a new DaemonSet revision adding the host-profiler container. Agent pods restart one node at a time.
