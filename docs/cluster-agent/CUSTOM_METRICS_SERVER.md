# Datadog Cluster Agent | Custom & External Metrics Provider 

The Datadog Cluster Agent is a **beta** feature and the Custom Metrics Server is in **alpha** if you are facing any issues please reach out to our [support team](http://docs.datadoghq.com/help).

## Introduction

The Horizontal Pod Autoscaling feature was introduced in [Kubernetes v1.2](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#before-you-begin).
It allows users to autoscale off of basic metrics like `CPU`, but requires a resource called metrics-server to run along side your application.
As of Kubernetes v1.6, it is possible to autoscale off of [custom metrics](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-custom-metrics).
Custom metrics are user defined and are collected from within the cluster.
As of Kubernetes v1.10, support for external metrics was introduced so users can autoscale off of any metric from outside the cluster that is collected for you by Datadog.

The custom and external metric providers, as opposed to the metrics server, are resources that have to be implemented and registered by the user.

As of v1.0.0, the Custom Metrics Server in the Datadog Cluster Agent implements the External Metrics Provider interface for External Metrics.
This walkthrough explains how to set it up and how to autoscale your Kubernetes workload based off of your Datadog metrics.

## Requirements

1. Running Kubernetes >v1.10 in order to be able to register the External Metrics Provider resource against the API Server.
2. Having the Aggregation layer enabled, refer to the [Kubernetes aggregation layer configuration documentation](https://kubernetes.io/docs/tasks/access-kubernetes-api/configure-aggregation-layer/) to learn how to enable it.

## Walkthrough

### Preliminary disclaimer

Autoscaling over External Metrics does not require the Node Agent to be running, you only need the metrics to be available in your Datadog account. Nevertheless, for this walkthrough, we autoscale an NGINX Deployment based off of NGINX metrics, collected by a Node Agent.

If you want to proceed further, we are assuming that:
1. You have Node Agents running (ideally from a DaemonSet) with the Autodiscovery process enabled and functional.
2. Agents are configured to securely (see [the security premise section](/Dockerfiles/cluster-agent/README.md#security-premise) of the official documentation) communicate with the Datadog Cluster Agent.

The second point is not mandatory, but it enables the enrichment of the metadata collected by the Node Agents. 

### Spinning up the Datadog Cluster Agent

In order to spin up the Datadog Cluster Agent, create the appropriate RBAC rules.
The Datadog Cluster Agent is acting as a proxy between the API Server and the Node Agent, to this extent it needs to have access to some cluster level resources.

`kubectl apply -f manifests/cluster-agent/rbac-cluster-agent.yaml`

```
clusterrole.rbac.authorization.k8s.io "dca" created
clusterrolebinding.rbac.authorization.k8s.io "dca" created
serviceaccount "dca" created
```

Then create the Datadog Cluster Agent and its services.
Start by adding your `<API_KEY>` and `<APP_KEY>` in the Deployment manifest of the Datadog Cluster Agent.
Then enable the HPA Processing by setting the `DD_EXTERNAL_METRICS_PROVIDER_ENABLED` variable to true.
Finally, spin up the resources:

- `kubectl apply -f manifests/cluster-agent/cluster-agent.yaml`
- `kubectl apply -f manifests/cluster-agent/datadog-cluster-agent_service.yaml`
- `kubectl apply -f manifests/cluster-agent/hpa-example/cluster-agent-hpa-svc.yaml`

Note that the first service is used for the communication between the Node Agents and the Datadog Cluster Agent but the second is used by Kubernetes to register the External Metrics Provider.

At this point you should be having:

```
PODS:

NAMESPACE     NAME                                     READY     STATUS    RESTARTS   AGE
default       datadog-cluster-agent-7b7f6d5547-cmdtc   1/1       Running   0          28m

SVCS:

NAMESPACE     NAME                  TYPE        CLUSTER-IP        EXTERNAL-IP   PORT(S)         AGE
default       datadog-custom-metrics-server   ClusterIP   192.168.254.87    <none>        443/TCP         28m
default       datadog-cluster-agent           ClusterIP   192.168.254.197   <none>        5005/TCP        28m

```

### Register the External Metrics Provider

Once the Datadog Cluster Agent is up and running, register it as an External Metrics Provider, via the service exposing the port 443.

To do so, apply the following RBAC rules:

`kubectl apply -f manifest/hpa-example/rbac-hpa.yaml`

```
clusterrolebinding.rbac.authorization.k8s.io "system:auth-delegator" created
rolebinding.rbac.authorization.k8s.io "dca" created
apiservice.apiregistration.k8s.io "v1beta1.external.metrics.k8s.io" created
clusterrole.rbac.authorization.k8s.io "external-metrics-reader" created
clusterrolebinding.rbac.authorization.k8s.io "external-metrics-reader" created
```
 
Once you have the Datadog Cluster Agent running and the service registered, create an HPA manifest and let the Datadog Cluster Agent pull metrics from Datadog.

## Running the HPA
<a name="running-the-hpa"></a>
At this point, you should be seeing:
```
PODS

NAMESPACE     NAME                                     READY     STATUS    RESTARTS   AGE
default       datadog-agent-4c5pp                      1/1       Running   0          14m
default       datadog-agent-ww2da                      1/1       Running   0          14m
default       datadog-agent-2qqd3                      1/1       Running   0          14m
[...]
default       datadog-cluster-agent-7b7f6d5547-cmdtc   1/1       Running   0          16m
```

Now is time to create a Horizontal Pod Autoscaler manifest. If you take a look at [the hpa-manifest.yaml file](/manifest/cluster-agent/hpa-example/hpa-manifest.yaml), you should see:
- The HPA is configured to autoscale the Deployment called nginx
- The maximum number of replicas created is 5 and the minimum is 1
- The metric used is `nginx.net.request_per_s` and the scope is `kube_container_name: nginx`. Note that this metric format corresponds to the Datadog one.

Every 30 seconds (this can be configured) Kubernetes queries the Datadog Cluster Agent to get the value of this metric and autoscales proportionally if necessary.
For advanced use cases, it is possible to have several metrics in the same HPA, as you can see [in the Kubernetes horizontal pod autoscale documentation](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-multiple-metrics) the largest of the proposed value will be the one chosen.

Now, let's create the NGINX deployment:

`kubectl apply -f manifests/cluster-agent/hpa-example/nginx.yaml`

Then, apply the HPA manifest.

`kubectl apply -f manifests/cluster-agent/hpa-example/hpa-manifest.yaml`

You should be seeing your nginx pod running with the corresponding service:

```
POD:

default       nginx-6757dd8769-5xzp2                   1/1       Running   0          3m

SVC:

NAMESPACE     NAME                  TYPE        CLUSTER-IP        EXTERNAL-IP   PORT(S)         AGE
default       nginx                 ClusterIP   192.168.251.36    none          8090/TCP        3m


HPAS:

NAMESPACE   NAME       REFERENCE          TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
default     nginxext   Deployment/nginx   0/9 (avg)       1         3         1        3m

```

### Stressing your service

At this point, the set up is ready to be stressed.
As a result of the stress Kubernetes will autoscale the NGINX pods.

If you curl the IP of the NGINX service as follows:
`curl <nginx_svc>:8090/nginx_status`, you should receive an output like:

```
$ curl 192.168.254.216:8090/nginx_status

Active connections: 1 
server accepts handled requests
 1 1 1 
Reading: 0 Writing: 1 Waiting: 0 
```

Behind the scenes, the number of request per second also increased. 
This metric is being collected by the Node Agent, as it autodiscovered the NGINX pod through its annotations (for more information on how autodiscovery works, see our [autodiscovery documentation](https://docs.datadoghq.com/agent/autodiscovery/#template-source-kubernetes-pod-annotations)).
Therefore, if you stress it, you should see the uptick in your Datadog app.
As you referenced this metric in your HPA manifest, the Datadog Cluster Agent is also pulling its latest value every 20 seconds.
Then, as Kubernetes queries the Datadog Cluster Agent to get this value, it notices that the number is going above the threshold and autoscales accordingly.

Let's do it!

Run `while true; do curl <nginx_svc>:8090/nginx_status; sleep 0.1; done`
And you should soon see the number of requests per second spiking, going above 9, the threshold over which the NGINX pods autoscale.
Then, you should see new NGINX pods being created:

```
PODS:

NAMESPACE     NAME                                     READY     STATUS    RESTARTS   AGE
default       datadog-cluster-agent-7b7f6d5547-cmdtc   1/1       Running   0          9m
default       nginx-6757dd8769-5xzp2                   1/1       Running   0          2m
default       nginx-6757dd8769-k6h6x                   1/1       Running   0          2m
default       nginx-6757dd8769-vzd5b                   1/1       Running   0          29m

HPAS:

NAMESPACE   NAME       REFERENCE          TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
default     nginxext   Deployment/nginx   30/9 (avg)     1         3         3         29m

```

Voilà.

### Troubleshooting

- Make sure you have the Aggregation layer and the certificates set up as per the requirements section.
- Always make sure the metrics you want to autoscale on are available.
As you create the HPA, the Datadog Cluster Agent parses the manifest and queries Datadog to try to fetch the metric.
If there is a typographic issue with your metric name or if the metric does not exist within your Datadog application the following error is raised:
```
2018-07-03 13:47:56 UTC | ERROR | (datadogexternal.go:45 in queryDatadogExternal) | Returned series slice empty
```

You can run the `datadog-cluster-agent status` command to see the status of the External Metrics Provider process:

```
  Custom Metrics Provider
  =======================
  External Metrics
  ================
    ConfigMap name: datadog-hpa
    Number of external metrics detected: 2
```
Note: Errors with the External Metrics Provider process are displayed with this command.
If you want a more verbose output, run the flare command:
`datadog-cluster-agent flare`.
The flare command generates a zip file containing the `custom-metrics-provider.log` where you can see an output as follows:
```
  Custom Metrics Provider
  =======================
  External Metrics
  ================
    ConfigMap name: datadog-hpa
    Number of external metrics detected: 2
    
    hpa:
    - name: nginxext
    - namespace: default
    labels:
    - cluster: eks
    metricName: redis.key
    ts: 1.532042322e&#43;09
    valid: false
    value: 0
    
    hpa:
    - name: nginxext
    - namespace: default
    labels:
    - dcos_version: 1.9.4
    metricName: docker.mem.limit
    ts: 1.532042322e&#43;09
    valid: true
    value: 2.68435456e&#43;08


```
 
If the metric's flag `Valid` is set to false, the metric is not considered in the HPA pipeline.

- If you see the following mesage when describing the hpa manifest
```
Conditions:
  Type           Status  Reason                   Message
  ----           ------  ------                   -------
  AbleToScale    True    SucceededGetScale        the HPA controller was able to get the target's current scale
  ScalingActive  False   FailedGetExternalMetric  the HPA was unable to compute the replica count: unable to get external metric default/nginx.net.request_per_s/&LabelSelector{MatchLabels:map[string]string{kube_container_name: nginx,},MatchExpressions:[],}: unable to fetch metrics from external metrics API: the server could not find the requested resource (get nginx.net.request_per_s.external.metrics.k8s.io)

```

Then it's likely that you don't have the proper RBAC set for the HPA.
Make sure that `kubectl api-versions` shows:
```
autoscaling/v2beta1
[...]
external.metrics.k8s.io/v1beta1 
```
The latter will show up if the Datadog Cluster Agent properly registers as an External Metrics Provider—and if you have the same service name referenced in the APIService for the External Metrics Provider, as well as the one for the Datadog Cluster Agent on port 443.
Also make sure you have created the RBAC from the Register the External Metrics Provider step.

- If you see the following error when describing the hpa manifest

```
  Warning  FailedComputeMetricsReplicas  3s (x2 over 33s)  horizontal-pod-autoscaler  failed to get nginx.net.request_per_s external metric: unable to get external metric default/nginx.net.request_per_s/&LabelSelector{MatchLabels:map[string]string{kube_container_name: nginx,},MatchExpressions:[],}: unable to fetch metrics from external metrics API: the server is currently unable to handle the request (get nginx.net.request_per_s.external.metrics.k8s.io)
```

Make sure the Datadog Cluster Agent is running, and the service exposing the port 443 which name is registered in the APIService are up.

- If you are not collecting the service tag from the Datadog Cluster Agent

Make sure the service map is available by exec'ing into the Datadog Cluster Agent pod and run:
`datadog-cluster-agent metamap`
Then, make sure you have the same secret (or a 32 characters long) token referenced in the Agent and in the Datadog Cluster Agent.
The best way to do this is to check the environment variables (just type `env` when in the Agent or the Datadog Cluster Agent pod).
Then make sure you have the `DD_CLUSTER_AGENT_ENABLED` option turned on in the Node Agent's manifest.

- Why am I not seeing the same value in Datadog and in Kubernetes?

As Kubernetes autoscales your resources the current target is weighted by the number of replicas of the scaled Deployment.
So the value returned by the Datadog Cluster Agent is fetched from Datadog and should be proportionally equal to the current target times the number of replicas. 
Example:
```
    hpa:
    - name: nginxext
    - namespace: default
    labels:
    - app: puppet
    - env: demo
    metricName: nginx.net.request_per_s
    ts: 1.532042322e&#43;09
    valid: true
    value: 2472
``` 
We fetched 2472, but the HPA indicates:
```
NAMESPACE   NAME       REFERENCE          TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
default     nginxext   Deployment/nginx   824/9 (avg)   1         3         3          41m
```
And indeed 824 * 3 replicas = 2472.

*Disclaimer*: The Datadog Cluster Agent processes the metrics set in different HPA manifests and queries Datadog to get values every 20 seconds. Kubernetes queries the Datadog Cluster Agent every 30 seconds.
Both frequencies are configurable.
As this process is done asynchronously, you should not expect to see the above rule verified at all times, especially if the metric varies.
