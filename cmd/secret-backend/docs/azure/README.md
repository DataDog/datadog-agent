# Azure Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following Azure services:

| Backend Type | Azure Service |
| --- | --- |
| [azure.keyvault](keyvault.md) | [Azure Keyvault](https://docs.microsoft.com/en-us/Azure/key-vault/secrets/quick-create-portal) |


## Azure Session

The following authentication methods are supported for authenticating to Azure, and operate in this order of precedence if multiple authentication methods are defined:

1. **Service Principal Client Credentials**. The Azure tenant ID, service principal client ID, and service principal client secret defined on the backed configuration's `azure_session` section within the datadog-secret-backend.yaml file.

2. **Service Principal Certificate Credential**.The Azure tenant ID, application client ID, service principal certificate path, and service principal certificate password (if applicable) defined on the backed configuration's `azure_session` section within the datadog-secret-backend.yaml file.

3. **Azure Managed Identity**. No `azure_session` block needs to be defined at all, since this is all handled through the standard Azure credential chain.

## Azure Session Settings

The following `azure_session` settings are available on all supported Azure Service backends:

| Setting | Description |
| --- | --- |
| azure_tenant_id | Azure Tenant ID |
| azure_client_id | Azure Application Client ID |
| azure_client_secret | Azure Application Client Secret |
| azure_certificate_path | Path to Application Azure Certificate |
| azure_certificate_password | Password for Azure Certificate |

In all cases you'll need to specify `keyvaulturl`, and in any service princiapal based authentication, `azure_tenant_id` and `azure_client_id` to correspond to the Azure KeyVault reasource and the application definition being used to authenticate to Azure.

## Example Session Configurations

### Azure Service Principal With Client Credentials
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

### Azure Service Principal With Certificate and Certificate Password
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

### Azure Service Principal With Certificate Without Certificate Password
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
<<<<<<< HEAD
=======
```
>>>>>>> f09380c13840213a2c439df4653663ffcd603afc

### Azure Managed Identity
```yaml
---
backends:
  keyvault1:
    secret_id: apikey
    backend_type: azure.keyvault
    keyvaulturl: "https://my-keyvault.vault.azure.net"
```

Review the [azure.keyvault](keyvault.md) backend documentation examples of configurations for Datadog Agent secrets.
