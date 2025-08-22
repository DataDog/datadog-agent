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

	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DockerLogsDir is the default Docker logs directory
	DockerLogsDir = "/var/lib/docker"

	// IssueIDDockerFileTailingDisabled is the ID for the Docker file tailing disabled issue
	IssueIDDockerFileTailingDisabled = "docker-file-tailing-disabled"
	// IssueIDDockerNotAccessible is the ID for Docker daemon not accessible issue
	IssueIDDockerNotAccessible = "docker-not-accessible"
	// IssueIDDockerNonJSONLogging is the ID for Docker non-JSON logging driver issue
	IssueIDDockerNonJSONLogging = "docker-non-json-logging"
)

// DockerPermissionsCheck groups all Docker-related permission and access checks
type DockerPermissionsCheck struct{}

// NewDockerPermissionsCheck creates a new Docker permissions check
func NewDockerPermissionsCheck() *DockerPermissionsCheck {
	return &DockerPermissionsCheck{}
}

// Name returns the name of this sub-check
func (d *DockerPermissionsCheck) Name() string {
	return "docker-permissions"
}

// Check performs all Docker-related permission and access health checks
func (d *DockerPermissionsCheck) Check(ctx context.Context) ([]healthplatform.Issue, error) {
	var issues []healthplatform.Issue

	// Check Docker daemon accessibility
	if issue := d.checkDockerAccessibility(ctx); issue != nil {
		issues = append(issues, *issue)
	}

	// Only check file-related permissions if Docker is accessible
	if len(issues) == 0 {
		// Check Docker file tailing permissions
		if issue := d.checkDockerFileTailing(); issue != nil {
			issues = append(issues, *issue)
		}

		// Check Docker logging driver configuration
		if issue := d.checkDockerLoggingDriver(ctx); issue != nil {
			issues = append(issues, *issue)
		}
	}

	return issues, nil
}

// checkDockerAccessibility checks if Docker is running and accessible
func (d *DockerPermissionsCheck) checkDockerAccessibility(ctx context.Context) *healthplatform.Issue {
	// Try to run a simple Docker command
	cmd := exec.CommandContext(ctx, "docker", "version")
	if err := cmd.Run(); err != nil {
		return &healthplatform.Issue{
			ID:                 IssueIDDockerNotAccessible,
			IssueName:          "docker_not_accessible",
			Title:              "Docker Daemon Not Accessible",
			Description:        "Docker daemon is not accessible. Log collection may be affected.",
			Category:           "connectivity",
			Location:           "logs-agent",
			Severity:           "high",
			DetectedAt:         "", // Will be filled by the platform
			Integration:        nil,
			Extra:              "Docker daemon is not accessible. Log collection may be affected.",
			IntegrationFeature: "logs",
			Remediation: &healthplatform.Remediation{
				Summary: "Check Docker daemon status and ensure it's running and accessible.",
				Steps: []healthplatform.RemediationStep{
					{Order: 1, Text: "Check if Docker service is running: systemctl status docker"},
					{Order: 2, Text: "Verify Docker socket permissions: ls -la /var/run/docker.sock"},
					{Order: 3, Text: "Ensure user has access to Docker group: groups $USER"},
				},
				Script: &healthplatform.Script{
					Language:     "bash",
					Filename:     "fix_docker_access.sh",
					RequiresRoot: true,
					Content:      "systemctl start docker && usermod -aG docker $USER && systemctl restart docker",
				},
			},
			Tags: []string{"docker", "connectivity", "logs", "urgent"},
		}
	}
	return nil
}

// checkDockerFileTailing checks if Docker file tailing is disabled due to permission issues
func (d *DockerPermissionsCheck) checkDockerFileTailing() *healthplatform.Issue {
	// Check if Docker logs directory exists and check permissions
	if _, err := os.Stat(DockerLogsDir); os.IsNotExist(err) {
		// Docker logs directory doesn't exist
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
			return &healthplatform.Issue{
				ID:                 IssueIDDockerFileTailingDisabled,
				IssueName:          "docker_logs_readable",
				Title:              "Agent Cannot Read Docker Log Files",
				Description:        fmt.Sprintf("Docker logs directory %s is not accessible due to permission restrictions. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir),
				Category:           "permissions",
				Location:           "logs-agent",
				Severity:           "medium",
				DetectedAt:         "", // Will be filled by the platform
				Integration:        nil,
				Extra:              fmt.Sprintf("Docker logs directory %s is not accessible due to permission restrictions. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir),
				IntegrationFeature: "logs",
				Remediation: &healthplatform.Remediation{
					Summary: "Grant the agent read access to Docker logs and restart the agent.",
					Steps: []healthplatform.RemediationStep{
						{Order: 1, Text: "Add the agent user to the docker group: usermod -aG docker $USER"},
						{Order: 2, Text: "Ensure log file permissions allow group read: chmod -R g+rX /var/lib/docker/containers"},
						{Order: 3, Text: "Restart the log-agent service: systemctl restart datadog-agent"},
					},
					Script: &healthplatform.Script{
						Language:     "bash",
						Filename:     "fix_docker_log_access.sh",
						RequiresRoot: true,
						Content:      "usermod -aG docker $USER && chmod -R g+rX /var/lib/docker/containers && systemctl restart datadog-agent",
					},
				},
				Tags: []string{"docker", "logs", "permissions", "attention"},
			}
		}
	}

	// Check if we can read files in the directory
	if !d.canReadDockerLogs() {
		return &healthplatform.Issue{
			ID:                 IssueIDDockerFileTailingDisabled,
			IssueName:          "docker_logs_readable",
			Title:              "Agent Cannot Read Docker Log Files",
			Description:        fmt.Sprintf("Docker logs directory %s is not accessible. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir),
			Category:           "permissions",
			Location:           "logs-agent",
			Severity:           "medium",
			DetectedAt:         "", // Will be filled by the platform
			Integration:        nil,
			Extra:              fmt.Sprintf("Docker logs directory %s is not accessible. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir),
			IntegrationFeature: "logs",
			Remediation: &healthplatform.Remediation{
				Summary: "Grant the agent read access to Docker logs and restart the agent.",
				Steps: []healthplatform.RemediationStep{
					{Order: 1, Text: "Add the agent user to the docker group: usermod -aG docker $USER"},
					{Order: 2, Text: "Ensure log file permissions allow group read: chmod -R g+rX /var/lib/docker/containers"},
					{Order: 3, Text: "Restart the log-agent service: systemctl restart datadog-agent"},
				},
				Script: &healthplatform.Script{
					Language:     "bash",
					Filename:     "fix_docker_log_access.sh",
					RequiresRoot: true,
					Content:      "usermod -aG docker $USER && chmod -R g+rX /var/lib/docker/containers && systemctl restart datadog-agent",
				},
			},
			Tags: []string{"docker", "logs", "permissions", "attention"},
		}
	}

	return nil
}

// checkDockerLoggingDriver checks Docker logging driver configuration
func (d *DockerPermissionsCheck) checkDockerLoggingDriver(ctx context.Context) *healthplatform.Issue {
	// Check if Docker is configured to use json-file logging driver
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.LoggingDriver}}")
	output, err := cmd.Output()
	if err != nil {
		log.Debugf("Could not check Docker logging driver: %v", err)
		return nil
	}

	loggingDriver := strings.TrimSpace(string(output))
	if loggingDriver != "json-file" {
		return &healthplatform.Issue{
			ID:                 IssueIDDockerNonJSONLogging,
			IssueName:          "docker_non_json_logging",
			Title:              "Docker Using Non-JSON Logging Driver",
			Description:        fmt.Sprintf("Docker is using '%s' logging driver instead of 'json-file'. This may affect log collection capabilities.", loggingDriver),
			Category:           "configuration",
			Location:           "logs-agent",
			Severity:           "low",
			DetectedAt:         "", // Will be filled by the platform
			Integration:        nil,
			Extra:              fmt.Sprintf("Docker is using '%s' logging driver instead of 'json-file'. This may affect log collection capabilities.", loggingDriver),
			IntegrationFeature: "logs",
			Remediation: &healthplatform.Remediation{
				Summary: "Configure Docker to use json-file logging driver for optimal log collection.",
				Steps: []healthplatform.RemediationStep{
					{Order: 1, Text: "Edit Docker daemon configuration: /etc/docker/daemon.json"},
					{Order: 2, Text: "Add logging driver configuration: {\"log-driver\": \"json-file\"}"},
					{Order: 3, Text: "Restart Docker daemon: systemctl restart docker"},
				},
				Script: &healthplatform.Script{
					Language:     "bash",
					Filename:     "fix_docker_logging.sh",
					RequiresRoot: true,
					Content:      "echo '{\"log-driver\": \"json-file\"}' >> /etc/docker/daemon.json && systemctl restart docker",
				},
			},
			Tags: []string{"docker", "logs", "configuration", "info"},
		}
	}

	return nil
}

// canReadDockerLogs checks if we can read Docker log files
func (d *DockerPermissionsCheck) canReadDockerLogs() bool {
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
