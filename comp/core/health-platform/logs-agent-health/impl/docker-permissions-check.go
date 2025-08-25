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
	log.Info("Creating new Docker permissions check instance")
	return &DockerPermissionsCheck{}
}

// Name returns the name of this sub-check
func (d *DockerPermissionsCheck) Name() string {
	log.Debug("Returning Docker permissions check name")
	return "docker-permissions"
}

// Check performs Docker file tailing permission health checks
func (d *DockerPermissionsCheck) Check(_ context.Context) ([]healthplatform.Issue, error) {
	log.Info("Starting Docker permissions health check")
	var issues []healthplatform.Issue

	// Check Docker file tailing permissions
	log.Info("Checking Docker file tailing permissions")
	if issue := d.checkDockerFileTailing(); issue != nil {
		log.Infof("Docker file tailing issue detected: %s", issue.ID)
		issues = append(issues, *issue)
	} else {
		log.Info("No Docker file tailing issues detected")
	}

	log.Infof("Docker permissions check completed with %d issues found", len(issues))
	return issues, nil
}

// checkDockerFileTailing checks if Docker file tailing is disabled due to permission issues
func (d *DockerPermissionsCheck) checkDockerFileTailing() *healthplatform.Issue {
	log.Info("Checking Docker file tailing access and permissions")

	// Check if Docker logs directory exists
	log.Infof("Checking if Docker logs directory exists: %s", DockerLogsDir)
	if _, err := os.Stat(DockerLogsDir); os.IsNotExist(err) {
		log.Infof("Docker logs directory does not exist: %s - no issues to report", DockerLogsDir)
		// Docker logs directory doesn't exist, no issue to report
		return nil
	}
	log.Info("Docker logs directory exists, proceeding with permission checks")

	// Check if the current process has read access to the directory
	log.Info("Checking read access to Docker logs directory")
	if _, err := os.Open(DockerLogsDir); err != nil {
		log.Infof("Failed to open Docker logs directory: %v", err)
		// Check if this is a permission denied error
		if strings.Contains(err.Error(), "permission denied") {
			log.Warn("Permission denied when accessing Docker logs directory - creating issue")
			return d.createDockerFileTailingIssue()
		}
		log.Infof("Error opening Docker logs directory (not permission related): %v", err)
	} else {
		log.Info("Successfully opened Docker logs directory")
	}

	// Check if we can actually read Docker log files
	log.Info("Checking ability to read Docker log files")
	if !d.canReadDockerLogs() {
		log.Warn("Cannot read Docker log files - creating issue")
		return d.createDockerFileTailingIssue()
	}
	log.Info("Successfully verified ability to read Docker log files")

	return nil
}

// createDockerFileTailingIssue creates the issue for Docker file tailing being disabled
func (d *DockerPermissionsCheck) createDockerFileTailingIssue() *healthplatform.Issue {
	log.Info("Creating Docker file tailing disabled issue")
	issue := &healthplatform.Issue{
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
	log.Infof("Created Docker file tailing issue with ID: %s", issue.ID)
	return issue
}

// canReadDockerLogs checks if we can read Docker log files
func (d *DockerPermissionsCheck) canReadDockerLogs() bool {
	log.Info("Attempting to read Docker log files to verify access")

	// Try to find and read a Docker log file
	log.Infof("Reading directory contents of: %s", DockerLogsDir)
	entries, err := os.ReadDir(DockerLogsDir)
	if err != nil {
		log.Infof("Could not read Docker logs directory: %v", err)
		return false
	}
	log.Infof("Found %d entries in Docker logs directory", len(entries))

	// Look for container directories
	log.Info("Searching for container directories")
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "containers") {
			log.Infof("Found containers directory: %s", entry.Name())
			containerDir := filepath.Join(DockerLogsDir, entry.Name())
			containerEntries, err := os.ReadDir(containerDir)
			if err != nil {
				log.Infof("Could not read containers directory %s: %v", containerDir, err)
				continue
			}
			log.Infof("Found %d entries in containers directory", len(containerEntries))

			// Look for actual container directories
			log.Info("Searching for individual container directories")
			for _, containerEntry := range containerEntries {
				if containerEntry.IsDir() && len(containerEntry.Name()) == 64 { // Docker container IDs are 64 chars
					log.Debugf("Found potential container directory: %s", containerEntry.Name())
					logFile := filepath.Join(containerDir, containerEntry.Name(), containerEntry.Name()+"-json.log")
					log.Infof("Attempting to open log file: %s", logFile)
					if _, err := os.Open(logFile); err == nil {
						log.Infof("Successfully opened Docker log file: %s", logFile)
						return true
					} else {
						log.Debugf("Could not open log file %s: %v", logFile, err)
					}
				}
			}
		}
	}

	log.Info("No accessible Docker log files found")
	return false
}
