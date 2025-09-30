# Agentless Agent - Build and Usage

## Build

```bash
# Build with proper agent version (required for Remote Config backend compatibility)
go build -tags "grpcnotrace serverless otlp" \
  -ldflags "-X 'github.com/DataDog/datadog-agent/pkg/version.AgentVersion=7.60.0'" \
  -o bin/agentless ./cmd/agentless
```

**Important**: The agent version must be >= 7.39.1 to satisfy the Remote Config backend constraints.

## Run

```bash
DD_API_KEY=your_api_key DD_LOG_LEVEL=debug ./bin/agentless
```

## Features

- ✅ Standalone trace agent with embedded remote configuration service
- ✅ No IPC dependency on main Datadog agent  
- ✅ Origin tag set to "agentless" instead of "lambda"
- ✅ Trace collection and APM data forwarding
- ✅ Debug logging with `DD_LOG_LEVEL=debug`
- ✅ Configuration via `datadog.yaml` or environment variables

## Configuration

The agent loads configuration from `datadog.yaml` in the current directory, or from the path specified in `DD_CONFIG_FILE`.

Minimal configuration:
```yaml
site: datadoghq.com
log_level: debug
remote_configuration:
  enabled: true
apm_config:
  enabled: true
```

## Remote Configuration

The agentless agent includes a **fully functional embedded Remote Configuration service** that:
- ✅ Fetches configurations from the Datadog backend
- ✅ Serves configurations to tracers via `/v0.7/config`
- ✅ Supports APM_TRACING, APM_SAMPLING, AGENT_CONFIG, and other products
- ✅ No dependency on the main Datadog agent
- ✅ Products are registered dynamically when tracers connect (no pre-configuration needed)

### Version Requirement

The Remote Config backend enforces a minimum agent version constraint: **`>= 6.39.1 || >= 7.39.1`**

This is why the build command includes `-ldflags` to set the version. Without this, the default version (`6.0.0`) will be rejected by the backend with a 400 error.

### How It Works

1. Agent starts with embedded RC service (no products registered yet)
2. Tracer connects and sends request to `/v0.7/config` with its product list (e.g., `ASM_DD`, `APM_TRACING`)
3. RC service registers these products and fetches configs from backend
4. Tracer receives configurations
5. Subsequent tracer requests receive cached/updated configs
