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
- [Get your Datadog API key](https://app.datadoghq.com/account/settings#api)

Then follow the [on-screen instructions](https://console.cloud.google.com/marketplace/details/datadog-saas/datadog).

### Command line instructions

Follow these instructions to install the Datadog Agent from the command line.

#### Prerequisites (one time setup)

##### Command-line tools

Your development environment should contain the following tools:

- [gcloud](https://cloud.google.com/sdk/gcloud/)
- [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/)
- [git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)

##### Create a Datadog account

- Create a Datadog [account](https://www.datadoghq.com/)
- [Get your Datadog API key](https://app.datadoghq.com/account/settings#api)

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

###### Clone this repository

Clone this repository.

```shell
git clone git@github.com:DataDog/datadog-agent.git
```

###### Install the Application resource definition

An Application resource is a collection of individual Kubernetes components,
such as Services, Deployments, and so on, that you can manage as a group.

To set up your cluster to understand Application resources, navigate to the
`google-marketplace` folder in the repository, and run the following command:

```shell
make crd/install
```

You need to run this command once.

The Application resource is defined by the
[Kubernetes SIG-apps](https://github.com/kubernetes/community/tree/master/sig-apps)
community. The source code can be found on
[github.com/kubernetes-sigs/application](https://github.com/kubernetes-sigs/application).

#### Install the Application

Navigate to the `google-marketplace` folder:

```shell
cd datadog-agent/google-marketplace
```

##### Configure the application with environment variables

Choose an instance name and
[namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/)
for the app. In most cases, you can use the `default` namespace.

```shell
export namespace=default
export name=datadog-agent
```

Configure the container image:

```shell
export datadogAgentImage=datadog/agent:latest
```

The image above is referenced by
[tag](https://docs.docker.com/engine/reference/commandline/tag). We recommend
that you pin each image to an immutable
[content digest](https://docs.docker.com/registry/spec/api/#content-digests).
This ensures that the installed application always uses the same images,
until you are ready to upgrade.

Fill in your Datadog API key:

```shell
export apiKeyEncoded=$(echo YOUR_DATADOG_API_KEY | base64)
```

##### Configure the service account

The Datadog Agent needs a service account in the target namespace with cluster wide
permissions to inspect Kubernetes resources.

Create a `ClusterRole` in your target Kubernetes cluster, a `ServiceAccount` in the namespace specified earlier, and a `ClusterRoleBinding` to tie them together:

```shell
export serviceAccount=datadog-agent-sa
make rbac/install  # from the datadog-agent/google-marketplace folder
```

##### Install the Datadog application

Install the Datadog Agent with the following command:

```bash
make app/install
```

This expands the templates located in the `datadog-agent/google-marketplace/manifest` folder with the parameters given previously, and apply them, creating a Secret for the API key, and a DaemonSet tied to the service account previously created.

## Basic Usage

Once the application is installed, your Kubernetes nodes show up in your [Datadog Infrastructure Map](https://app.datadoghq.com/infrastructure/map) and metrics start [flowing to your Datadog account](https://app.datadoghq.com/metric/summary)!

To further configure your agents, and setup your account, refer to [the Datadog documentation](https://docs.datadoghq.com/).

## Backup and restore

The agent is stateless, and all your data is backed up in your Datadog account.

## Image updates

## Scaling

The agent is deployed as a DaemonSet, it will automatically scale with your cluster.

## Deletion

Remove the agent resources following the [on-screen instructions](https://console.cloud.google.com/marketplace/details/datadog-saas/datadog).

Or if you followed the command line instructions, by running the following commands from the `datadog-agent/google-marketplace` folder:

```shell
make app/uninstall
make rbac/uninstall
export serviceAccount=datadog-agent-sa
make rbac/install  # from the datadog-agent/google-marketplace folder
```
