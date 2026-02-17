// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rofspermissions

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
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
	directory := context["directory"]
	if directory == "" {
		directory = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"directory": directory,
		"impact":    "The Agent cannot start or function correctly without write access to this directory.",
	})
	if err != nil {
		return nil, fmt.Errorf("error building read-only filesystem permission issue: %w", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "read_only_filesystem_error",
		Title:       "Agent Cannot Write to Read-Only Filesystem",
		Description: fmt.Sprintf("The Agent attempted to write to %s but failed because the filesystem is read-only. This is common in containerized environments with readOnlyRootFilesystem: true.", directory),
		Category:    "permissions",
		Location:    "core",
		Severity:    "high",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "agent",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: fmt.Sprintf("Mount a writable volume to %s", directory),
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "RECOMMENDED: Grant write permissions to the directory"},
				{Order: 1, Text: "Identify your container configuration (Docker, Kubernetes, ECS) for the Agent."},
				{Order: 2, Text: fmt.Sprintf("Add a writable volume mount for the path: %s", directory)},
				//{Order: 3, Text: "For Kubernetes, add an emptyDir volume:"},
				//{Order: 4, Text: fmt.Sprintf("  - name: writable-%s", volName)},
				//{Order: 5, Text: "    emptyDir: {}"},
				//{Order: 6, Text: fmt.Sprintf("  volumeMounts:\n    - mountPath: %s\n      name: writable-%s", directory, volName)},
				//{Order: 7, Text: "Restart the Agent container."},
			},
		},
		Tags: []string{"rofs", "permissions", "startup", "configuration"},
	}, nil
}
