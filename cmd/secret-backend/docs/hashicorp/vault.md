# Hashicorp Vault Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [Hashicorp Vault Secrets Engine](https://learn.hashicorp.com/tutorials/vault/static-secrets)

## Configuration

### Backend Settings

| Setting | Description |
| --- | --- |
| backend_type | Backend type |
| vault_address | DNS/IP of the Hashicorp Vault system |
| vault_tls_config | TLS Configuration to access the Vault system |
| vault_session | Authentication configuration to access the Vault system |

### TLS Settings

| Setting | Description |
| --- | --- |
| ca_cert | Path to PEM-encoded CA cert file to verify the Vault server SSL certificate |
| ca_path | Path to directory of PEM-encoded CA cert files to verify the Vault server SSL certificate |
| client_cert | Path to the certificate for Vault communication |
| client_key | Path to the private key for Vault communication |
| tls_server | If set, is used to set the SNI host when connecting via TLS |
| Insecure | Enables or disables SSL verification (bool) |

## Backend Configuration

The backend configuration for Hashicorp Vault has the following pattern:

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: hashicorp.vault
secret_backend_config:
  vault_address: http://myvaultaddress.net
  vault_tls_config:
      # ... TLS settings if applicable
  vault_session:
    vault_auth_type: aws
    # ... additional session settings
```

**backend_type** must be set to `hashicorp.vault`.

The path to the secret and the backend secret itself is referenced in your Datadog Agent configuration file using the **ENC** notation. The two need to be separated by a semicolon.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{secret_path};{secret}]"

secret_backend_type: hashicorp.vault
secret_backend_config:
  vault_address: http://myvaultaddress.net
  vault_tls_config:
      # ... TLS settings if applicable
  vault_session:
    vault_role_id: 123456-************
    vault_secret_id: abcdef-********
```

## Configuration Examples

In the following examples, assume the Hashicorp Vault secret path prefix is `/Datadog/Production` with a parameter key of `api_key`:

```sh
/DatadogAgent/Production/api_key: (SecureString) "••••••••••••0f83"
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
api_key: "ENC[/Datadog/Production;apikey]" 
```

### Hashicorp Vault Authentication with AWS Instance Profile

```yaml
# /etc/datadog-agent/datadog.yaml
---
secret_backend_type: hashicorp.vault
secret_backend_config:
  vault_address: http://myvaultaddress.net
  vault_session:
    vault_auth_type: aws
    vault_aws_role: Name-of-IAM-role-attached-to-machine
    aws_region: us-east-1
```