# Installation

## Quick install with Google Cloud Marketplace

Get up and running with a few clicks! Install the Datadog agent daemonset to a
Google Kubernetes Engine cluster using Google Cloud Marketplace. Follow the
on-screen instructions:
*TODO: link to solution details page*

## Command line instructions

Follow these instructions to install the Datadog agent from the command line.

### Prerequisites

- Setup cluster
  - Permissions
- Setup kubectl
- Create a Datadog [account](https://www.datadoghq.com/)

### Commands

Set environment variables (update when necessary):
```
export namespace=default
export name=datadog-agent
export name_apikey_secret=datadog-api-key
export api_key=INSERT_YOUR_DATADOG_API_KEY
export name_service_account=datadog-sa
export image_datadog_agent=datadog/agent:latest
export name_cluster_role=datadog-cr
```

One-time install the `Application` CRD:
```
make crd/install
```

Install the Datadog agent:
```
make app/install
```

# Upgrades

*TODO: instructions for upgrades*
