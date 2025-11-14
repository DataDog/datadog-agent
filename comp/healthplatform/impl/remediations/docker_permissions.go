// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remediations

import (
	"fmt"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// DockerPermissionIssue provides complete issue template (metadata + OS-specific remediation)
type DockerPermissionIssue struct{}

// NewDockerPermissionIssue creates a new Docker permission issue template
func NewDockerPermissionIssue() *DockerPermissionIssue {
	return &DockerPermissionIssue{}
}

// BuildIssue creates a complete issue with metadata and OS-specific remediation
func (t *DockerPermissionIssue) BuildIssue(context map[string]string) *healthplatform.Issue {
	dockerDir := context["dockerDir"]
	if dockerDir == "" {
		dockerDir = "/var/lib/docker" // fallback
	}

	osName := context["os"]
	if osName == "" {
		osName = "linux" // fallback
	}

	return &healthplatform.Issue{
		ID:          "docker-file-tailing-disabled",
		IssueName:   "docker_file_tailing_disabled",
		Title:       "Host Agent Cannot Tail Docker Log Files",
		Description: fmt.Sprintf("Docker file tailing is enabled by default but cannot work on this host install. The directory %s has restricted permissions, causing the agent to fall back to socket tailing. This becomes problematic with high volume Docker logs as socket tailing can hit limits.", dockerDir),
		Category:    "permissions",
		Location:    "logs-agent",
		Severity:    "medium",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "logs",
		Extra:       fmt.Sprintf("Integration: docker. Docker logs directory %s is not accessible due to permission restrictions on %s. The agent will fall back to socket tailing, which may hit limits with high volume logs.", dockerDir, osName),
		Remediation: t.buildRemediation(dockerDir, osName),
		Tags:        []string{"docker", "logs", "permissions", "file-tailing", "socket-tailing", "host-install", osName},
	}
}

// buildRemediation creates OS-specific remediation
func (t *DockerPermissionIssue) buildRemediation(dockerDir, osName string) *healthplatform.Remediation {
	switch osName {
	case "windows":
		return t.buildWindows(dockerDir)
	default: // linux, darwin
		return t.buildLinux(dockerDir)
	}
}

// buildLinux creates Linux-specific remediation steps
func (t *DockerPermissionIssue) buildLinux(dockerDir string) *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Grant minimal access to Docker log files using ACLs (recommended) or add dd-agent to root group as last resort",
		Steps: []healthplatform.RemediationStep{
			{Order: 1, Text: "RECOMMENDED: Grant minimal access using ACLs (safer than root group):"},
			{Order: 2, Text: fmt.Sprintf("sudo setfacl -Rm g:dd-agent:rx %s/containers", dockerDir)},
			{Order: 3, Text: fmt.Sprintf("sudo setfacl -Rm g:dd-agent:r %s/containers/*/*.log", dockerDir)},
			{Order: 4, Text: fmt.Sprintf("sudo setfacl -Rdm g:dd-agent:rx %s/containers", dockerDir)},
			{Order: 5, Text: "Restart the datadog-agent service: systemctl restart datadog-agent"},
			{Order: 6, Text: "Verify Docker file tailing is working by checking agent logs"},
			{Order: 7, Text: "⚠️  LAST RESORT: If ACLs don't work, add dd-agent to root group (gives root privileges):"},
			{Order: 8, Text: "usermod -aG root dd-agent && systemctl restart datadog-agent"},
		},
		Script: &healthplatform.Script{
			Language:        "bash",
			LanguageVersion: "4.0+",
			Filename:        "fix-docker-log-permissions.sh",
			RequiresRoot:    true,
			Content: fmt.Sprintf(`#!/bin/bash
# Fix Docker log file permissions for Datadog Agent
# This grants the dd-agent user read access to Docker log files using ACLs

set -e

DOCKER_DIR="%s"

echo "Granting dd-agent read access to Docker logs..."
setfacl -Rm g:dd-agent:rx "$DOCKER_DIR/containers"
setfacl -Rm g:dd-agent:r "$DOCKER_DIR/containers"/*/*.log
setfacl -Rdm g:dd-agent:rx "$DOCKER_DIR/containers"

echo "Restarting Datadog Agent..."
systemctl restart datadog-agent

echo "Done! Check agent status with: datadog-agent status"
`, dockerDir),
		},
	}
}

// buildWindows creates Windows-specific remediation steps
func (t *DockerPermissionIssue) buildWindows(dockerDir string) *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Grant read access to Docker log files for the ddagentuser account",
		Steps: []healthplatform.RemediationStep{
			{Order: 1, Text: "Open PowerShell as Administrator"},
			{Order: 2, Text: fmt.Sprintf("Grant read permissions to ddagentuser: icacls \"%s\\containers\" /grant ddagentuser:(OI)(CI)RX /T", dockerDir)},
			{Order: 3, Text: "Restart the Datadog Agent service: Restart-Service -Name datadogagent"},
			{Order: 4, Text: "Verify Docker file tailing is working by checking agent logs"},
			{Order: 5, Text: "Alternative: Use the Services management console (services.msc) to restart 'Datadog Agent'"},
		},
		Script: &healthplatform.Script{
			Language:        "powershell",
			LanguageVersion: "5.1+",
			Filename:        "Fix-DockerLogPermissions.ps1",
			RequiresRoot:    true,
			Content: fmt.Sprintf(`# Fix Docker log file permissions for Datadog Agent
# This grants the ddagentuser read access to Docker log files

$dockerDir = "%s"
$containerPath = Join-Path $dockerDir "containers"

Write-Host "Granting ddagentuser read access to Docker logs..."
icacls "$containerPath" /grant ddagentuser:(OI)(CI)RX /T

Write-Host "Restarting Datadog Agent..."
Restart-Service -Name datadogagent -Force

Write-Host "Done! Check agent status with: & 'C:\Program Files\Datadog\Datadog Agent\bin\agent.exe' status"
`, dockerDir),
		},
	}
}
