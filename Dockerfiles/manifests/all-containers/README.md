The kubernetes manifests found in this directory have been automatically generated
from the [helm chart `datadog/datadog`](https://github.com/DataDog/helm-charts/tree/master/charts/datadog)
version 3.49.6 with the following `values.yaml`:

```yaml
datadog:
  collectEvents: true
  leaderElection: true
  logs:
    enabled: true
  apm:
    enabled: true
  processAgent:
    enabled: true
  networkMonitoring:
    enabled: true
  securityAgent:
    compliance:
      enabled: true
    runtime:
      enabled: true
```
