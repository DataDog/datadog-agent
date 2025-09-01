// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealthimpl provides the implementation for the logs agent health checker sub-component.
package logsagenthealthimpl

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DockerLogsDir is the default Docker logs directory
	DockerLogsDir = "/var/lib/docker"
	// DockerSocketPath is the default Docker socket path
	DockerSocketPath = "/var/run/docker.sock"

	// IssueIDDockerFileTailingDisabled is the ID for the Docker file tailing disabled issue
	IssueIDDockerFileTailingDisabled = "docker-file-tailing-disabled"
	// IssueIDDockerSocketInaccessible is the ID for the Docker socket inaccessible issue
	IssueIDDockerSocketInaccessible = "docker-socket-inaccessible"
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

// Check performs Docker file tailing permission health checks
func (d *DockerPermissionsCheck) Check(_ context.Context) ([]healthplatform.Issue, error) {
	var issues []healthplatform.Issue

	// Check Docker socket accessibility
	if issue := d.checkDockerSocketAccess(); issue != nil {
		issues = append(issues, *issue)
	}

	// Check Docker file tailing permissions
	if issue := d.checkDockerFileTailing(); issue != nil {
		issues = append(issues, *issue)
	}

	return issues, nil
}

// checkDockerSocketAccess checks if the Docker socket is accessible
func (d *DockerPermissionsCheck) checkDockerSocketAccess() *healthplatform.Issue {
	// Check if Docker socket exists
	if _, err := os.Stat(DockerSocketPath); os.IsNotExist(err) {
		return nil
	}

	// Check if we can connect to the Docker socket
	conn, err := net.DialTimeout("unix", DockerSocketPath, 500*time.Millisecond)
	if err != nil {
		// Check if this is a permission denied error
		if strings.Contains(err.Error(), "permission denied") {
			return d.createDockerIssue("socket")
		}
		return nil
	}

	if conn != nil {
		conn.Close()
	}

	return nil
}

// createDockerIssue creates a generic Docker permission issue
func (d *DockerPermissionsCheck) createDockerIssue(issueType string) *healthplatform.Issue {
	var id, name, title, description, extra, integrationFeature, severity, remediationSummary, scriptFilename, scriptContent string
	var remediationSteps []healthplatform.RemediationStep
	var tags []string

	switch issueType {
	case "socket":
		id = IssueIDDockerSocketInaccessible
		name = "docker_socket_inaccessible"
		title = "Docker Socket Not Accessible"
		description = fmt.Sprintf("The agent cannot access the Docker socket at %s due to permission restrictions. This prevents the agent from collecting Docker metrics and container information.", DockerSocketPath)
		severity = "high"
		extra = fmt.Sprintf("Docker socket %s is not accessible due to permission restrictions. The agent cannot collect Docker metrics or container information.", DockerSocketPath)
		integrationFeature = "docker"
		remediationSummary = "Add the dd-agent user to the docker group to enable Docker socket access"
		remediationSteps = []healthplatform.RemediationStep{
			{Order: 1, Text: "Add the dd-agent user to the docker group: usermod -aG docker dd-agent"},
			{Order: 2, Text: "Verify the user was added to the docker group: groups dd-agent"},
			{Order: 3, Text: "Restart the datadog-agent service: systemctl restart datadog-agent"},
			{Order: 4, Text: "Verify Docker socket access by checking agent logs"},
		}
		scriptFilename = "fix_docker_socket_access.sh"
		scriptContent = "usermod -aG docker dd-agent && systemctl restart datadog-agent"
		tags = []string{"docker", "socket", "permissions", "access-control", "metrics"}

	case "file-tailing":
		id = IssueIDDockerFileTailingDisabled
		name = "docker_file_tailing_disabled"
		title = "Host Agent Cannot Tail Docker Log Files"
		description = fmt.Sprintf("Docker file tailing is enabled by default but cannot work on this host install. The directory %s is owned by the root group, causing the agent to fall back to socket tailing. This becomes problematic with high volume Docker logs as socket tailing can hit limits.", DockerLogsDir)
		severity = "medium"
		extra = fmt.Sprintf("Docker logs directory %s is not accessible due to permission restrictions. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir)
		integrationFeature = "logs"
		remediationSummary = "⚠️  WARNING: Add the dd-agent user to the root group to enable Docker file tailing. This gives the agent root privileges and should only be used when necessary."
		remediationSteps = []healthplatform.RemediationStep{
			{Order: 1, Text: "⚠️  WARNING: Adding the dd-agent user to the root group gives the agent root privileges. This should only be done when Docker file tailing is essential and socket tailing is hitting limits."},
			{Order: 2, Text: "Add the dd-agent user to the root group: usermod -aG root dd-agent"},
			{Order: 3, Text: "Verify the user was added to the root group: groups dd-agent"},
			{Order: 4, Text: "Restart the datadog-agent service: systemctl restart datadog-agent"},
			{Order: 5, Text: "Verify Docker file tailing is working by checking agent logs"},
		}
		scriptFilename = "fix_docker_file_tailing.sh"
		scriptContent = "usermod -aG root dd-agent && systemctl restart datadog-agent"
		tags = []string{"docker", "logs", "permissions", "file-tailing", "socket-tailing", "host-install"}

	default:
		log.Errorf("Unknown Docker issue type: %s", issueType)
		return nil
	}

	issue := &healthplatform.Issue{
		ID:                 id,
		IssueName:          name,
		Title:              title,
		Description:        description,
		Category:           "permissions",
		Location:           "logs-agent",
		Severity:           severity,
		DetectedAt:         "", // Will be filled by the platform
		Integration:        nil,
		Extra:              extra,
		IntegrationFeature: integrationFeature,
		Remediation: &healthplatform.Remediation{
			Summary: remediationSummary,
			Steps:   remediationSteps,
			Script: &healthplatform.Script{
				Language:     "bash",
				Filename:     scriptFilename,
				RequiresRoot: true,
				Content:      scriptContent,
			},
		},
		Tags: tags,
	}

	return issue
}

// checkDockerFileTailing checks if Docker file tailing is disabled due to permission issues
func (d *DockerPermissionsCheck) checkDockerFileTailing() *healthplatform.Issue {
	// Check if Docker logs directory exists
	if _, err := os.Stat(DockerLogsDir); os.IsNotExist(err) {
		// Docker logs directory doesn't exist, no issue to report
		return nil
	}

	// Check if the current process has read access to the directory
	if _, err := os.Open(DockerLogsDir); err != nil {
		// Check if this is a permission denied error
		if strings.Contains(err.Error(), "permission denied") {
			return d.createDockerIssue("file-tailing")
		}
	}

	// Check if we can actually read Docker log files
	if !d.canReadDockerLogs() {
		return d.createDockerIssue("file-tailing")
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
