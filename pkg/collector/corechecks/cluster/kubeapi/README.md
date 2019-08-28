# Kubernetes Api Cluster Agent Check
This repository contains the StackState implementation of the `kubernetes_apiserver.go` check.

We have refactored the kubernetes api server check into multiple smaller checks to assist with maintainability / configurability in the future.

## Kubernetes Api Check Common
This common class sets up the communication with the Kubernetes API and contains all common code for the checks.

## Kubernetes Api Events Check
This fetches all kubernetes events.

It is enabled using the: `collect_kubernetes_events` config parameter or the `STS_COLLECT_KUBERNETES_EVENTS` environment variable.

Default: **enabled**

## Kubernetes Api Metrics Check (_WIP_)
This fetches some of the metrics of the Kubernetes / Openshift cluster.

The check is enabled using the: `collect_kubernetes_metrics` config parameter or the `STS_COLLECT_KUBERNETES_METRICS` environment variable.

Default: **enabled**

## Kubernetes Api Topology Check (_WIP_)
This fetches all kubernetes topology components.

The check is enabled using the: `collect_kubernetes_topology` config parameter or the `STS_COLLECT_KUBERNETES_TOPOLOGY` environment variable.

Default: **enabled**
