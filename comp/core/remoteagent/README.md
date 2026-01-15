# Remote Agent Component

The `remoteagent` component provides infrastructure for exposing any Agent process data to the Core Agent. Every Remote Agent advertises itself at startup to the Core Agent, sharing its gRPC endpoint and the services it provides. The Core Agent can then use the gRPC endpoint to call the Remote Agent services.

## Supported Services

The Remote Agent currently supports these services:
- **StatusProvider**: provides status details printed by the `agent status` command
- **FlareProvider**: provides flare files inserted in the flare archive  
- **TelemetryProvider**: provides telemetry data aggregated to the Core Agent telemetry component

More services will be added in the future. Feel free to reach out to us (Agent-runtimes team) if you need any specific service.

## Quick Start

The easiest way to expose your Agent data to the Core Agent is to copy and customize the provided templates:

1. **Copy the templates** to your new component location:
   ```bash
   # Copy the fx module template
   cp -r comp/core/remoteagent/fx-template comp/core/remoteagent/fx-<your-agent-name>
   
   # Copy the implementation template  
   cp -r comp/core/remoteagent/impl-template comp/core/remoteagent/impl-<your-agent-name>
   ```

2. **Update import paths** in the copied `fx.go` and `remoteagent.go` files to match your new component location

3. **Customize the implementation** in your copied `impl-<your-agent-name>/remoteagent.go`:
   - Add your gRPC service registrations in the `NewComponent` function
   - Implement your business logic in the `remoteagentImpl` struct
   - Add any additional dependencies to the `Requires` struct

4. **Wire it up** using the fx module in your Agent's main function

## Example

Here's how to register Remote Agent services:

```go
// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) (Provides, error) {
   remoteAgentServer, err := helper.NewUnimplementedRemoteAgentServer(...)
   // ...

   // Register your services:
   pbcore.RegisterStatusProviderServer(remoteAgentServer.GetGRPCServer(), remoteagentImpl)
   // ...
}


// Then implement the service methods:
func (r *remoteagentImpl) GetStatusDetails(_ context.Context, req *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	log.Printf("Got request for status details: %v", req)

	// Implement your business logic here

	return &pbcore.GetStatusDetailsResponse{
      // ...
   }, nil
}
```

## Helper Package Features

The `helper` package handles the complex parts of remote Agent setup:

- **TLS Configuration**: Automatic TLS setup using the IPC component
- **Authentication**: Token-based authentication with the Core Agent
- **Registration**: Automatic registration and periodic refresh with the Core Agent  
- **Session Management**: Session ID handling for proper Agent identification
- **Lifecycle**: Proper startup/shutdown hooks and gRPC server management

## Important Notes

- **Service Registration**: gRPC services must be registered in the constructor only, not in lifecycle callbacks. The gRPC server starts serving immediately after the lifecycle OnStart hook.
