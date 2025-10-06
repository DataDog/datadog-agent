# Remote Agent Helper

This package provides a helper for creating remote agent servers that automatically register with the Datadog Core Agent.

## Usage

### 1. Create the Server

```go
server, err := helper.NewUnimplementedRemoteAgentServer(
    ipcComp,
    log,
    lc,
    agentIpcAddress,
    "my-agent",        // agent flavor
    "My Custom Agent", // display name
)
if err != nil {
    return err
}
```

### 2. Register Your Services

After creating the `UnimplementedRemoteAgentServer`, register your gRPC services using the generated protobuf code:

```go
// Register your service implementations
pbcore.RegisterStatusProviderServer(register.GetGRPCServer(), remoteagentImpl)
pbcore.RegisterFlareProviderServer(register.GetGRPCServer(), remoteagentImpl)
pbcore.RegisterTelemetryProviderServer(register.GetGRPCServer(), remoteagentImpl)
// ... register other services as needed
```

**Important**: Services must be registered in the constructor only, not in lifecycle callbacks. The gRPC server starts serving immediately after the lifecycle OnStart hook, so all services must be registered before that point.

## What it handles

- **TLS Configuration**: Automatic TLS setup using IPC component
- **Authentication**: Token-based auth with the Core Agent
- **Registration**: Automatic registration and periodic refresh with the Core Agent
- **Session Management**: Session ID handling for proper agent identification
- **Lifecycle**: Proper startup/shutdown hooks

The server automatically handles the gRPC server lifecycle and maintains registration with the Core Agent.
