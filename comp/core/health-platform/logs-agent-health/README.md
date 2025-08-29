# Logs Agent Health Checker Sub-Component

This is a sub-component of the health platform that specifically checks for logs agent health issues.

## Overview

The Logs Agent Health Checker monitors logs agent capabilities and reports issues that may affect log collection performance, particularly focusing on:

- **File Tailing Disabled**: When Docker file tailing is disabled due to permission restrictions
- **Docker Accessibility**: Whether the Docker daemon is accessible
- **Logging Driver**: The Docker logging driver configuration

## Key Issue: Docker File Tailing Disabled

This sub-component specifically addresses the issue where Docker file tailing is enabled by default but won't work on host installs because `/var/lib/docker` is owned by the root group. This causes the agent to fall back to socket tailing, which can become problematic with high volume Docker logs.

## Usage

```go
import (
    logsagenthealthfx "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/fx"
    "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health"
)

// Add to your fx.Options
fx.Options(
    logsagenthealthfx.Module(),
    // ... other modules
)

// Inject the component
func (c *myComponent) checkLogsAgentHealth(logsAgentChecker logsagenthealth.Component) {
    issues, err := logsAgentChecker.CheckHealth(context.Background())
    if err != nil {
        log.Printf("Failed to check logs agent health: %v", err)
        return
    }

    for _, issue := range issues {
        log.Printf("Logs agent issue: %s - %s", issue.Name, issue.Extra)
    }
}
```

## Configuration

```yaml
# Health check interval for logs agent (default: 5 minutes)
health_platform:
  logs_agent:
    interval: 5m
```

## Integration with Parent Health Platform

This sub-component is designed to be registered with the parent health platform component, which will:

1. Start the logs agent health checker when the health platform starts
2. Collect issues from the logs agent health checker during health reports
3. Stop the logs agent health checker when the health platform stops

## Team

**Team**: agent-runtimes
