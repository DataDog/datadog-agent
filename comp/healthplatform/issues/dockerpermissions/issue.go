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
		Location:    "logs-agent",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		DetectedAt:  "", // Will be filled by health platform
		Source:      "agent",
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
		Summary: "Add the ddagentuser account to the local group Docker's daemon trusts for named-pipe access (docker-users by default on Docker Desktop).",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Affected named pipe(s): " + socketPaths},
			{Order: 2, Text: "Open PowerShell as Administrator"},
			{Order: 3, Text: `Check the daemon's configured group in C:\ProgramData\docker\config\daemon.json (the "group" key); Docker Desktop sets this to docker-users, but a standalone Docker Engine install on Windows Server may not set it at all`},
			{Order: 4, Text: `Add ddagentuser to that group (docker-users by default): Add-LocalGroupMember -Group "docker-users" -Member "ddagentuser"`},
			{Order: 5, Text: `If daemon.json has no "group" configured, membership alone will not grant access: add "group": "docker-users" to it and restart the Docker service (Restart-Service docker)`},
			{Order: 6, Text: "Restart the Datadog Agent service: Restart-Service -Name datadogagent"},
			{Order: 7, Text: "Verify the issue is resolved by checking agent status"},
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

// DockerSocketUnavailableIssue provides the issue template (metadata +
// OS-specific remediation steps) for a Docker socket that exists but cannot
// be reached for a reason other than a permission error (connection refused,
// stale socket, daemon not running).
type DockerSocketUnavailableIssue struct{}

// NewDockerSocketUnavailableIssue creates a new Docker socket unavailable issue template
func NewDockerSocketUnavailableIssue() *DockerSocketUnavailableIssue {
	return &DockerSocketUnavailableIssue{}
}

// BuildIssue creates a complete issue with metadata and OS-specific remediation
func (t *DockerSocketUnavailableIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
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
		IssueName:   SocketUnavailableIssueName,
		IssueType:   SocketUnavailableIssueType,
		Title:       fmt.Sprintf("Docker socket unavailable at '%s'", socketPaths),
		Description: fmt.Sprintf("The Docker socket at %s exists but the agent could not connect to it. This is not a permission error — likely causes are the Docker daemon not running, a stale socket file left behind by a stopped daemon, or the daemon refusing connections.", socketPaths),
		Category:    "availability",
		Location:    "logs-agent",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		DetectedAt:  "", // Will be filled by health platform
		Source:      "agent",
		Extra:       issueExtra,
		Remediation: t.buildRemediation(socketPaths, osName),
		Tags:        []string{"docker", osName},
	}, nil
}

// buildRemediation creates OS-specific remediation
func (t *DockerSocketUnavailableIssue) buildRemediation(socketPaths, osName string) *healthplatform.Remediation {
	if osName == "windows" {
		return t.buildWindows(socketPaths)
	}
	return t.buildLinux(socketPaths) // linux, darwin
}

// buildLinux creates Linux-specific remediation steps. No auto-fix script is
// provided since the root cause (daemon down, stale socket, ...) is unknown.
func (t *DockerSocketUnavailableIssue) buildLinux(socketPaths string) *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Verify the Docker daemon is running and reachable at the affected socket(s).",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Affected socket(s): " + socketPaths},
			{Order: 2, Text: "Check whether the Docker daemon is running: sudo systemctl status docker"},
			{Order: 3, Text: "If it is not running, start it: sudo systemctl start docker"},
			{Order: 4, Text: "If it is running, confirm the socket path matches the daemon's configured listen address and that DOCKER_HOST (if set) points at it"},
			{Order: 5, Text: "Restart the datadog-agent service: sudo systemctl restart datadog-agent"},
			{Order: 6, Text: "Verify the issue is resolved by checking agent status: datadog-agent status"},
		},
	}
}

// buildWindows creates Windows-specific remediation steps. No auto-fix script
// is provided since the root cause (daemon down, stale pipe, ...) is unknown.
func (t *DockerSocketUnavailableIssue) buildWindows(socketPaths string) *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Verify the Docker daemon is running and reachable at the affected named pipe(s).",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Affected named pipe(s): " + socketPaths},
			{Order: 2, Text: "Open PowerShell as Administrator"},
			{Order: 3, Text: "Check whether the Docker service is running: Get-Service docker"},
			{Order: 4, Text: "If it is not running, start it: Start-Service docker"},
			{Order: 5, Text: "If it is running, confirm the named pipe path matches the daemon's configuration"},
			{Order: 6, Text: "Restart the Datadog Agent service: Restart-Service -Name datadogagent"},
			{Order: 7, Text: "Verify the issue is resolved by checking agent status"},
		},
	}
}
