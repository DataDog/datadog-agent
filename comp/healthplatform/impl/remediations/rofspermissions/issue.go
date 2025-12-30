// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package rofspermissions provides remediation for Read-Only Filesystem permission issues.
package rofspermissions

import (
	"fmt"
	"strings"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// RofsPermissionIssue provides issue template for Read-Only Filesystem errors
type RofsPermissionIssue struct{}

// NewRofsPermissionIssue creates a new ROFS permission issue template
func NewRofsPermissionIssue() *RofsPermissionIssue {
	return &RofsPermissionIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *RofsPermissionIssue) BuildIssue(context map[string]string) *healthplatform.Issue {
	directory := context["directory"]
	if directory == "" {
		directory = "unknown"
	}

	volName := strings.TrimPrefix(strings.ReplaceAll(directory, "/", "-"), "-")
	if volName == "" {
		volName = "data"
	}

	return &healthplatform.Issue{
		ID:          "read-only-filesystem-error",
		IssueName:   "read_only_filesystem_error",
		Title:       "Agent Cannot Write to Read-Only Filesystem",
		Description: fmt.Sprintf("The Agent attempted to write to %s but failed because the filesystem is read-only. This is common in containerized environments with readOnlyRootFilesystem: true.", directory),
		Category:    "permissions",
		Location:    "agent-startup",
		Severity:    "critical",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "agent",
		Extra: map[any]any{
			"directory": directory,
			"impact":    "The Agent cannot start or function correctly without write access to this directory.",
		},
		Remediation: &healthplatform.Remediation{
			Summary: fmt.Sprintf("Mount a writable volume to %s", directory),
			Steps: []healthplatform.RemediationStep{
				{Order: 1, Text: "Identify the container configuration (Docker, Kubernetes, ECS) for the Agent."},
				{Order: 2, Text: fmt.Sprintf("Add a writable volume mount for the path: %s", directory)},
				{Order: 3, Text: "For Kubernetes, add an emptyDir volume:"},
				{Order: 4, Text: fmt.Sprintf("  - name: writable-%s", volName)},
				{Order: 5, Text: "    emptyDir: {}"},
				{Order: 6, Text: fmt.Sprintf("  volumeMounts:\n    - mountPath: %s\n      name: writable-%s", directory, volName)},
				{Order: 7, Text: "Restart the Agent container."},
			},
		},
		Tags: []string{"rofs", "permissions", "startup", "configuration"},
	}
}
