# JSON File Backend

> [Datadog Agent Secrets](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) using [JSON](https://en.wikipedia.org/wiki/JSON) files.

## Configuration

### Backend Settings

| Setting | Description |
| --- | --- |
| backend_type | Backend type |
| file_path| Absolute directory path to the JSON file |

## Backend Configuration

The backend configuration for JSON file secrets has the following pattern:

```yaml
---
backends:
  {backendId}:
    backend_type: file.json
    file_path: /path/to/json/file
```

The backend secret is referenced in your Datadog Agent configuration files using the **ENC** notation.

```yaml
# /etc/datadog-agent/datadog.yaml

api_key: "ENC[{backendId}:{json_property_name}"

```

## Configuration Examples

In the following examples, assume the JSON file is `/opt/production-secrets/secrets.json` with the following file contents:

```json
{
  "api_key": "••••••••••••0f83"
}
```

The following example will access the JSON secret from the Datadog Agent configuration YAML file(s) as such:

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
    backend_type: file.json
    file_path: /opt/production-secrets/secrets.json
```
