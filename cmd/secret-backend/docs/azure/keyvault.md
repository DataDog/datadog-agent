# Azure Keyvault Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [Azure Keyvault](https://docs.microsoft.com/en-us/Azure/key-vault/secrets/quick-create-portal)

## Configuration

### Backend Settings

| Setting | Description |
| --- | --- |
| keyvaulturl | URL of the Azure keyvault |
| backend_type | Backend type |
| secret_id | Secret friendly name or Amazon Resource Name |
| azure_session | Azure session configuration |

## Backend Configuration

The backend configuration for Azure Key Vault secrets has the following pattern:

```yaml
---
backends:
  {backendId}:
    backend_type: azure.keyvault
    keyvaulturl: https://mykeyvault.vault.azure.net
    # interpret contents of secret as string
    force_string: [true|false]
    azure_session:
      azure_client_id: {clientId}
      # ... additional session settings
    secret_id: {secretId}
```

**backend_type** must be set to `azure.keyvault` and **secret_id** must be set to your target Azure Key Vault secret name.

The backend secret is referenced in your Datadog Agent configuration files using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{backendId}:{secretKey}]"

```

Azure Keyvault can hold multiple secret keys and values. For example, assuming a secret with a **backend_id** of `MySecretBackend` and an Azure secret id of `my_azure_secret_id`:

```json
{
    "ddapikey": "SecretValue1",
    "ddappkey": "SecretValue2",
    "ddorgname": "SecretValue3"
}
```

Support for simple string values can be defined adding the config variable `force_string: true`. This will interpret the contents of secret as a string regardless of input.

This will allow for generating simple string values with or without JSON parsing using the _ secretID. All input will be interpreted as a single string and if it fails to unmarshal the JSON, it will grab the raw string from the input. The secretId then gets coerced as _ and will access DD with the backendID and its secret using the ENC[] notation.

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: azure.keyvault
    secret_id: my_azure_secret_id
    keyvaulturl: https://mykeyvault.vault.azure.net
    # interpret contents of secret as string
    force_string: true
    azure_session:
      azure_tenant_id: abcdef-*****
      azure_client_id: 123456-*****
      azure_client_secret: ************
```

```yaml
# /etc/datadog-agent/datadog.yml
api_key: "ENC[MySecretBackend:ddapikey]"
app_key: "ENC[MySecretBackend:ddappkey]"
property3: "ENC[MySecretBackend:ddorgname]"
```

Multiple secret backends, of the same or different types, can be defined in your `datadog-secret-backend` yaml configuration. As a result, you can leverage multiple supported backends (file.yaml, file.json, aws.ssm, and aws.secrets, azure.keyvault) in your Datadog Agent configuration.


## Configuration Examples

In the following examples, assume the Azure secret name (id) is `ddapikey` with a secret value containing the Datadog Agent api_key:

```json
{
    "ddapikey": "••••••••••••0f83"
}
```

Each of the following examples will access the secret from the Datadog Agent configuration yaml file(s) as such:

```yaml
# /etc/datadog-agent/datadog.yaml

#########################
## Basic Configuration ##
#########################

## @param api_key - string - required
## @env DD_API_KEY - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key: "ENC[MySecretBackend:ddapikey]" 
```

**Azure Service Principal With Client Credentials**

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: azure.keyvault
    secret_id: my_azure_secret_id
    keyvaulturl: https://mykeyvault.vault.azure.net
    # interpret contents of secret as string
    force_string: true
    azure_session:
      azure_tenant_id: abcdef-*****
      azure_client_id: 123456-*****
      azure_client_secret: ************
```

**Azure Service Principal With Client Certificate Without Password Protection**

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: azure.keyvault
    secret_id: my_azure_secret_id
    keyvaulturl: https://mykeyvault.vault.azure.net
    # interpret contents of secret as string
    force_string: true
    azure_session:
      azure_tenant_id: abcdef-*****
      azure_client_id: 123456-*****
      azure_certificate_path: /path/to/cert.pfx
```

**Azure Service Principal With Client Certificate With Password Protection**

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: azure.keyvault
    secret_id: my_azure_secret_id
    keyvaulturl: https://mykeyvault.vault.azure.net
    # interpret contents of secret as string
    force_string: true
    azure_session:
      azure_tenant_id: abcdef-*****
      azure_client_id: 123456-*****
      azure_certificate_path: /path/to/cert.pfx
      azure_certificate_password: mycertificatepassword
```

**Azure Managed Identity**

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  MySecretBackend:
    backend_type: azure.keyvault
    secret_id: my_azure_secret_id
    keyvaulturl: https://mykeyvault.vault.azure.net
```
