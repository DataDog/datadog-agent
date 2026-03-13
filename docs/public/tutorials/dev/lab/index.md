# Lab environments

-----

Lab environments are Kubernetes clusters for testing the Datadog Agent. The `dda lab` command group manages these environments.

## Supported providers

| Provider | Category | Description |
|----------|----------|-------------|
| [Kind](kind.md) | Local | Kubernetes in Docker - lightweight local clusters |

## Common commands

### List environments

```bash
dda lab list

# Filter by type
dda lab list --type kind

# JSON output for scripting
dda lab list --json
```

### Delete environment

```bash
dda lab delete

# Non-interactive deletion
dda lab delete --id <id> --yes
```

## Configuration

If the environment provider installs the Agent (for example `dda lab local kind`), you must provide an API key. API keys can be configured via environment variables:

```bash
export E2E_API_KEY=your_api_key
export E2E_APP_KEY=your_app_key  # optional
```

Or in `~/.test_infra_config.yaml`:

```yaml
configParams:
  agent:
    apiKey: your_api_key
    appKey: your_app_key
```

