# Datadog Cluster Agent (DCA) | Getting Started

The DCA is a **beta** feature, if you are facing any issues please reach out to our [support team](http://docs.datadoghq.com/help)

## Introduction

The Horizontal Pod Autoscaling feature has been introduced in [Kubernetes v1.2](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#before-you-begin).
It allowed users to autoscale off of basic metrics like CPU, but required a resource called the metrics-server to run along side your application.
As of Kubernetes v1.6, it is possible to autoscale off of [custom metrics](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-custom-metrics).
Custom metrics are user defined and are collected from within the cluster.
As of Kubernetes v1.10, support for external metrics was introduced so users can autoscale off of any metric from outside the cluster that is collected for you by Datadog.

The custom and external metric provider as opposed to the metrics server, are resources that have to be implemented and registered by the user.

As of v1.0.0, the DCA implements the External Metrics Provider interface.
This walkthrough will explain how to set it up and how it can help you autoscale your Kubernetes workload based off of your Datadog metrics.


## Requirements

1. Have Kubernetes v1.10, this is so the External Metrics Provider resource can be registered against the API Server.
2. You will need to have the Aggregation layer enabled as well, please refer to the official guide:
https://kubernetes.io/docs/tasks/access-kubernetes-api/configure-aggregation-layer/

## Walkthrough

### Preliminary disclaimer

Autoscaling over External Custom Metrics does not require the node agent to be running. Nevertheless, for the sake of this walkthrough, we will autoscale an nginx deployment based off of nginx metrics, collected by a node agent.
To this extent, should you want to further proceed, we are assuming that:
1. You have node agents running (ideally from a DaemonSet) with the Autodiscovery process enabled and functional.
2. Agents are configured to securely (see [this section](/Dockerfiles/cluster-agent/README.md#security-premise) of the official documentation) communicate with the DCA.

The Second point is not mandatory, but it enables the enrichment of the metadata collected by the node agents. 

### Spinning up the DCA

In order to spin up the DCA, you will need to create the appropriate RBAC rules.
The DCA is acting as a proxy between the API Server and the node agent, to this extent it will need to have access to some cluster level resources.

`kubectl apply -f manifests/cluster-agent/rbac-cluster-agent.yaml`

```
clusterrole.rbac.authorization.k8s.io "dca" created
clusterrolebinding.rbac.authorization.k8s.io "dca" created
serviceaccount "dca" created
```

Then you will need to create the DCA and its services.
Start by adding your API_KEY and APP_KEY in the deployment manifest of the DCA.
Then enable the HPA Processing by setting the `DD_ENABLE_HPA` variable to true.
Finally, spin up the resources:

`kubectl apply -f manifests/cluster-agent/cluster-agent.yaml`
`kubectl apply -f manifests/cluster-agent/datadog-cluster-agent_service.yaml`
`kubectl apply -f manifests/cluster-agent/hpa-example/cluster-agent-hpa-svc.yaml`

Note that the first service is used for the communication between the node agents and the DCA but the second is used by Kubernetes to register the External Metrics Provider.

At this point you should be having:

```
PODS:

NAMESPACE     NAME                                     READY     STATUS    RESTARTS   AGE
default       datadog-cluster-agent-7b7f6d5547-cmdtc   1/1       Running   0          28m

SVCS:

NAMESPACE     NAME                  TYPE        CLUSTER-IP        EXTERNAL-IP   PORT(S)         AGE
default       datadog-custom-metrics-server   ClusterIP   192.168.254.87    <none>        443/TCP         28m
default       dca                   ClusterIP   192.168.254.197   <none>        5005/TCP        28m

```

### Register the External Metrics Provider

Once the DCA is up and running, you can register it as an External Metrics Provider, via the service exposing the port 443.

To do so, simply apply the following RBAC rules:

`kubectl apply -f manifest/hpa-example/rbac-hpa.yaml`

```
clusterrolebinding.rbac.authorization.k8s.io "system:auth-delegator" created
rolebinding.rbac.authorization.k8s.io "dca" created
apiservice.apiregistration.k8s.io "v1beta1.external.metrics.k8s.io" created
clusterrole.rbac.authorization.k8s.io "external-metrics-reader" created
clusterrolebinding.rbac.authorization.k8s.io "external-metrics-reader" created
```
 
Once you have the DCA running and the service registered, you can create an HPA manifest and let the DCA pull metrics from Datadog.
The following part of the walkthrough explains how you can set up your agents in order to collect metrics from the applications running on your cluster.
Those metrics will then be available for you to autoscale the resources of your cluster, via the DCA.
If your agents are already instrumented and configured to communicate with the DCA, you can directly jump to the [running the hpa]() section.


## Running the HPA

At this point, you should be seeing:
```
PODS

NAMESPACE     NAME                                     READY     STATUS    RESTARTS   AGE
default       datadog-agent-4c5pp                      1/1       Running   0          14m
default       datadog-agent-ww2da                      1/1       Running   0          14m
default       datadog-agent-2qqd3                      1/1       Running   0          14m
[...]
default       datadog-cluster-agent-7b7f6d5547-cmdtc   1/1       Running   0          16m
default       nginx-6757dd8769-5xzp2                   1/1       Running   0          3m

```

Now is time to create a Horizontal Pod Autoscaler manifest. If you take a look at /manifest/cluster-agent/hpa-example/hpa-manifest.yaml, you will see:
- The HPA is configured to autoscale the Deployment called nginx
- The maximum number of replicas created will be 5 and the minumum is 1
- The metric used is `nginx.net.request_per_s` and the scope is `kube_container_name: nginx`. Note that this metric format corresponds to the Datadog one.

Every 30 seconds (this can be configured) Kubernetes will query the DCA to get the value of this metric and will autoscale proportionally if necessary.
For advanced use cases, it is possible to have several metrics in the same HPA, as you can see [here](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-multiple-metrics) the largest of the proposed value will be the one chosen.

Now, let's create our nginx deployment:

`kubectl apply -f manifests/cluster-agent/hpa-example/nginx.yaml`

Then, let's apply the HPA

`kubectl apply -f manifests/cluster-agent/hpa-example/hpa-manifest.yaml`

### Stressing your service

Now is time to see the magic happening!

If you curl the IP of the nginx service as follows:
`curl <nginx_svc>:8090/nginx_status`, you should receive an output like:

```
$ curl 192.168.254.216:8090/nginx_status

Active connections: 1 
server accepts handled requests
 1 1 1 
Reading: 0 Writing: 1 Waiting: 0 
```

Behind the scene, the number of request per second also increased. 
This metric is being collected by the node agent, as it autodiscovered the nginx pod through its annotations (for more information on how autodiscovery works, see our doc [here](https://docs.datadoghq.com/agent/autodiscovery/#template-source-kubernetes-pod-annotations)).
Therefore, if you stress it, you will see the uptick in your Datadog app. 
As you referenced this metric in your HPA manifest, the DCA is also pulling its latest value every 20 seconds.
Then, as Kubernetes queries the DCA to get this value, it will notice that the number is going above the threshold and will autoscale accordingly.

Let's do it!

Just run `while true; do curl <nginx_svc>:8090/nginx_status; sleep 0.1; done`
And you should soon see the number of requests per second spiking, going above 9 which is the threshold over which we want to autoscale out nginx boxes.
Then, you should see new nginx pods being created:

```
PODS:

NAMESPACE     NAME                                     READY     STATUS    RESTARTS   AGE
default       datadog-cluster-agent-7b7f6d5547-cmdtc   1/1       Running   0          9m
default       nginx-6757dd8769-5xzp2                   1/1       Running   0          2m
default       nginx-6757dd8769-k6h6x                   1/1       Running   0          2m
default       nginx-6757dd8769-vzd5b                   1/1       Running   0          29m

HPAS:

NAMESPACE   NAME       REFERENCE          TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
default     nginxext   Deployment/nginx   30/9 (avg)   1         3         3          29m

```

Voila

### Troubleshooting

- Make sure you have the Aggregation layer and the certificates set up as per the requirements section.
- Always make sure the metrics you want to autoscale on are available
As you create the HPA, the DCA will parse the manifest and query Datadog to try to fetch the metric.
If there is a typo or if the metric does not exist you will see an error such as:
```
2018-07-03 13:47:56 UTC | ERROR | (datadogexternal.go:45 in queryDatadogExternal) | Returned series slice empty
```
You can check in the Configmap used to store and share the HPA state by the DCA:
`kubectl get cm datadog-hpa -o yaml`
will yield:
```
apiVersion: v1
data:
  external.metrics.default.nginxext-nginx.net.request_per_s: '{"name":"nginx.net.request_per_s","labels":{"kube_container_name":"nginx"},"ts":1530625676,"hpa_name":"nginxext","hpa_namespace":"default","value":0,"valid":false}'
kind: ConfigMap
metadata:
  creationTimestamp: 2018-07-03T13:40:50Z
  name: datadog-hpa
  namespace: default
  resourceVersion: "1742"
```
Here you can see if the metric has a typo or check that it exists in Datadog. If the metric's flag `Valid` is set to false, the metric will not be considered in the HPA pipeline.
In this case, the scope could be incorrect, if we fix it by setting the right labels in the hpa manifest we will see:
```
data:
  external.metrics.default.nginxext-nginx.net.request_per_s: '{"name":"nginx.net.request_per_s","labels":{"app":"puppet","env":"demo"},"ts":1530625976,"hpa_name":"nginxext","hpa_namespace":"default","value":2493,"valid":true}'
```
In the ConfigMap, indicating that it works.

- If you see:
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
The latter will show up if the DCA properly registers as an External Metrics Provider.

And that you have the same service name referenced in the APIService for the External Metrics Provider and the one for the DCA serving on port 443.
Also make sure you have created the RBAC from the Register the External Metrics Provider step.

- If you see

```
  Warning  FailedComputeMetricsReplicas  3s (x2 over 33s)  horizontal-pod-autoscaler  failed to get nginx.net.request_per_s external metric: unable to get external metric default/nginx.net.request_per_s/&LabelSelector{MatchLabels:map[string]string{kube_container_name: nginx,},MatchExpressions:[],}: unable to fetch metrics from external metrics API: the server is currently unable to handle the request (get nginx.net.request_per_s.external.metrics.k8s.io)
```

Make sure the DCA is running and the service exposing the port 443 and which name is registered in the APIService are up.

- You are not collecting the service tag from the DCA
Make sure the service map is available by exec'ing into the DCA pod and run:
`datadog-cluster-agent metamap`
Then, make sure you have the same secret (or a 32 characters long) token referenced in the agent and in the DCA.
The best is to check in the environment variables (just type `env` when in the agent or the DCA pod).
Then make sure you have the `DD_CLUSTER_AGENT` option turned on in the node agent's manifest.

- Why am I not seeing the same value in Datadog and in Kubernetes ?

As Kubernetes autoscales your resources the current target will be weighted by the number of replicas of the scaled deployement.
So the value returned by the DCA is fetched from Datadog should be proportionnaly equal to the current target times the number of replicas. 
Example:
```
data:
  external.metrics.default.nginxext-nginx.net.request_per_s: '{"name":"nginx.net.request_per_s","labels":{"app":"puppet","env":"demo"},"ts":1530626816,"hpa_name":"nginxext","hpa_namespace":"default", value":2472,"valid":true}
``` 
We fetched 2472 but the HPA indicates:
```
NAMESPACE   NAME       REFERENCE          TARGETS       MINPODS   MAXPODS   REPLICAS   AGE
default     nginxext   Deployment/nginx   824/9 (avg)   1         3         3          41m
```
And indeed 824 * 3 replicas = 2472.

*Disclaimer*: The DCA will process the metrics set in different HPA manifests and will query Datadog to get values every 20 seconds. Kubernetes will query the DCA every 30 seconds.
Both frequencies are configurable.
As this process is done asynchronously, you should not expect to see the above rule verified at all times, expecially if the metric varies.
