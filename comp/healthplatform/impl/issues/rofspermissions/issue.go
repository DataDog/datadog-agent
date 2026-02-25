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
			Summary: "Mount writable volumes to each directory listed.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Mount writable volumes to the following directories: " + directoriesStr},
				{Order: 2, Text: "Use a bind mount or emptyDir volume depending on your container platform (Docker, Kubernetes, ECS)."},
				{Order: 3, Text: "For detailed instructions, see: https://docs.datadoghq.com/containers/guide/readonly-root-filesystem"},
			},
		}
	} else {
		remediation = &healthplatform.Remediation{
			Summary: "Grant write permissions to the dd-agent user for the required directories.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Grant write permissions to the following directories: " + directoriesStr},
				{Order: 2, Text: fmt.Sprintf("Run: sudo setfacl -Rm g:dd-agent:rwx '%s'", strings.ReplaceAll(directories[0], `'`, `'\''`))},
				{Order: 3, Text: "Alternatively, add the dd-agent user to a group with write access to these directories."},
			},
		}
	}

	descriptionDirectory := "directory"
	if len(directories) > 1 {
		descriptionDirectory = "directories"
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "read_only_filesystem_error",
		Title:       "Agent Cannot Write to Read-Only Filesystem",
		Description: fmt.Sprintf("Agent is missing write access to %v %v. Without write access, the Agent may experience issues starting or operating correctly.", len(directories), descriptionDirectory),
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
