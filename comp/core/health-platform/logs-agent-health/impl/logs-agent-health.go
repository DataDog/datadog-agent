// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealthimpl provides the implementation for the logs agent health checker sub-component.
package logsagenthealthimpl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logsagenthealth "github.com/DataDog/datadog-agent/comp/core/health-platform/logs-agent-health/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultCheckInterval is the default interval for health checks
	DefaultCheckInterval = 5 * time.Minute

	// DockerLogsDir is the default Docker logs directory
	DockerLogsDir = "/var/lib/docker"

	// IssueIDDockerFileTailingDisabled is the ID for the Docker file tailing disabled issue
	IssueIDDockerFileTailingDisabled = "docker-file-tailing-disabled"
)

// Dependencies lists the dependencies for the logs agent health checker
type Dependencies struct {
	Config config.Component
}

// component implements the logs agent health checker sub-component
type component struct {
	cfg config.Component

	// health checking control
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// check interval
	interval time.Duration

	// mutex for thread safety
	mu sync.RWMutex
}

// NewComponent creates a new logs agent health checker component
func NewComponent(deps Dependencies) logsagenthealth.Component {
	return &component{
		cfg:      deps.Config,
		interval: DefaultCheckInterval,
	}
}

// isLogsAgentEnabled checks if the logs agent is enabled in the configuration
func (c *component) isLogsAgentEnabled() bool {
	if c.cfg == nil {
		return false
	}

	// Check both the current and deprecated config keys
	return c.cfg.GetBool("logs_enabled") || c.cfg.GetBool("log_enabled")
}

// CheckHealth performs health checks related to logs agent health
func (c *component) CheckHealth(_ context.Context) ([]logsagenthealth.Issue, error) {
	var issues []logsagenthealth.Issue

	// Check if logs agent is enabled before running health checks
	if !c.isLogsAgentEnabled() {
		return issues, nil // No issues to report if logs agent is disabled
	}

	// Check if Docker file tailing is disabled due to permission issues
	if issue := c.checkDockerFileTailing(); issue != nil {
		issues = append(issues, *issue)
	}

	// Check if Docker is running and accessible
	if issue := c.checkDockerAccessibility(); issue != nil {
		issues = append(issues, *issue)
	}

	// Check Docker log volume and performance
	if issue := c.checkDockerLogVolume(); issue != nil {
		issues = append(issues, *issue)
	}

	return issues, nil
}

// checkDockerFileTailing checks if Docker file tailing is disabled due to permission issues
func (c *component) checkDockerFileTailing() *logsagenthealth.Issue {
	// Check if Docker logs directory exists and check permissions
	if _, err := os.Stat(DockerLogsDir); os.IsNotExist(err) {
		// Docker logs directory doesn't exist, this is not a host install
		return nil
	}

	// Check if the agent is running as root or has access to Docker logs
	if _, err := os.Stat(DockerLogsDir); err != nil {
		log.Debugf("Could not stat Docker logs directory: %v", err)
		return nil
	}

	// Check if the current process has read access to the directory
	if _, err := os.Open(DockerLogsDir); err != nil {
		// Check if this is a host install with permission issues
		if strings.Contains(err.Error(), "permission denied") {
			return &logsagenthealth.Issue{
				ID:       IssueIDDockerFileTailingDisabled,
				Name:     "Docker File Tailing Disabled",
				Extra:    fmt.Sprintf("Docker logs directory %s is not accessible due to permission restrictions. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir),
				Severity: logsagenthealth.SeverityMedium,
			}
		}
	}

	// Check if we can read files in the directory
	if !c.canReadDockerLogs() {
		return &logsagenthealth.Issue{
			ID:       IssueIDDockerFileTailingDisabled,
			Name:     "Docker File Tailing Disabled",
			Extra:    fmt.Sprintf("Docker logs directory %s is not accessible. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir),
			Severity: logsagenthealth.SeverityMedium,
		}
	}

	return nil
}

// checkDockerAccessibility checks if Docker is running and accessible
func (c *component) checkDockerAccessibility() *logsagenthealth.Issue {
	// Try to run a simple Docker command
	cmd := exec.CommandContext(context.Background(), "docker", "version")
	if err := cmd.Run(); err != nil {
		return &logsagenthealth.Issue{
			ID:       "docker-not-accessible",
			Name:     "Docker Not Accessible",
			Extra:    "Docker daemon is not accessible. Log collection may be affected.",
			Severity: logsagenthealth.SeverityHigh,
		}
	}
	return nil
}

// checkDockerLogVolume checks Docker log volume and performance
func (c *component) checkDockerLogVolume() *logsagenthealth.Issue {
	// Check if Docker is configured to use json-file logging driver
	cmd := exec.CommandContext(context.Background(), "docker", "info", "--format", "{{.LoggingDriver}}")
	output, err := cmd.Output()
	if err != nil {
		log.Debugf("Could not check Docker logging driver: %v", err)
		return nil
	}

	loggingDriver := strings.TrimSpace(string(output))
	if loggingDriver != "json-file" {
		return &logsagenthealth.Issue{
			ID:       "docker-non-json-logging",
			Name:     "Docker Non-JSON Logging Driver",
			Extra:    fmt.Sprintf("Docker is using '%s' logging driver instead of 'json-file'. This may affect log collection capabilities.", loggingDriver),
			Severity: logsagenthealth.SeverityLow,
		}
	}

	return nil
}

// canReadDockerLogs checks if we can read Docker log files
func (c *component) canReadDockerLogs() bool {
	// Try to find and read a Docker log file
	entries, err := os.ReadDir(DockerLogsDir)
	if err != nil {
		return false
	}

	// Look for container directories
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "containers") {
			containerDir := filepath.Join(DockerLogsDir, entry.Name())
			containerEntries, err := os.ReadDir(containerDir)
			if err != nil {
				continue
			}

			// Look for actual container directories
			for _, containerEntry := range containerEntries {
				if containerEntry.IsDir() && len(containerEntry.Name()) == 64 { // Docker container IDs are 64 chars
					logFile := filepath.Join(containerDir, containerEntry.Name(), containerEntry.Name()+"-json.log")
					if _, err := os.Open(logFile); err == nil {
						return true
					}
				}
			}
		}
	}

	return false
}

// Start begins periodic health checking
func (c *component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		return fmt.Errorf("Docker logs health checker is already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.done = make(chan struct{})

	// Get check interval from config
	if c.cfg != nil {
		if configObj := c.cfg.Object(); configObj != nil {
			if durGetter, ok := configObj.(interface{ GetDuration(string) time.Duration }); ok {
				if configInterval := durGetter.GetDuration("health_platform.logs_agent.interval"); configInterval > 0 {
					c.interval = configInterval
				}
			}
		}
	}

	go c.checkingLoop()
	log.Infof("Started Docker logs health checker with interval: %v", c.interval)
	return nil
}

// Stop stops periodic health checking
func (c *component) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel == nil {
		return fmt.Errorf("Docker logs health checker is not running")
	}

	c.cancel()
	<-c.done
	c.cancel = nil
	c.done = nil

	log.Info("Stopped Docker logs health checker")
	return nil
}

// checkingLoop handles the periodic health checking
func (c *component) checkingLoop() {
	defer close(c.done)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if issues, err := c.CheckHealth(c.ctx); err != nil {
				log.Warnf("Failed to perform Docker logs health check: %v", err)
			} else if len(issues) > 0 {
				log.Debugf("Docker logs health check found %d issues", len(issues))
			}
		}
	}
}
