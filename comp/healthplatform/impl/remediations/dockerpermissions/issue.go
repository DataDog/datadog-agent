// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerpermissions provides remediation for Docker permission issues.
package dockerpermissions

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	template "github.com/DataDog/datadog-agent/pkg/template/text"
)

//go:embed fix-docker-log-permissions.sh
var linuxScriptTemplate string

//go:embed Fix-DockerLogPermissions.ps1
var windowsScriptTemplate string

const (
	// IssueIDSocket is for Docker socket permission issues
	IssueIDSocket = "docker-socket-permission-denied"
	// IssueIDLogFiles is for Docker log file permission issues
	IssueIDLogFiles = "docker-file-tailing-disabled"
)

// DockerPermissionIssue provides complete issue template for Docker permission problems
type DockerPermissionIssue struct{}

// NewDockerPermissionIssue creates a new Docker permission issue template
func NewDockerPermissionIssue() *DockerPermissionIssue {
	return &DockerPermissionIssue{}
}

// issueBuilder is a function that builds a specific type of issue
type issueBuilder func(*DockerPermissionIssue, map[string]string, string) *healthplatform.Issue

// issueBuilders maps issue types to their builder functions
var issueBuilders = map[string]issueBuilder{
	"socket":    (*DockerPermissionIssue).buildSocketIssue,
	"log-files": (*DockerPermissionIssue).buildLogFilesIssue,
}

// BuildIssue creates a complete issue with metadata and OS-specific remediation
func (t *DockerPermissionIssue) BuildIssue(context map[string]string) *healthplatform.Issue {
	osName := context["os"]
	if osName == "" {
		osName = "linux"
	}

	if builder, ok := issueBuilders[context["type"]]; ok {
		return builder(t, context, osName)
	}
	return t.buildGenericIssue(context, osName)
}

// Helper functions

func isWindows(os string) bool {
	return os == "windows"
}

func newBaseIssue(id, name, title, desc, location, severity, source string, tags []string, extra map[string]any, remediation *healthplatform.Remediation) *healthplatform.Issue {
	return &healthplatform.Issue{
		ID:          id,
		IssueName:   name,
		Title:       title,
		Description: desc,
		Category:    "permissions",
		Location:    location,
		Tags:        tags,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: remediation,
	}
}

func buildRemediation(summary string, steps []string, script *healthplatform.Script) *healthplatform.Remediation {
	remSteps := make([]healthplatform.RemediationStep, len(steps))
	for i, text := range steps {
		remSteps[i] = healthplatform.RemediationStep{Order: i + 1, Text: text}
	}

	return &healthplatform.Remediation{
		Summary: summary,
		Steps:   remSteps,
		Script:  script,
	}
}

// Issue builders

// buildSocketIssue creates an issue for Docker socket permission problems
func (t *DockerPermissionIssue) buildSocketIssue(context map[string]string, osName string) *healthplatform.Issue {
	socketPath := context["socketPath"]
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}

	return newBaseIssue(
		IssueIDSocket,
		"docker_socket_permission_denied",
		"Agent Cannot Access Docker Socket",
		t.buildSocketDescription(socketPath, osName),
		"docker-integration",
		"high",
		"docker",
		[]string{"integration:docker", "docker:socket", "permissions", osName},
		map[string]any{
			"integration": "docker",
			"issue_type":  "socket-access",
			"socketPath":  socketPath,
			"os":          osName,
			"impact":      "Cannot discover containers, collect metrics, or access logs",
		},
		t.buildSocketRemediation(socketPath, osName),
	)
}

// buildLogFilesIssue creates an issue for Docker log file permission problems
func (t *DockerPermissionIssue) buildLogFilesIssue(context map[string]string, osName string) *healthplatform.Issue {
	dockerDir := context["dockerDir"]
	if dockerDir == "" {
		dockerDir = "/var/lib/docker"
	}

	return newBaseIssue(
		IssueIDLogFiles,
		"docker_file_tailing_disabled",
		"Host Agent Cannot Tail Docker Log Files",
		t.buildLogFilesDescription(dockerDir),
		"logs-agent",
		"medium",
		"logs",
		[]string{"docker", "logs", "permissions", "file-tailing", "socket-tailing", "host-install", osName},
		map[string]any{
			"integration": "docker",
			"issue_type":  "log-file-access",
			"dir_path":    dockerDir,
			"os":          osName,
			"impact":      "The agent will fall back to socket tailing, which may hit limits with high volume logs",
		},
		t.buildLogFilesRemediation(dockerDir, osName),
	)
}

// buildGenericIssue creates a generic Docker permission issue
func (t *DockerPermissionIssue) buildGenericIssue(_ map[string]string, osName string) *healthplatform.Issue {
	return newBaseIssue(
		"docker-permission-issue",
		"docker_permission_issue",
		"Docker Permission Issue",
		"The Datadog Agent cannot access Docker resources due to permission restrictions.",
		"docker-integration",
		"medium",
		"docker",
		[]string{"integration:docker", "permissions", osName},
		map[string]any{
			"integration": "docker",
			"os":          osName,
		},
		t.buildGenericRemediation(osName),
	)
}

// Description builders

// buildSocketDescription creates description for socket access issues
func (t *DockerPermissionIssue) buildSocketDescription(socketPath, osName string) string {
	base := fmt.Sprintf(
		"The Datadog Agent cannot access the Docker socket at '%s'. "+
			"This prevents the agent from discovering and monitoring Docker containers, "+
			"collecting Docker metrics, and accessing container logs.",
		socketPath,
	)

	if isWindows(osName) {
		return base
	}

	return base + " The 'dd-agent' user needs to be added to the 'docker' group."
}

// buildLogFilesDescription creates description for log file access issues
func (t *DockerPermissionIssue) buildLogFilesDescription(dockerDir string) string {
	return fmt.Sprintf(
		"Docker file tailing is enabled by default but cannot work on this host install. "+
			"The directory %s has restricted permissions, causing the agent to fall back to socket tailing. "+
			"This becomes problematic with high volume Docker logs as socket tailing can hit limits.",
		dockerDir,
	)
}

// Remediation builders

// buildSocketRemediation creates remediation for socket access issues
func (t *DockerPermissionIssue) buildSocketRemediation(socketPath, osName string) *healthplatform.Remediation {
	if isWindows(osName) {
		return buildRemediation(
			"Grant the Datadog Agent service account access to Docker named pipes",
			[]string{
				"Open Services (services.msc) as Administrator",
				"Find the 'Datadog Agent' service",
				"Right-click → Properties → Log On tab",
				"Ensure the service account has permissions to access Docker named pipes",
				"Restart the Datadog Agent service",
			},
			nil,
		)
	}

	return buildRemediation(
		"Grant the dd-agent user access to Docker by adding it to the docker group",
		[]string{
			"Add the dd-agent user to the docker group: `sudo usermod -aG docker dd-agent`",
			"Restart the Datadog Agent: `sudo systemctl restart datadog-agent`",
			fmt.Sprintf("Verify socket access: `sudo -u dd-agent test -r %s && echo OK || echo FAIL`", socketPath),
		},
		nil,
	)
}

// buildLogFilesRemediation creates remediation for log file access issues
func (t *DockerPermissionIssue) buildLogFilesRemediation(dockerDir, osName string) *healthplatform.Remediation {
	if isWindows(osName) {
		return buildRemediation(
			"Grant read access to Docker log files for the ddagentuser account",
			[]string{
				"Open PowerShell as Administrator",
				fmt.Sprintf("Grant read permissions to ddagentuser: icacls \"%s\\containers\" /grant ddagentuser:(OI)(CI)RX /T", dockerDir),
				"Restart the Datadog Agent service: Restart-Service -Name datadogagent",
				"Verify Docker file tailing is working by checking agent logs",
			},
			&healthplatform.Script{
				Language:        "powershell",
				LanguageVersion: "5.1+",
				Filename:        "Fix-DockerLogPermissions.ps1",
				RequiresRoot:    true,
				Content:         renderTemplate(windowsScriptTemplate, dockerDir),
			},
		)
	}

	return buildRemediation(
		"Grant minimal access to Docker log files using ACLs (recommended) or add dd-agent to docker group",
		[]string{
			"RECOMMENDED: Add dd-agent to docker group (grants socket + log access):",
			"sudo usermod -aG docker dd-agent",
			"sudo systemctl restart datadog-agent",
			"ALTERNATIVE: Grant minimal access using ACLs (log files only):",
			fmt.Sprintf("sudo setfacl -Rm g:dd-agent:rx %s/containers", dockerDir),
			fmt.Sprintf("sudo setfacl -Rm g:dd-agent:r %s/containers/*/*.log", dockerDir),
			fmt.Sprintf("sudo setfacl -Rdm g:dd-agent:rx %s/containers", dockerDir),
			"sudo systemctl restart datadog-agent",
		},
		&healthplatform.Script{
			Language:        "bash",
			LanguageVersion: "4.0+",
			Filename:        "fix-docker-log-permissions.sh",
			RequiresRoot:    true,
			Content:         renderTemplate(linuxScriptTemplate, dockerDir),
		},
	)
}

// buildGenericRemediation creates generic Docker permission remediation
func (t *DockerPermissionIssue) buildGenericRemediation(osName string) *healthplatform.Remediation {
	if isWindows(osName) {
		return buildRemediation(
			"Grant the Datadog Agent service account access to Docker",
			[]string{
				"Ensure the Datadog Agent service has appropriate permissions to access Docker",
				"Restart the Datadog Agent service",
			},
			nil,
		)
	}

	return buildRemediation(
		"Grant the dd-agent user access to Docker resources",
		[]string{
			"Add the dd-agent user to the docker group: `sudo usermod -aG docker dd-agent`",
			"Restart the Datadog Agent: `sudo systemctl restart datadog-agent`",
		},
		nil,
	)
}

// renderTemplate renders a script template with the given dockerDir
func renderTemplate(templateStr, dockerDir string) string {
	tmpl, err := template.New("script").Parse(templateStr)
	if err != nil {
		return strings.ReplaceAll(templateStr, "{{.DockerDir}}", dockerDir)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, struct{ DockerDir string }{DockerDir: dockerDir})
	if err != nil {
		return strings.ReplaceAll(templateStr, "{{.DockerDir}}", dockerDir)
	}

	return result.String()
}

// NewDockerSocketPermissionIssue creates a health issue report for Docker socket permission problems
func NewDockerSocketPermissionIssue(socketPath string) *healthplatform.IssueReport {
	return &healthplatform.IssueReport{
		IssueID: IssueIDSocket,
		Context: map[string]string{
			"type":       "socket",
			"socketPath": socketPath,
			"os":         runtime.GOOS,
		},
		Tags: []string{"integration:docker", "docker:socket"},
	}
}

// NewDockerLogFilePermissionIssue creates a health issue report for Docker log file permission problems
func NewDockerLogFilePermissionIssue(dockerDir string) *healthplatform.IssueReport {
	return &healthplatform.IssueReport{
		IssueID: IssueIDLogFiles,
		Context: map[string]string{
			"type":      "log-files",
			"dockerDir": dockerDir,
			"os":        runtime.GOOS,
		},
		Tags: []string{"docker", "logs", "permissions"},
	}
}
