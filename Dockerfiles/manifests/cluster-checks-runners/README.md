The kubernetes manifests found in this directory have been automatically generated
from the [helm chart `datadog/datadog`](https://github.com/DataDog/helm-charts/tree/master/charts/datadog)
version 2.4.5 with the following `values.yaml`:

```yaml
datadog:
  collectEvents: true
  processAgent:
    enabled: false
  clusterChecks:
    enabled: true
clusterAgent:
  enabled: true
  metricsProvider:
    enabled: true
clusterChecksRunner:
  enabled: true
```
