# Installation

## Quick install with Google Cloud Marketplace

Get up and running with a few clicks! Install the Datadog agent daemonset to a
Google Kubernetes Engine cluster using Google Cloud Marketplace.

Prior to that:

- Create a Datadog [account](https://www.datadoghq.com/)
- Create a ClusterRole in your target Kubernetes cluster with the [required permissions](https://docs.datadoghq.com/integrations/faq/using-rbac-permission-with-your-kubernetes-integration/)

Next, follow the on-screen instructions:
*TODO: link to solution details page*

## Command line instructions

Follow these instructions to install the Datadog agent from the command line.

### Prerequisites

- Setup cluster
  - Permissions
- Setup kubectl
- Create a Datadog [account](https://www.datadoghq.com/)
- Create a ClusterRole in your target Kubernetes cluster with the [required permissions](https://docs.datadoghq.com/integrations/faq/using-rbac-permission-with-your-kubernetes-integration/)

### Commands

Set environment variables (update when necessary):

```bash
export namespace=default
export name=datadog-agent
export name_apikey_secret=datadog-api-key
export api_key=YOUR_DATADOG_API_KEY_IN_BASE64
export name_service_account=datadog-sa
export image_datadog_agent=datadog/agent:latest
export name_cluster_role=datadog-cr
```

One-time install the `Application` CRD:

```bash
make crd/install
```

Install the Datadog agent:

```bash
make app/install
```
