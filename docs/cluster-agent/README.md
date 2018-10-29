# Datadog Cluster Agent | User documentation

The Datadog Cluster Agent is a **beta** feature, if you are facing any issues please reach out to our [support team](http://docs.datadoghq.com/help).

## Introduction

In the context of monitoring Orchestrators, solely relying on insights from the node is not enough.
Kubernetes, DCOS, Swarm etc are working at the node level but also at the cluster level.
The Datadog Agent has the capability to fully monitor a node at the system and the application level, it also gives good insights of the cluster's health.
Nevertheless, in order to separate concerns it is important to keep the Node Agent to the context of the node and have a Cluster Agent take care of the higher level.

## Monitoring a cluster

At the cluster level, users should care about the events, the orchestrator's behavior as well as the cluster level metadata.
For instance, how a load balancer (a service in the context of Kubernetes) is serving a certain set of pods living on different nodes.

## Before the Datadog Cluster Agent

Node Agents would run a leader election process among each other, the leader would query the API Server on a regular basis to collect the kubernetes events.
Each Agent would query the API server on their own to get the services serving the pods on their node and map them to tag the relevant application metrics with the appropriate pod name and service associated.
The limits of such an approach are:
- Non linear increase of the load on the API Server and ETCD
- Error prone process that can lead to a duplicate collection of events. 

## Enters the Datadog Cluster Agent

The goal of the Datadog Cluster Agent is to enhance the experience of monitoring Kubernetes:

* It acts as a proxy between the API server and the Node Agent in order to separate concerns.
* It provides cluster level metadata that can only be found in the API server to the Node Agents for them to enrich the metadata of the locally collected metrics.
* It enables the collection of cluster level data such as the monitoring of services or SPOF and events. These would otherwise require a mix of [Leader Election](../../Dockerfiles/agent/README.md#leader-election) and [Autodiscovery](../../pkg/autodiscovery/README.md) to be monitored.
* It implements the [External Metrics Provider](CUSTOM_METRICS_SERVER.md) interface, enabling the users to autoscale their applications out of any metrics available in their Dataadog accounts. 

## When to use the Datadog Cluster Agent

Beyond hundred nodes hitting the API Server, can surface a non negligible impact.
We recommend using the Datadog Cluster Agent: 
- To alleviate the impact of the Agents on your infrastructure.
- To isolate the Node Agent to the node, reducing the RBAC rules to solely read metrics and metadata from the kubelet.
- To leverage the Horizontal Pod Autoscaling feature with custom metrics of Kubernetes, use the Datadog Cluster Agent to pull metrics from Datadog.
You will be able to autoscale your deployments based off of any metric available in your Datadog account.
Refer to [the dedicated guide](CUSTOM_METRICS_SERVER.md) to get more details about this feature.


## Limitations of the Datadog Cluster Agent

The Datadog Cluster Agent implements a Go HTTP server (from `http/net`) to expose its API.
This implementations is [largely sufficient](https://github.com/valyala/fasthttp#http-server-performance-comparison-with-nethttp) as the Datadog Cluster Agent should only be receiving calls from up to 5K nodes that will be made every minute by default.
Load testing the Datadog Cluster Agent, there were no problems handling 200 rq/s for an extended period of time. We would still recommend running 3 replicas of the Datadog Cluster Agent for infrastructures beyond a thousand nodes with the Agent.

Only the External Metrics Provider is implemented as of v1.0.0, hence you must be running Kubernetes v1.10+ to leverage this feature.
