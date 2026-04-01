// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package apmcgroupv2

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// Issue provides the issue template for APM cgroup v2 container tag enrichment failures
type Issue struct{}

// NewIssue creates a new APM cgroup v2 issue template
func NewIssue() *Issue {
	return &Issue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *Issue) BuildIssue(_ map[string]string) (*healthplatform.Issue, error) {
	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "apm_cgroup_v2_container_tags_missing",
		Title:       "APM Traces Missing Container Tags on Kubernetes with cgroup v2",
		Description: "The agent is running in a Kubernetes environment with cgroup v2, and the APM container ID resolver may not be able to read container IDs from the cgroup filesystem. This prevents container-level tags (container_id, kube_container_name, kube_namespace, etc.) from being added to traces.",
		Category:    "runtime",
		Location:    "apm",
		Severity:    "medium",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "apm",
		Remediation: buildRemediation(),
		Tags:        []string{"apm", "cgroup", "kubernetes", "container-tags", "traces", "linux"},
	}, nil
}

// buildRemediation creates the remediation steps for the APM cgroup v2 issue
func buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Ensure the APM container ID resolver can read cgroup v2 information and that the Agent version supports cgroup v2",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: fmt.Sprintf("Check agent log for: %q", "Failed to identify cgroups version")},
			{Order: 2, Text: "Verify cgroup version: stat /sys/fs/cgroup/cgroup.controllers (file exists = cgroup v2 is active)"},
			{Order: 3, Text: "Ensure Agent version >= 7.35.0 which has proper cgroup v2 support"},
			{Order: 4, Text: "If using Docker Desktop or kind, verify the kubelet uses cgroup v2 driver (--cgroup-driver=systemd)"},
			{Order: 5, Text: "Check that DD_KUBELET_TLS_VERIFY is not causing kubelet auth failures that prevent container metadata resolution"},
		},
	}
}
