# Deploying Bundled: Using Helm Charts

## Prerequisite

Deploy the Datadog Agent using Helm (chart version 3.220.0 or later). See the [installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

1. Add to your `values.yaml` (create the file if you don't have one):

```yaml
datadog:
  hostProfiler:
    enabled: true
    image: "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"
```

2. Upgrade:

```shell
helm upgrade --install datadog datadog/datadog \
  -f values.yaml \
  --reuse-values \
  -n datadog
```

The chart automatically configures all required capabilities and seccomp.

Wait for the rollout to complete:

```shell
kubectl rollout status daemonset/datadog -n datadog
```

## AppArmor (optional)

To use a custom AppArmor profile instead of `unconfined`, load [`apparmor-profile`](../apparmor-profile) on each node, then set:

```yaml
datadog:
  hostProfiler:
    apparmor: localhost/dd-host-profiler
```
