The `scc.yaml` manifest found in this directory has been automatically generated
from the [helm chart `datadog/datadog`](https://github.com/DataDog/helm-charts/tree/master/charts/datadog)
version 3.246.0 with the following `values.yaml`:

```yaml
datadog:
  apm:
    portEnabled: true
    socketEnabled: false

agents:
  useHostNetwork: true
  podSecurity:
    securityContextConstraints:
      create: true

clusterAgent:
  podSecurity:
    securityContextConstraints:
      create: true
```

It combines the Agent and Cluster Agent `SecurityContextConstraints` into a single file.
