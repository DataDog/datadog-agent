# Azure Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following Azure services:

| Backend Type | Azure Service |
| --- | --- |
| [azure.keyvault](keyvault.md) | [Azure Keyvault](https://docs.microsoft.com/en-us/Azure/key-vault/secrets/quick-create-portal) |


## Azure Session

The following authentication methods are supported for authenticating to Azure, and operate in this order of precedence if multiple authentication methods are defined:

1. **Service Principal Client Credentials**. The Azure tenant ID, service principal client ID, and service principal client secret defined on the backed configuration's `azure_session` section within the datadog-secret-backend.yaml file.

2. **Service Principal Certificate Credential**. The Azure tenant ID, application client ID, service principal certificate path, and service principal certificate password (if applicable) defined on the backed configuration's `azure_session` section within the datadog-secret-backend.yaml file.

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

In all cases, you will need to specify `keyvaulturl` and with service principal based authentication, the `azure_tenant_id` and `azure_client_id` corresponding to the Azure KeyVault resource.

Simple string values can be defined adding the config variable `force_string: true`. The `force_string: true` backend configuration setting will interpret the contents of the Azure Key Vault Secret as a string, even if the stored secret value is valid JSON.

When `force_string: false` is defined, or when the backend setting `force_string` is not defined, then the secret value will be interpreted as JSON with simple depth of one (1) and create a secretId with each field name. If the secret value is not valid JSON, then it will behave as if `force_string: true` was defined and will return the full contents of the secret value with secretId `_`.

## Example Session Configurations

### Azure Service Principal With Client Credentials
```yaml
---
backends:
  keyvault1:
    secret_id: apikey
    backend_type: azure.keyvault
    keyvaulturl: "https://my-keyvault.vault.azure.net"
    force_string: true
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
    force_string: true
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
    force_string: true
    azure_session:
      azure_tenant_id: "1234abcd-5e6f-7g8h-9ijk-lmnopqrstuv0"
      azure_client_id: "0vutsrqp-onml-kji9-h8g7-f6e5dcba4321"
      azure_certificate_path: "/path/to/certificate.pem"
```

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
