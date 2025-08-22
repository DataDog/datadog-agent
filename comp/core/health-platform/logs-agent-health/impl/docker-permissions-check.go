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

	// Check Docker file tailing permissions
	if issue := d.checkDockerFileTailing(); issue != nil {
		issues = append(issues, *issue)
	}

	return issues, nil
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
			return d.createDockerFileTailingIssue()
		}
	}

	// Check if we can actually read Docker log files
	if !d.canReadDockerLogs() {
		return d.createDockerFileTailingIssue()
	}

	return nil
}

// createDockerFileTailingIssue creates the issue for Docker file tailing being disabled
func (d *DockerPermissionsCheck) createDockerFileTailingIssue() *healthplatform.Issue {
	return &healthplatform.Issue{
		ID:                 IssueIDDockerFileTailingDisabled,
		IssueName:          "docker_file_tailing_disabled",
		Title:              "Host Agent Cannot Tail Docker Log Files",
		Description:        fmt.Sprintf("Docker file tailing is enabled by default but cannot work on this host install. The directory %s is owned by the root group, causing the agent to fall back to socket tailing. This becomes problematic with high volume Docker logs as socket tailing can hit limits.", DockerLogsDir),
		Category:           "permissions",
		Location:           "logs-agent",
		Severity:           "medium",
		DetectedAt:         "", // Will be filled by the platform
		Integration:        nil,
		Extra:              fmt.Sprintf("Docker logs directory %s is not accessible due to permission restrictions. The agent will fall back to socket tailing, which may hit limits with high volume logs.", DockerLogsDir),
		IntegrationFeature: "logs",
		Remediation: &healthplatform.Remediation{
			Summary: "⚠️  WARNING: Add the dd-agent user to the root group to enable Docker file tailing. This gives the agent root privileges and should only be used when necessary.",
			Steps: []healthplatform.RemediationStep{
				{Order: 1, Text: "⚠️  WARNING: Adding the dd-agent user to the root group gives the agent root privileges. This should only be done when Docker file tailing is essential and socket tailing is hitting limits."},
				{Order: 2, Text: "Add the dd-agent user to the root group: usermod -aG root dd-agent"},
				{Order: 3, Text: "Verify the user was added to the root group: groups dd-agent"},
				{Order: 4, Text: "Restart the datadog-agent service: systemctl restart datadog-agent"},
				{Order: 5, Text: "Verify Docker file tailing is working by checking agent logs"},
			},
			Script: &healthplatform.Script{
				Language:     "bash",
				Filename:     "fix_docker_file_tailing.sh",
				RequiresRoot: true,
				Content:      "usermod -aG root dd-agent && systemctl restart datadog-agent",
			},
		},
		Tags: []string{"docker", "logs", "permissions", "file-tailing", "socket-tailing", "host-install"},
	}
}

// canReadDockerLogs checks if we can read Docker log files
func (d *DockerPermissionsCheck) canReadDockerLogs() bool {
	// Try to find and read a Docker log file
	entries, err := os.ReadDir(DockerLogsDir)
	if err != nil {
		log.Debugf("Could not read Docker logs directory: %v", err)
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
