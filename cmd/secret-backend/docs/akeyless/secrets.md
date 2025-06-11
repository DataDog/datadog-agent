# Akeyless Secrets Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [Akeyless Secrets](https://docs.akeyless.io/docs/static-secrets)

## Configuration

### Backend Settings

| Setting | Description                                  |
| --- |----------------------------------------------|
| backend_type | Backend type, Must be set to `akeyless`             |
| akeyless_url | URL of your Akeyless instance (e.g., https://myakeyless.io) |
|akeyless_session | 	Configuration for the Akeyless session                     |

The `akeyless_session` section defines the credentials used to authenticate with Akeyless.

| Setting | Description |
| --- | --- |
| akeyless_access_id | Akeyless Access ID |
| akeyless_access_key | Akeyless Access Key |

## Backend Configuration

The backend configuration for Akeyless follows this pattern:

```yaml
---
backends:
  akeyless:
    backend_type: 'akeyless'
    akeyless_url: 'https://api.akeyless.io'
    akeyless_session:
      akeyless_access_id: 'abcdef123456**********'
      akeyless_access_key: 'abcdef123456**********'
```

**backend_type** must be set to `akeyless` and both **akeyless_access_id** and **akeyless_access_key** must be provided in each backend configuration.

The backend secret is referenced in your Datadog Agent configuration files using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{backendId}:{secret-path}"

```

## Configuration Examples

In the following examples, assume the Hashicorp Vault secret path prefix is `/Datadog/Production` with a parameter key of `api_key`:

```sh
/secret-folder/datadog-sample-key: (SecureString) "••••••••••••0f83"
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
api_key: "ENC[akeyless:/secret-folder/datadog-sample-key]" 
```

**Akeyless Authentication with Access ID and Access Key**


```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  akeyless:
    backend_type: 'akeyless'
    akeyless_url: 'https://api.akeyless.io'
    akeyless_session:
      akeyless_access_id: 'abcdef123456**********'
      akeyless_access_key: 'abcdef123456**********'
```

Multiple secret backends, of the same or different types, can be defined in your `datadog-secret-backend` yaml configuration. As a result, you can leverage multiple supported backends (file.yaml, file.json, aws.ssm, and aws.secrets, azure.keyvault) in your Datadog Agent configuration.
