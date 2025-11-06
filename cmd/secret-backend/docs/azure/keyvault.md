# Azure Keyvault Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [Azure Keyvault](https://docs.microsoft.com/en-us/Azure/key-vault/secrets/quick-create-portal)

## Configuration

### Managed Identity

To access your Key Vault, create a Managed Identity in your environment. Assign that Identity as a Role on your Virtual Machine, which will give it access to your Key Vault's secrets.

### Backend Settings

| Setting | Description |
| --- | --- |
| keyvaulturl | URL of the Azure keyvault |

## Backend Configuration

In your `datadog.yaml` config, the setting `secret_backend_type` must be set to `azure.keyvault`.

The backend configuration for Azure Key Vault secrets has the following pattern:

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: azure.keyvault
secret_backend_config:
  keyvaulturl: {keyVaultURL}
```

**backend_type** must be set to `azure.keyvault` and **keyvaulturl** must be set to your target Azure Key Vault URL.

The backend secret is referenced in your Datadog Agent configuration file using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{secretHandle}]"
```

Azure Keyvault can hold multiple secret keys and values using json. For example, assuming an Azure secret with a **Secret Name** of `MySecret`:

```json
{
    "ddapikey": "SecretValue1",
    "ddappkey": "SecretValue2",
    "ddorgname": "SecretValue3"
}
```

This can be accessed using a semicolon (`;`) to separate the Secret Name from the Key. The notation in the datadog.yaml config file looks like **ENC[SecretName;SecretKey]**. If this semicolon is not present, then the entire string will be treated as the plain text value of the secret. Otherwise, `SecretKey` is the json key referring to the actual secret that you are trying to pull the value of.

```yaml
# /etc/datadog-agent/datadog.yml
api_key: "ENC[MySecret;ddapikey]"
app_key: "ENC[MySecret;ddappkey]"
property3: "ENC[MySecret;ddorgname]"
```

## Configuration Example

In the following example, assume the Azure secret name is `MySecretName` with a secret value containing the Datadog Agent api_key:

```json
{
    "ddapikey": "••••••••••••0f83"
}
```

Also assume that the Key Vault's URL is `https://mykeyvault.vault.azure.net`

The below example will access the secret from the Datadog Agent configuration yaml file(s) like so:

```yaml
# /etc/datadog-agent/datadog.yaml

#################################
## Datadog Agent Configuration ##
#################################

## @param api_key - string - required
## @env DD_API_KEY - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key: "ENC[MySecretName;ddapikey]" 
...
...
...
secret_backend_type: azure.keyvault
secret_backend_config:
  keyvaulturl: https://mykeyvault.vault.azure.net
```
