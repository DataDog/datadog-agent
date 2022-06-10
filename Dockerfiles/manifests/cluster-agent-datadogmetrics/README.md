The kubernetes manifests found in this directory have been automatically generated
from the [helm chart `datadog/datadog`](https://github.com/DataDog/helm-charts/tree/master/charts/datadog)
version 2.33.6 with the following `values.yaml`:

```yaml
datadog:
  collectEvents: true
  apm:
    socketEnabled: false
  processAgent:
    enabled: false
clusterAgent:
  enabled: true
  metricsProvider:
    enabled: true
    useDatadogMetrics: true
```
