// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerpermissions provides remediation for Docker log file permission issues.
package dockerpermissions

import (
	_ "embed"
	"fmt"
	"strings"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	template "github.com/DataDog/datadog-agent/pkg/template/text"
)

//go:embed fix-docker-log-permissions.sh
var linuxScriptTemplate string

//go:embed Fix-DockerLogPermissions.ps1
var windowsScriptTemplate string

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
		Extra: map[any]any{
			"integration": "docker",
			"dir_path":    dockerDir,
			"os":          osName,
			"impact":      "The agent will fall back to socket tailing, which may hit limits with high volume logs",
		},
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
	scriptContent := renderTemplate(linuxScriptTemplate, dockerDir)

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
			Content:         scriptContent,
		},
	}
}

// buildWindows creates Windows-specific remediation steps
func (t *DockerPermissionIssue) buildWindows(dockerDir string) *healthplatform.Remediation {
	scriptContent := renderTemplate(windowsScriptTemplate, dockerDir)

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
			Content:         scriptContent,
		},
	}
}

// renderTemplate renders a script template with the given dockerDir
func renderTemplate(templateStr, dockerDir string) string {
	tmpl, err := template.New("script").Parse(templateStr)
	if err != nil {
		// Fallback to basic string replacement if template parsing fails
		return strings.ReplaceAll(templateStr, "{{.DockerDir}}", dockerDir)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, struct{ DockerDir string }{DockerDir: dockerDir})
	if err != nil {
		// Fallback to basic string replacement if template execution fails
		return strings.ReplaceAll(templateStr, "{{.DockerDir}}", dockerDir)
	}

	return result.String()
}
