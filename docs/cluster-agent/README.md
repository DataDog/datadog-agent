# Datadog Cluster Agent - DCA | User documentation

The DCA is a **beta** feature, if you are facing any issues please reach out to our [support team](http://docs.datadoghq.com/help)

## Introduction

In the context of monitoring Orchestrators, solely relying on insight from the node is not enough.
Kubernetes, DCOS, Swarm etc are working at the node level but also at the cluster level.
The Datadog agent has the capability to fully monitor a node at the system and the application level, it also gives good insights of the cluster's health.
Nevertheless, in order to separate concerns it is important to keep the node agent to the context of the node and have a cluster agent take care of the higher level.

## Monitoring a cluster

From the cluster level, users should care about the events as well as the cluster level metadata.
For instance, the services serving a certain pod living on a node.

## Before the DCA

Node agents would run a leader election process among each other, the leader would query the API Server on a regular basis to collect the kubernetes events.
Each agent would query the API server on their own to get the services serving the pods on their node and map them to tag the kubernetes metrics with the appropriate pod name and service associated.
The limits of such an approach are:
- Non linear increase of the load on the API Server and ETCD
- Error prone process that can lead to a duplicate collection of events. 

## Enters the DCA

The goal of the DCA is to enhance the experience of monitoring Kubernetes:

* It acts as a proxy between the API server and the node agent in order to separate concerns.
* It provides cluster level metadata that can only be found in the API server to the node agents for them to enrich the metadata of the locally collected metrics.
* It enables a cluster level collection of data such as the monitoring of services or SPOF and events that could otherwise than via a mix of [Leader Election] and [Autodiscovery] could not be monitored.

## When to use the DCA

Beyond a few hundred nodes hitting the API Server, can surface a non negligible impact.
We recommend using the DCA should you want to alleviate the impact of the agents on your infrastructure and continue getting the same experience.
Furthermore, should you want to isolate the node agent to the node, reducing the RBAC rules to solely read metrics and metadata from the kubelet.

## Limitations of the DCA

The DCA implements a go HTTP server (from http/net) to expose it's API.
This implementations is [largely sufficient](https://github.com/valyala/fasthttp#http-server-performance-comparison-with-nethttp) as the DCA should only be receiving calls from up to 5K nodes that will be made every 5 minutes by default.
Load testing the DCA, there were no problems handling 200 rq/s for an extended period of time. We would still recommend running 3 replicas of the DCA for infrastructures beyond 1 thousand nodes with the agent.

## Poka yoke

The DCA integrates in complex distributed architectures to enhance and improve an overall system observability.
Hence its mission is pretty critical and needs to have many processes in place to ensure it works seamlessly. 
As well as provide good tooling to get a clear picture of the cluster and should something go wrong, allow for a user to easily troubleshoot and identify issues.
As the DCA works hands in hands with the node agent, we have tried to make its usage fool proof and document the lifecycle.
To further read about the Lifecycle, please refer to the dedicated documentation [here](GETTING_STARTED.md).
Should you have issues with the DCA and the Troubleshooting section don't help, reach out to our [support team](mailto:support@datadoghq.com)  
