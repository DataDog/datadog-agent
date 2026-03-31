// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dockerlogsrotation

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// Issue provides the issue template for Docker log rotation risk
type Issue struct{}

// NewIssue creates a new Docker logs rotation issue template
func NewIssue() *Issue {
	return &Issue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *Issue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	dockerLogDir := context["dockerLogDir"]
	if dockerLogDir == "" {
		dockerLogDir = "/var/lib/docker/containers"
	}

	reason := context["reason"]

	issueExtra, err := structpb.NewStruct(map[string]any{
		"integration": "docker",
		"docker_dir":  dockerLogDir,
		"reason":      reason,
		"impact":      "Docker log rotation can cause the agent to lose its position in the log stream, resulting in log gaps or complete collection failure until the agent is restarted",
		"fix":         "Enable file-based Docker log tailing with logs_config.docker_container_use_file: true",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "docker_logs_rotation_risk",
		Title:       "Docker Container Logs May Stop After Log Rotation",
		Description: "The agent is collecting Docker container logs via socket-based tailing (docker_container_use_file: false). Socket-based tailing does not handle Docker log rotation gracefully — when Docker rotates log files, the agent may lose the file position and stop receiving new logs until the agent is restarted.",
		Category:    "configuration",
		Location:    "logs-agent",
		Severity:    "medium",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "logs",
		Extra:       issueExtra,
		Remediation: buildRemediation(),
		Tags:        []string{"docker", "logs", "rotation", "file-tailing", "socket-tailing"},
	}, nil
}

// buildRemediation creates the remediation steps for this issue
func buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Switch to file-based Docker log tailing to handle log rotation correctly",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Enable file-based Docker log tailing: add `logs_config.docker_container_use_file: true` to datadog.yaml"},
			{Order: 2, Text: "Restart the agent: systemctl restart datadog-agent"},
			{Order: 3, Text: "Alternatively, configure Docker to disable log rotation: set \"log-opts\": {\"max-size\": \"0\"} in /etc/docker/daemon.json (requires Docker daemon restart)"},
			{Order: 4, Text: "Verify: datadog-agent status | grep -A 10 \"Log Agent\""},
		},
	}
}
