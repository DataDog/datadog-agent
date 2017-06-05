# Kubernetes Testing/Development

To test and develop the Kubernetes metadata provider you need a semi-real environment that provides API.

This folder provides some basic files to get you bootstrapped and running.

## Prerequisites

* You need a real/semi-real Kubernetes environment. We recommend using [minikube](https://kubernetes.io/docs/getting-started-guides/minikube/).

* You need access to the environment with `kubectl`.

* The `/path/to/go/src` in `pod-configuration.yaml` should be the full, absolute path to your GOPATH with the `datadog-agent` code.

## Setup

You will create a simple pod that deploys a dumb simple container that mounts your code code.

```
$ kubectl create -f pod-configuration.yaml
```

Wait until the pod is `Running`

```
$ kubectl get pods
```

Shell into the environment and run the code.

```
$ kubectl exec kubedev -c node -i -t -- bash

root@kubedev:/go/src# cd github.com/DataDog/datadog-agent/pkg/metadata/kubernetes/
root@kubedev:/go/src# go test
```
