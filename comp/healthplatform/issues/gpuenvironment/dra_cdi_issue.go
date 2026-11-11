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
	// DRACDIIssueID is the issue ID prefix for this issue. Reporters append a
	// suffix identifying the Agent deployment, as the backend deduplicates issues
	// by ID.
	DRACDIIssueID = "gpu-dra-cdi-specs-unavailable"

	// DRACDIContextKeySearchedPaths is the context key holding the comma-separated CDI spec directories that were checked.
	DRACDIContextKeySearchedPaths = "searched_paths"
	// DRACDIContextKeyError is the context key holding why no CDI spec could be found in the searched directories.
	DRACDIContextKeyError = "error"

	draCDIIssueName = DRACDIIssueName
	draCDIIssueType = DRACDIIssueType
	draCDIImpactMsg = "GPU metrics of containers using NVIDIA DRA claims that the Agent has not already mapped to their devices may be missing pod and container tags, or be attributed by device index instead of the allocated device"
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
	extra, err := structpb.NewStruct(map[string]any{
		DRACDIContextKeySearchedPaths: searchedPaths,
		DRACDIContextKeyError:         errMsg,
		"impact":                      draCDIImpactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   draCDIIssueName,
		IssueType:   draCDIIssueType,
		Title:       fmt.Sprintf("GPU monitoring cannot find the CDI specs of NVIDIA DRA claims (searched %s)", searchedPaths),
		Description: "Containers use NVIDIA DRA GPU claims, but the Agent cannot find or read the CDI specs written by the NVIDIA DRA driver to map them to GPU devices. Error: " + errMsg,
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Mount the host directory where the NVIDIA DRA driver writes CDI specs (/var/run/cdi by default) into the Agent container.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Helm chart: setting datadog.criSocketPath or datadog.dockerSocketPath replaces the default /var/run mount with the socket directory only; add a hostPath volume for /var/run/cdi in agents.volumes and mount it at /host/var/run/cdi (read-only) with agents.volumeMounts"},
				{Order: 2, Text: "Datadog Operator: setting spec.global.criSocketPath or spec.global.dockerSocketPath replaces the default /var/run mount with the socket only; add a hostPath volume for /var/run/cdi in spec.override.nodeAgent.volumes and mount it at /host/var/run/cdi (read-only) in spec.override.nodeAgent.containers.agent.volumeMounts"},
				{Order: 3, Text: "Run 'ls /var/run/cdi' on the host to verify that the NVIDIA DRA driver writes CDI specs (k8s.gpu.nvidia.com-claim_*.yaml) there"},
				{Order: 4, Text: "If it does not, check the CDI root of the NVIDIA DRA driver kubelet plugin (CDI_ROOT environment variable or --cdi-root flag): its Helm chart uses /var/run/cdi, but the binary defaults to /etc/cdi. Mount that host directory at /host/var/run/cdi (read-only) instead of /var/run/cdi"},
				{Order: 5, Text: "For other deployments, mount the host CDI root directory at /host/var/run/cdi (read-only) in the Agent container"},
				{Order: 6, Text: "Verify that the Agent process has read permissions on the CDI spec files"},
				{Order: 7, Text: "Review GPU monitoring setup: " + gpuDocsURL},
			},
		},
		Tags: []string{"gpu", "dra", "cdi"},
	}, nil
}
