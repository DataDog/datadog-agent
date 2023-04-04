The Kubernetes manifests found in this directory are automatically generated from our [Datadog Helm chart](https://github.com/helm/charts/tree/master/stable/datadog) with the [`generate.sh`](generate.sh) script.

If the manifests in this repository had to be updated, the ones in the [documentation repository](https://github.com/DataDog/documentation/tree/master/static/resources/yaml) should probably also be updated.

The manifests found here do not aim at being exhaustive in term of possible configurations.
Instead, they aim at being examples that can be further customized.

Several examples are provided:
* [`agent-only`](agent-only) Contains only the DaemonSet with the core agent;
* [`all-containers`](all-containers) Contains the DaemonSet with the core agent, the trace agent, the process agent and system-probe;
* [`cluster-agent`](cluster-agent) Contains the agent DaemonSet as well as the cluster agent;
* [`cluster-agent-datadogMetrics`](cluster-agent) Contains the agent DaemonSet as well as the cluster agent with DatadogMetric CRD support;
* [`cluster-checks-runners`](cluster-checks-runners) Contains the agent DaemonSet as well as the cluster agent and the cluster check runners.

**NOTE:** Manifests are generated in the `default` namespace. You will need to modify `namespace: default` occurrences if you are installing in another namespace.


## Running generate.sh
In order to run `generate.sh`, you must have [helm](https://github.com/helm/helm) and [yq](https://github.com/mikefarah/yq) installed.
The `datadog` helm repo must point to our public helm repository:
```
$ helm repo list
NAME       	URL
internal-dd	gs://datadog-helm-prod
datadog    	https://helm.datadoghq.com
```

If your `datadog` helm repo points to the internal one:
```
NAME       	URL
datadog     gs://datadog-helm-prod
```

Run the following:
```
$ helm repo remove datadog
"datadog" has been removed from your repositories
$ helm repo add internal-dd gs://datadog-helm-prod
"internal-dd" has been added to your repositories
$ helm repo add datadog https://helm.datadoghq.com
"datadog" has been added to your repositories
$ ./generate.sh 
...
```
