# Build the agentless agent with embedded remote config
go build -tags "grpcnotrace serverless otlp" -o bin/agentless ./cmd/agentless

# Run the agentless agent
DD_API_KEY=your_api_key DD_LOG_LEVEL=debug ./bin/agentless

# Features:
# - Standalone trace agent with embedded remote configuration
# - No IPC dependency on main Datadog agent
# - Origin tag set to "agentless" instead of "lambda"
# - Debug logging enabled with DD_LOG_LEVEL=debug
# - Configuration via DD_CONFIG_FILE environment variable
