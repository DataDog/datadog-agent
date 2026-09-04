// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dockerpermissions

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

//go:embed fix-docker-socket-permissions.sh
var linuxScript string

//go:embed Fix-DockerSocketPermissions.ps1
var windowsScript string

// DockerPermissionIssue provides complete issue template (metadata + OS-specific remediation)
type DockerPermissionIssue struct{}

// NewDockerPermissionIssue creates a new Docker permission issue template
func NewDockerPermissionIssue() *DockerPermissionIssue {
	return &DockerPermissionIssue{}
}

// BuildIssue creates a complete issue with metadata and OS-specific remediation
func (t *DockerPermissionIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	osName := context["os"]
	if osName == "" {
		osName = "linux" // fallback
	}

	socketPaths := context["socketPaths"]
	if socketPaths == "" {
		if osName == "windows" {
			socketPaths = "//./pipe/docker_engine" // fallback
		} else {
			socketPaths = "/var/run/docker.sock" // fallback
		}
	}

	issueExtra, err := structpb.NewStruct(map[string]any{
		"integration":  "docker",
		"socket_paths": socketPaths,
		"os":           osName,
		"impact":       "The agent cannot query the Docker daemon, so container metadata, logs, and checks that rely on the Docker API will be missing or incomplete.",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       fmt.Sprintf("Docker socket permission denied at '%s'", socketPaths),
		Description: fmt.Sprintf("The dd-agent user does not have permission to connect to the Docker socket at %s. The socket exists but the agent gets a permission-denied error when connecting, so it cannot query the Docker daemon for container metadata, logs, or checks.", socketPaths),
		Category:    "permissions",
		Location:    "agent",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		DetectedAt:  "", // Will be filled by health platform
		Source:      "docker",
		Extra:       issueExtra,
		Remediation: t.buildRemediation(socketPaths, osName),
		Tags:        []string{"docker", osName},
	}, nil
}

// buildRemediation creates OS-specific remediation
func (t *DockerPermissionIssue) buildRemediation(socketPaths, osName string) *healthplatform.Remediation {
	if osName == "windows" {
		return t.buildWindows(socketPaths)
	}
	return t.buildLinux(socketPaths) // linux, darwin
}

// buildLinux creates Linux-specific remediation steps
func (t *DockerPermissionIssue) buildLinux(socketPaths string) *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Add the dd-agent user to the docker group so it can connect to the Docker socket.",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Affected socket(s): " + socketPaths},
			{Order: 2, Text: "Add dd-agent to the docker group: sudo usermod -aG docker dd-agent"},
			{Order: 3, Text: "Restart the datadog-agent service: sudo systemctl restart datadog-agent"},
			{Order: 4, Text: "Verify the issue is resolved by checking agent status: datadog-agent status"},
		},
		Script: &healthplatform.Script{
			Language:        "bash",
			LanguageVersion: "4.0+",
			Filename:        "fix-docker-socket-permissions.sh",
			RequiresRoot:    true,
			Content:         linuxScript,
		},
	}
}

// buildWindows creates Windows-specific remediation steps
func (t *DockerPermissionIssue) buildWindows(socketPaths string) *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Add the ddagentuser account to the docker-users group so it can connect to the Docker named pipe.",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Affected named pipe(s): " + socketPaths},
			{Order: 2, Text: "Open PowerShell as Administrator"},
			{Order: 3, Text: `Add ddagentuser to the docker-users group: Add-LocalGroupMember -Group "docker-users" -Member "ddagentuser"`},
			{Order: 4, Text: "Restart the Datadog Agent service: Restart-Service -Name datadogagent"},
			{Order: 5, Text: "Verify the issue is resolved by checking agent status"},
		},
		Script: &healthplatform.Script{
			Language:        "powershell",
			LanguageVersion: "5.1+",
			Filename:        "Fix-DockerSocketPermissions.ps1",
			RequiresRoot:    true,
			Content:         windowsScript,
		},
	}
}
