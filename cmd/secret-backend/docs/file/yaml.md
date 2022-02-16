# YAML File Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [YAML](https://en.wikipedia.org/wiki/YAML) files.

## Configuration

### Backend Settings

| Setting | Description |
| --- | --- |
| backend_type | Backend type |
| file_path| Absolute directory path to the YAML file |

## Backend Configuration

The backend configuration for JSON file secrets has the following pattern:

```yaml
---
backends:
  {backendId}:
    backend_type: file.yaml
    file_path: /path/to/yaml/file
```

The backend secret is referenced in your Datadog Agent configuration files using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{backendId}:{yaml_property_name}]"

```

## Configuration Examples

In the following examples, assume the YAML file is `/opt/production-secrets/secrets.yaml` with the following file contents:

```yaml
---
api_key: "••••••••••••0f83"
```

The following example will access the YAML secret from the Datadog Agent configuration YAML file(s) as such:

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
api_key: "ENC[agent_secret:api_key]" 
```

```yaml
# /opt/datadog-secret-backend/datadog-secret-backend.yaml
---
backends:
  agent_secret:
    backend_type: file.yaml
    file_path: /opt/production-secrets/secrets.yaml
```
