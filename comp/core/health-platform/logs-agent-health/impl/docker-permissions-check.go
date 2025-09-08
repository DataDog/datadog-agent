// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealthimpl provides the implementation for the logs agent health checker sub-component.
package logsagenthealthimpl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// isPermissionError checks if an error is permission-related using proper error type checking
func isPermissionError(err error) bool {
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
}

// pingDocker performs an actual HTTP GET /_ping via the unix socket to test Docker API access
func pingDocker(sockPath string, timeout time.Duration) bool {
	dial := func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.DialTimeout("unix", sockPath, timeout/2)
	}
	tr := &http.Transport{DialContext: dial}
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: timeout}

	req, err := http.NewRequest("GET", "http://unix/_ping", nil)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		// Only treat permission errors as failures, other errors might be temporary
		return !isPermissionError(err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
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

// resolveDockerSocketPath resolves the Docker socket path from configuration or environment
func (d *DockerPermissionsCheck) resolveDockerSocketPath() string {
	// Check environment variable first
	if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost != "" {
		if strings.HasPrefix(dockerHost, "unix://") {
			return strings.TrimPrefix(dockerHost, "unix://")
		}
	}

	// TODO: Add support for reading from Agent config when available
	// For now, use the default path
	return DockerSocketPath
}

// checkDockerSocketAccess checks if the Docker socket is accessible
func (d *DockerPermissionsCheck) checkDockerSocketAccess() *healthplatform.Issue {
	sockPath := d.resolveDockerSocketPath()

	// Check if Docker socket exists
	if _, err := os.Stat(sockPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Socket doesn't exist, not a permission issue
			return nil
		}
		// If it's a permission error on the path itself, report it
		if isPermissionError(err) {
			return d.createDockerIssue("socket")
		}
		return nil
	}

	// Try a proper Docker API ping instead of just socket connection
	if !pingDocker(sockPath, 700*time.Millisecond) {
		// Only flag if the failure is permission-related
		// pingDocker already handles permission error detection
		return d.createDockerIssue("socket")
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
	if _, err := os.Stat(DockerLogsDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Docker logs directory doesn't exist, no issue to report
			return nil
		}
		// If it's a permission error, report it
		if isPermissionError(err) {
			return d.createDockerIssue("file-tailing")
		}
		return nil
	}

	// Check if the current process has read access to the directory
	if _, err := os.Open(DockerLogsDir); err != nil {
		// Check if this is a permission denied error using proper error checking
		if isPermissionError(err) {
			return d.createDockerIssue("file-tailing")
		}
		return nil
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
