// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpuenvironment

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// DRACDIIssueName is the human-readable issue name for CDI specs of NVIDIA DRA
	// claims not being found or readable by the Agent.
	DRACDIIssueName = "GPU DRA CDI Specs Unavailable"
	// DRACDIIssueType is the snake_case type key: DRACDIIssueName lowercased with
	// spaces replaced by underscores.
	DRACDIIssueType = "gpu_dra_cdi_specs_unavailable"
	// DRACDIIssueID is the issue ID for this issue. There is at most one instance per agent.
	DRACDIIssueID = "gpu-dra-cdi-specs-unavailable"

	// DRACDIContextKeySearchedPaths is the context key holding the comma-separated CDI spec directories that were checked.
	DRACDIContextKeySearchedPaths = "searched_paths"
	// DRACDIContextKeyError is the context key holding why no CDI spec could be found in the searched directories.
	DRACDIContextKeyError = "error"
	// DRACDIContextKeyContainerCount is the context key holding the number of containers with unresolved DRA GPU claims.
	DRACDIContextKeyContainerCount = "container_count"

	draCDIIssueName = DRACDIIssueName
	draCDIIssueType = DRACDIIssueType
	draCDIImpactMsg = "GPU metrics of containers using NVIDIA DRA claims may be missing pod and container tags, or be attributed by device index instead of the allocated device"
)

// DRACDIIssue provides the issue template for missing or unreadable DRA CDI specs.
type DRACDIIssue struct{}

// NewDRACDIIssue creates a new DRA CDI issue template.
func NewDRACDIIssue() *DRACDIIssue {
	return &DRACDIIssue{}
}

// BuildIssue creates the issue from the given context.
func (t *DRACDIIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	searchedPaths := context[DRACDIContextKeySearchedPaths]
	if searchedPaths == "" {
		searchedPaths = "unknown"
	}
	errMsg := context[DRACDIContextKeyError]
	if errMsg == "" {
		errMsg = "unknown error"
	}
	containerCount := context[DRACDIContextKeyContainerCount]
	if containerCount == "" {
		containerCount = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		DRACDIContextKeySearchedPaths:  searchedPaths,
		DRACDIContextKeyError:          errMsg,
		DRACDIContextKeyContainerCount: containerCount,
		"impact":                       draCDIImpactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   draCDIIssueName,
		IssueType:   draCDIIssueType,
		Title:       fmt.Sprintf("GPU monitoring cannot find the CDI specs of NVIDIA DRA claims (searched %s)", searchedPaths),
		Description: fmt.Sprintf("%s container(s) use NVIDIA DRA GPU claims, but the Agent cannot find or read the CDI specs written by the NVIDIA DRA driver to map them to GPU devices. Error: %s", containerCount, errMsg),
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Mount the host CDI spec directory (/var/run/cdi) into the Agent container.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Run 'ls /var/run/cdi' on the host to verify that the NVIDIA DRA driver writes CDI specs there"},
				{Order: 2, Text: "If the Agent runs in a container, mount the host /var/run/cdi directory at /host/var/run/cdi (read-only) in the Agent container"},
				{Order: 3, Text: "Verify that the Agent process has read permissions on the CDI spec files"},
				{Order: 4, Text: "Review GPU monitoring setup: " + gpuDocsURL},
			},
		},
		Tags: []string{"gpu", "dra", "cdi"},
	}, nil
}
