// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rofspermissions

import (
	"fmt"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"google.golang.org/protobuf/types/known/structpb"
)

// RofsPermissionIssue provides issue template for Read-Only Filesystem errors
type RofsPermissionIssue struct{}

// NewRofsPermissionIssue creates a new ROFS permission issue template
func NewRofsPermissionIssue() *RofsPermissionIssue {
	return &RofsPermissionIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *RofsPermissionIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	directoriesStr := context["directories"]
	if directoriesStr == "" {
		directoriesStr = "unknown"
	}

	directories := strings.Split(directoriesStr, ",")

	extra, err := structpb.NewStruct(map[string]any{
		"directories": directoriesStr,
		"impact":      "The Agent cannot start or function correctly without write access to these directories.",
	})
	if err != nil {
		return nil, fmt.Errorf("error building read-only filesystem permission issue: %w", err)
	}

	var remediation *healthplatform.Remediation
	if env.IsContainerized() {
		remediation = &healthplatform.Remediation{
			Summary: "Mount writable volumes to required directories.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Mount a writable volume (as a bind mount or VOLUME instruction) to the following directories:"},
				{Order: 2, Text: fmt.Sprintf("[%s]", directoriesStr)},
				{Order: 3, Text: "Identify your container configuration (Docker, Kubernetes, ECS) for the Agent."},
				{Order: 2, Text: "Learn More: https://docs.datadoghq.com/containers/guide/readonly-root-filesystem"},
			},
		}
	} else {
		// TODO: Add more specific remediation steps, possible scrip to run.
		remediation = &healthplatform.Remediation{
			Summary: "Grant write permissions to required directories using ACL or adding dd-agent to group",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Grant write permissions to the following directories:"},
				{Order: 2, Text: fmt.Sprintf("[%s]", directoriesStr)},
				{Order: 3, Text: fmt.Sprintf("Example: sudo setfacl -Rm g:dd-agent:rwx '%s'", strings.ReplaceAll(directories[0], `'`, `'\''`))},
			},
		}
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "read_only_filesystem_error",
		Title:       "Agent Cannot Write to Read-Only Filesystem",
		Description: fmt.Sprintf("The Agent may need to write to the following directories but has no write permissions: %s", directories),
		Category:    "permissions",
		Location:    "core",
		Severity:    "high",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "agent",
		Extra:       extra,
		Remediation: remediation,
		Tags:        []string{},
	}, nil
}
