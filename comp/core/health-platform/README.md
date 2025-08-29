# Health Platform Component

This is a parent component that will be used by the health platform. It provides a mechanism for collecting and reporting health information from the Datadog Agent to the backend.

## Overview

The Health Platform component follows the standard Datadog Agent component architecture and provides:

- Issue management (add, remove, list)
- Periodic and on-demand health reporting
- Host information integration (hostname, host ID)
- Backend communication via protobuf
- **Sub-component architecture** for modular health checking

## Sub-Components

The health platform supports sub-components that focus on specific areas of health monitoring:

- **Logs Agent Health Checker**: Monitors logs agent health issues
- Additional sub-components can be added for other health domains

Sub-components are automatically started/stopped with the parent component and their issues are aggregated into unified health reports.

## Usage

```go
import (
    healthplatformfx "github.com/DataDog/datadog-agent/comp/core/health-platform/fx"
    "github.com/DataDog/datadog-agent/comp/core/health-platform"
)

// Add to your fx.Options
fx.Options(
    healthplatformfx.Module(),
    // ... other modules
)

// Inject the component
func (c *myComponent) reportIssue(healthPlatform healthplatform.Component) {
    issue := healthplatform.Issue{
        ID:    "disk-space-low",
        Name:  "Disk Space Low",
        Extra: "Available space: 5GB",
    }

    err := healthPlatform.AddIssue(issue)
    if err != nil {
        log.Printf("Failed to add issue: %v", err)
    }
}
```

## Team

**Team**: agent-runtimes
