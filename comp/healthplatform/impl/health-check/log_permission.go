// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthcheck contains the implementation of the health check for the log permissions
package healthcheck

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

const (
	// DockerLogsDir is the default Docker logs directory
	DockerLogsDir = "/var/lib/docker"

	// IssueIDDockerFileTailingDisabled is the ID for the Docker file tailing disabled issue
	IssueIDDockerFileTailingDisabled = "docker-file-tailing-disabled"
)

// NewDockerLogPermissionsCheckConfig creates a CheckConfig for Docker log permissions
func NewDockerLogPermissionsCheckConfig() healthplatform.CheckConfig {
	return healthplatform.CheckConfig{
		CheckName: "docker-log-permissions",
		CheckID:   "docker-log-permissions-check",
		Callback:  CheckDockerPermissions,
	}
}

// isPermissionError checks if an error is permission-related using proper error type checking
func isPermissionError(err error) bool {
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
}

// CheckDockerPermissions performs Docker file tailing permission health checks
func CheckDockerPermissions() ([]healthplatform.Issue, error) {
	var issues []healthplatform.Issue

	// Check Docker file tailing permissions
	if issue := checkDockerFileTailing(); issue != nil {
		issues = append(issues, *issue)
	}

	return issues, nil
}

// checkDockerFileTailing checks if Docker file tailing permissions are working
func checkDockerFileTailing() *healthplatform.Issue {
	// Check if Docker logs directory exists and is accessible
	if _, err := os.Stat(DockerLogsDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Directory doesn't exist, not a permission issue
			return nil
		}
		// If it's a permission error, report it
		if isPermissionError(err) {
			return createDockerIssue("file-tailing")
		}
		return nil
	}

	// Try to access the containers subdirectory
	containersDir := DockerLogsDir + "/containers"
	if _, err := os.Stat(containersDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No containers directory, not a permission issue
			return nil
		}
		// If it's a permission error, report it
		if isPermissionError(err) {
			return createDockerIssue("file-tailing")
		}
		return nil
	}

	// Try to read the containers directory
	if _, err := os.ReadDir(containersDir); err != nil {
		if isPermissionError(err) {
			return createDockerIssue("file-tailing")
		}
		return nil
	}

	return nil
}

// createDockerIssue creates a Docker file tailing permission issue
func createDockerIssue(issueType string, dockerRootDir ...string) *healthplatform.Issue {
	// Use provided Docker root dir or fall back to default
	dockerDir := DockerLogsDir
	if len(dockerRootDir) > 0 && dockerRootDir[0] != "" {
		dockerDir = dockerRootDir[0]
	}

	// Only handle file-tailing case now
	if issueType != "file-tailing" {
		// Log error using fmt since we don't have access to logger here
		fmt.Printf("Unknown Docker issue type: %s\n", issueType)
		return nil
	}

	id := IssueIDDockerFileTailingDisabled
	name := "docker_file_tailing_disabled"
	title := "Host Agent Cannot Tail Docker Log Files"
	description := fmt.Sprintf("Docker file tailing is enabled by default but cannot work on this host install. The directory %s is owned by the root group, causing the agent to fall back to socket tailing. This becomes problematic with high volume Docker logs as socket tailing can hit limits.", dockerDir)
	severity := "medium"
	extra := fmt.Sprintf("Docker logs directory %s is not accessible due to permission restrictions. The agent will fall back to socket tailing, which may hit limits with high volume logs.", dockerDir)
	integrationFeature := "logs"
	remediationSummary := "Grant minimal access to Docker log files using ACLs (recommended) or add dd-agent to root group as last resort"
	remediationSteps := []healthplatform.RemediationStep{
		{Order: 1, Text: "RECOMMENDED: Grant minimal access using ACLs (safer than root group):"},
		{Order: 2, Text: fmt.Sprintf("sudo setfacl -Rm g:dd-agent:rx %s/containers", dockerDir)},
		{Order: 3, Text: fmt.Sprintf("sudo setfacl -Rm g:dd-agent:r %s/containers/*/*.log", dockerDir)},
		{Order: 4, Text: fmt.Sprintf("sudo setfacl -Rdm g:dd-agent:rx %s/containers", dockerDir)},
		{Order: 5, Text: "Restart the datadog-agent service: systemctl restart datadog-agent"},
		{Order: 6, Text: "Verify Docker file tailing is working by checking agent logs"},
		{Order: 7, Text: "⚠️  LAST RESORT: If ACLs don't work, add dd-agent to root group (gives root privileges):"},
		{Order: 8, Text: "usermod -aG root dd-agent && systemctl restart datadog-agent"},
	}
	scriptFilename := "update-agent-perm-2"
	scriptContent := fmt.Sprintf("setfacl -Rm g:dd-agent:rx %s/containers && setfacl -Rm g:dd-agent:r %s/containers/*/*.log && setfacl -Rdm g:dd-agent:rx %s/containers && systemctl restart datadog-agent", dockerDir, dockerDir, dockerDir)
	tags := []string{"docker", "logs", "permissions", "file-tailing", "socket-tailing", "host-install"}

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
