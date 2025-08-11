# Health Platform Component

The Health Platform component provides a mechanism for collecting and reporting health information from the Datadog Agent to the backend. It includes hostname information, host ID from gopsutil, organization ID, and a list of issues with their details.

## Features

- **Issue Management**: Add, remove, and list issues
- **Periodic Reporting**: Automatically send health reports at configurable intervals
- **On-Demand Reporting**: Manually trigger health reports
- **Host Information**: Automatically includes hostname and host ID
- **Organization Context**: Includes organization ID for proper routing
- **Protobuf Serialization**: Uses efficient protobuf format for backend communication

## Configuration

The component supports the following configuration options:

```yaml
# Reporting interval (default: 15 minutes)
health_platform:
  interval: 15m

# Organization ID (required)
org_id: 12345
# Alternative: API key org ID
api_key_orgid: 12345
```

## Usage Example

```go
package main

import (
    "context"
    "time"

    "github.com/DataDog/datadog-agent/comp/core/health-platform"
    "github.com/DataDog/datadog-agent/comp/core/health-platform/fx"
)

func example(healthPlatform healthplatform.Component) {
    ctx := context.Background()

    // Add an issue
    issue := healthplatform.Issue{
        ID:    "disk-space-low",
        Name:  "Disk Space Low",
        Extra: "Available space: 5GB",
    }

    err := healthPlatform.AddIssue(issue)
    if err != nil {
        log.Printf("Failed to add issue: %v", err)
        return
    }

    // List current issues
    issues := healthPlatform.ListIssues()
    log.Printf("Current issues: %d", len(issues))

    // Submit report immediately
    err = healthPlatform.SubmitReport(ctx)
    if err != nil {
        log.Printf("Failed to submit report: %v", err)
        return
    }

    // Start periodic reporting
    err = healthPlatform.Start(ctx)
    if err != nil {
        log.Printf("Failed to start health platform: %v", err)
        return
    }

    // Later, remove the issue when resolved
    err = healthPlatform.RemoveIssue("disk-space-low")
    if err != nil {
        log.Printf("Failed to remove issue: %v", err)
    }

    // Stop periodic reporting
    err = healthPlatform.Stop()
    if err != nil {
        log.Printf("Failed to stop health platform: %v", err)
    }
}
```

## Component Integration

To use the Health Platform component in your application:

1. **Add the FX Module**:

```go
import (
    healthplatformfx "github.com/DataDog/datadog-agent/comp/core/health-platform/fx"
)

// In your fx.Options
fx.Options(
    healthplatformfx.Module(),
    // ... other modules
)
```

2. **Inject the Component**:

```go
type Dependencies struct {
    fx.In

    HealthPlatform healthplatform.Component
    // ... other dependencies
}
```

## Data Format

The component sends data in the following protobuf format:

```protobuf
message HealthReport {
    string hostname = 1;      // Agent hostname
    string host_id = 2;       // Host ID from gopsutil
    uint64 org_id = 3;        // Organization ID
    repeated Issue issues = 4; // List of issues
    int64 timestamp = 5;      // Report generation timestamp
}

message Issue {
    string id = 1;            // Unique issue identifier
    string name = 2;          // Human-readable issue name
    optional string extra = 3; // Optional complementary information
}
```

## Architecture

The component follows the standard Datadog Agent component architecture:

- **Interface Definition** (`def/component.go`): Defines the public API
- **Implementation** (`impl/health-platform.go`): Contains the business logic
- **FX Module** (`fx/fx.go`): Handles dependency injection
- **Component Alias** (`component.go`): Provides convenient imports

## Dependencies

The Health Platform component depends on:

- **Config Component**: For organization ID and configuration
- **Hostname Component**: For retrieving agent hostname
- **Default Forwarder**: For sending data to the backend
- **gopsutil**: For host system information

## Thread Safety

The component is thread-safe and can be used concurrently from multiple goroutines. Internal issue storage is protected by mutexes.

## Error Handling

The component provides detailed error messages for common failure scenarios:

- Missing or invalid issue IDs/names
- Configuration errors (missing org ID)
- Network/communication failures
- Hostname resolution failures

## Testing

The component includes comprehensive unit tests. Run them with:

```bash
go test ./comp/core/health-platform/impl/...
```

## Team

**Team**: agent-configuration
