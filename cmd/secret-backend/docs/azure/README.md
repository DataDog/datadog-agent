# Azure Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following Azure services:

| Backend Type | Azure Service |
| --- | --- |
| [azure.keyvault](keyvault.md) | [Azure Keyvault](https://docs.microsoft.com/en-us/Azure/key-vault/secrets/quick-create-portal) |


## Azure Authentication

We recommend using Managed Identities in order to authenticate with Azure. This lets you associate cloud resources with AMI accounts, removing the need to put sensitive information in your `datadog.yaml` configuration file.

## Example Configuration

### Azure Managed Identity

```
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: azure.keyvault
secret_backend_config:
  keyvaulturl: "https://my-keyvault.vault.azure.net"
```

Review the [azure.keyvault](keyvault.md) backend documentation examples of configurations for Datadog Agent secrets.
