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

**NOTE:** Manifests are generated in the `default` namespace. You will need to modify `namespace: default` occurences if you are installing in another namespace.
