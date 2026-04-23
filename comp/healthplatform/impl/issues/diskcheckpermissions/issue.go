// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package diskcheckpermissions

import (
	"fmt"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName  = "disk_check_permission_denied"
	category   = "permissions"
	location   = "collector"
	severity   = "warning"
	source     = "disk"
	unknownVal = "unknown"
)

// DiskCheckPermissionIssue provides complete issue template for disk check permission errors
type DiskCheckPermissionIssue struct{}

// NewDiskCheckPermissionIssue creates a new disk check permission issue template
func NewDiskCheckPermissionIssue() *DiskCheckPermissionIssue {
	return &DiskCheckPermissionIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for disk check permission errors
func (t *DiskCheckPermissionIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	path := context["path"]
	if path == "" {
		path = unknownVal
	}

	errorMsg := context["error"]
	if errorMsg == "" {
		errorMsg = unknownVal
	}

	description := strings.NewReplacer(
		"{path}", path,
		"{error}", errorMsg,
	).Replace("The disk check cannot read disk statistics for path '{path}': {error}. Disk metrics for this mount point will not be collected.")

	extra, err := structpb.NewStruct(map[string]any{
		"path":  path,
		"error": errorMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   issueName,
		Title:       "Disk Check Lacks Read Permission",
		Description: description,
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Grant read access to the dd-agent user or exclude the problematic path.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Grant read access to the dd-agent user: `sudo chmod o+r " + path + "` or `sudo setfacl -m u:dd-agent:r " + path + "`"},
				{Order: 2, Text: "Check if the disk is mounted with `noexec` or restrictive options: `mount | grep " + path + "`"},
				{Order: 3, Text: "In containerized environments, ensure the disk path is mounted into the container"},
				{Order: 4, Text: "Exclude the problematic path with `excluded_filesystems` in disk check config if not needed"},
			},
		},
		Tags: []string{"disk", "permissions", "metrics"},
	}, nil
}
