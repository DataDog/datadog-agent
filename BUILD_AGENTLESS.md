# Build the agentless agent with embedded remote config
go build -tags "grpcnotrace serverless otlp" -o bin/agentless ./cmd/agentless

# The agentless agent is now ready to run as a standalone trace agent
# with embedded remote configuration (no IPC dependency on main agent)
