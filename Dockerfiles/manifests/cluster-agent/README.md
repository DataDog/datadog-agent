The kubernetes manifests found in this directory have been automatically generated
from the [helm chart `datadog/datadog`](https://github.com/DataDog/helm-charts/tree/master/charts/datadog)
version 2.9.11 with the following `values.yaml`:

```yaml
datadog:
  collectEvents: true
  processAgent:
    enabled: false
clusterAgent:
  enabled: true
  metricsProvider:
    enabled: true
```
