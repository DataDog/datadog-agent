# Azure Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following Azure services:

| Backend Type | Azure Service |
| --- | --- |
| [Azure.keyvault](keyvault.md) | [Azure Keyvault](https://docs.microsoft.com/en-us/Azure/key-vault/secrets/quick-create-portal) |


## Azure Session

The following authentication methods are supported for authenticating to Azure:

1. **Client Credentials**. The Azure tenant ID, service principal client ID, and service principal client secret defined on the backed configuration's `Azure_session` section within the datadog-secret-backend.yaml file.

2. **Certificate Credential**.The Azure tenant ID, application client ID, service principal certificate path, and service principal certificate password defined on the backed configuration's `Azure_session` section within the datadog-secret-backend.yaml file.

3. **Username/Password**. The Azure tenant ID, application client ID, Azure username, and Azure password defined on the backed configuration's `Azure_session` section within the datadog-secret-backend.yaml file.

## Azure Session Settings

The following `azure_session` settings are available on all supported Azure Service backends:

| Setting | Description |
| --- | --- |
| keyvaulturl | URL for the Azure Keyvault Hosting the Secret(s) |
| azure_tenant_id | Azure Tenant ID |
| azure_client_id | Azure Application Client ID |
| azure_client_secret | Azure Application Client Secret |
| azure_certificate_path | Path to Application Azure Certificate |
| azure_certificate_password | Password for Azure Certificate
| azure_username | Azure Username |
| azure_password | Azure Password |

In all cases, you'll need to specify `keyvaulturl`, `azure_tenant_id`, and `azure_client_id` to correspond to the Azure KeyVault reasource and the application definition being used to authenticate to Azure.

## Example Session Configurations

### Azure Client Credentials
```yaml
---
backends:
  keyvault1:
    secret_id: apikey
    backend_type: azure.keyvault
    keyvaulturl: "https://my-keyvault.vault.azure.net"
    azure_session:
      azure_tenant_id: "1234abcd-5e6f-7g8h-9ijk-lmnopqrstuv0"
      azure_client_id: "0vutsrqp-onml-kji9-h8g7-f6e5dcba4321"
      azure_client_secret: "averyrandomsecrethere"
```

### Azure Certificate
```yaml
---
backends:
  keyvault1:
    secret_id: apikey
    backend_type: azure.keyvault
    keyvaulturl: "https://my-keyvault.vault.azure.net"
    azure_session:
      azure_tenant_id: "1234abcd-5e6f-7g8h-9ijk-lmnopqrstuv0"
      azure_client_id: "0vutsrqp-onml-kji9-h8g7-f6e5dcba4321"
      azure_certificate_path: "/path/to/certificate.pem"
      azure_certificate_password: "asecretcertificatepassword"
```

### Azure Username/Password Credential
```yaml
---
backends:
  keyvault1:
    secret_id: apikey
    backend_type: azure.keyvault
    keyvaulturl: "https://my-keyvault.vault.azure.net"
    azure_session:
      azure_tenant_id: "1234abcd-5e6f-7g8h-9ijk-lmnopqrstuv0"
      azure_client_id: "0vutsrqp-onml-kji9-h8g7-f6e5dcba4321"
      azure_username: "myusername"
      azure_password: "asecretpassword"
```

Review the [azure.keyvault](keyvault.md) backend documentation examples of configurations for Datadog Agent secrets.
