# Google Cloud Launcher

## Overview

Datadog provides infrastructure monitoring, application performance monitoring, and log management in a single-pane-of-glass view so teams can scale rapidly and maintain operational excellence.

Installing the Datadog package via Google Cloud Launcher deploys the Datadog Agent on every node in your Kubernetes cluster, and configures it with a secure, RBAC-based authentication and authorization model.

## Installation

### Quick install with Google Cloud Marketplace

Get up and running with a few clicks! Install the Datadog Agent daemonset to a
Google Kubernetes Engine cluster using Google Cloud Marketplace.

Prior to that:

- Create a Datadog [account](https://www.datadoghq.com/)
- [Get your Datadog API key](https://app.datadoghq.com/organization-settings/api-keys)

Then follow the [on-screen instructions](https://console.cloud.google.com/marketplace/details/datadog-saas/datadog).

### Command line instructions

Follow these instructions to install the Datadog Agent from the command line.

#### Prerequisites (one time setup)

##### Command-line tools

Your development environment should contain the following tools:

- [gcloud](https://cloud.google.com/sdk/gcloud/)
- [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/)
- [git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)
- [helm + mpdev](https://github.com/GoogleCloudPlatform/marketplace-k8s-app-tools/blob/master/docs/tool-prerequisites.md)

##### Create a Datadog account

- Create a Datadog [account](https://www.datadoghq.com/)
- [Get your Datadog API key](https://app.datadoghq.com/organization-settings/api-keys)

##### Create a Google Kubernetes Engine cluster

Create a new cluster from the command line:

```shell
export CLUSTER=datadog-cluster
export ZONE=us-west1-a

gcloud container clusters create "$CLUSTER" --zone "$ZONE"
```

Configure `kubectl` to connect to the new cluster:

```shell
gcloud container clusters get-credentials "$CLUSTER" --zone "$ZONE"
```


###### Install the (app.k8s.io/v1beta1) Application CRD


Follow instructions from [here](https://github.com/kubernetes-sigs/application/blob/master/docs/quickstart.md).


###### Clone this repository

Clone this repository.

```shell
git clone git@github.com:DataDog/datadog-agent.git
```

Change into this directory.

```shell
cd google-marketplace
```

###### Deploy the application through mpdev

It's recommended to install the application in a dedicated namespace.
Before launching installation you need to setup some varaibles:

```
export REGISTRY=gcr.io/$(gcloud config get-value project | tr ':' '/')
export APP_NAME=datadog
export TAG=1.3.0 # Datadog Operator version that will be installed
```

You may also need to customize some parameters (name, namespace, APIKey)

```shell
docker build --pull --platform linux/amd64 --build-arg TAG=$TAG --tag $REGISTRY/$APP_NAME/deployer . && docker push $REGISTRY/$APP_NAME/deployer && mpdev install \
  --deployer=$REGISTRY/$APP_NAME/deployer \
  --parameters='{"name": "datadog", "namespace": "datadog-agent", "datadog.credentials.apiKey": "<your_api_key>"}'
```
