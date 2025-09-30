# Agentless Agent - Build and Usage

## Build

### Local Build (macOS/Linux native)

```bash
# Build with proper agent version (required for Remote Config backend compatibility)
go build -tags "grpcnotrace serverless otlp" \
  -ldflags "-s -w -X 'github.com/DataDog/datadog-agent/pkg/version.AgentVersion=7.60.0'" \
  -o bin/agentless ./cmd/agentless
```

**Important**: The agent version must be >= 7.39.1 to satisfy the Remote Config backend constraints.

### Cross-Compilation for Linux (from macOS)

The agentless agent uses CGo dependencies (e.g., `zstd`), which require a C compiler for the target platform. Use Docker for cross-compilation:

```bash
# Build for Linux AMD64 (with UPX compression)
docker run --rm --platform linux/amd64 -v "$PWD":/workspace -w /workspace golang:1.24 bash -c '
  go build -tags "grpcnotrace serverless otlp" \
    -ldflags "-s -w -X '\''github.com/DataDog/datadog-agent/pkg/version.AgentVersion=7.60.0'\''" \
    -o bin/agentless-linux-amd64 ./cmd/agentless && \
  apt-get update -qq && apt-get install -y -qq upx-ucl > /dev/null 2>&1 && \
  upx --best --lzma bin/agentless-linux-amd64
'

# Build for Linux ARM64 (with UPX compression)
docker run --rm --platform linux/arm64 -v "$PWD":/workspace -w /workspace golang:1.24 bash -c '
  go build -tags "grpcnotrace serverless otlp" \
    -ldflags "-s -w -X '\''github.com/DataDog/datadog-agent/pkg/version.AgentVersion=7.60.0'\''" \
    -o bin/agentless-linux-arm64 ./cmd/agentless && \
  apt-get update -qq && apt-get install -y -qq upx-ucl > /dev/null 2>&1 && \
  upx --best --lzma bin/agentless-linux-arm64
'
```

The `-s -w` ldflags strip debug symbols (reducing binary size before compression), and UPX applies additional compression.

**Note**: The project requires Go >= 1.24.7 (see `go.work`).

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
- ✅ **UDS/Named Pipe by default** - TCP listener disabled (`DD_APM_RECEIVER_PORT=0`)
- ✅ **Default socket path**: `/tmp/datadog_libagent.socket` (Unix) or `\\.\pipe\datadog-libagent` (Windows)

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
  # Note: receiver_port defaults to 0 (TCP disabled)
  # receiver_socket defaults to /tmp/datadog_libagent.socket
```

### Default APM Receiver Settings

The agentless agent uses **platform-specific IPC by default** for better performance and security:

- **`DD_APM_RECEIVER_PORT=0`** - TCP listener is disabled by default

**Unix/Linux/macOS:**
- **`DD_APM_RECEIVER_SOCKET=/tmp/datadog_libagent.socket`** - Default UDS path

**Windows:**
- **`DD_APM_WINDOWS_PIPE_NAME=\\.\pipe\datadog-libagent`** - Default named pipe

To enable TCP listener (e.g., for compatibility):
```bash
DD_APM_RECEIVER_PORT=8126 ./bin/agentless
```

To use a custom socket/pipe path:
```bash
# Unix/Linux/macOS
DD_APM_RECEIVER_SOCKET=/var/run/datadog/apm.socket ./bin/agentless

# Windows
DD_APM_WINDOWS_PIPE_NAME=\\.\pipe\my-custom-pipe ./bin/agentless
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
