// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gpuenvironment provides Path-B issue templates for GPU environment problems.
package gpuenvironment

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// IssueName is the human-readable issue name for GPU environment issues.
	IssueName = "GPU Environment Issue"
	// IssueType is the snake_case type key for GPU environment issues:
	// IssueName lowercased with spaces replaced by underscores.
	IssueType = "gpu_environment_issue"
	// IssueID is the issue ID prefix for GPU environment issues.
	// External reporters append a reason-specific suffix: IssueID + ":nvml-unavailable".
	IssueID = "gpu-environment-issue"
	// Source is the reporting component identifier used in health-platform issues.
	Source = "gpu"

	// ReasonNvmlUnavailable indicates that NVML has remained unavailable past the reporting threshold.
	ReasonNvmlUnavailable = "nvml-unavailable"

	issueName  = IssueName
	issueType  = IssueType
	category   = "availability"
	location   = "gpu"
	severity   = healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	source     = Source
	impactMsg  = "GPU monitoring metrics cannot be collected while the NVML library is unavailable"
	gpuDocsURL = "https://docs.datadoghq.com/gpu_monitoring/setup/"
)

type issueContent struct {
	title       string
	description string
	summary     string
	steps       []*healthplatform.RemediationStep
}

// GPUEnvironmentIssue provides the issue template for GPU environment issues.
type GPUEnvironmentIssue struct{}

// NewGPUEnvironmentIssue creates a new GPU environment issue template.
func NewGPUEnvironmentIssue() *GPUEnvironmentIssue {
	return &GPUEnvironmentIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for GPU environment issues.
func (t *GPUEnvironmentIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	reason := context["reason"]
	content, err := buildContent(reason)
	if err != nil {
		return nil, fmt.Errorf("failed to build content: %w", err)
	}

	extra, err := structpb.NewStruct(map[string]any{
		"reason": reason,
		"impact": impactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   issueName,
		IssueType:   issueType,
		Title:       content.title,
		Description: content.description,
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: content.summary,
			Steps:   content.steps,
		},
		Tags: []string{"gpu", "nvml"},
	}, nil
}

func buildContent(reason string) (issueContent, error) {
	switch reason {
	case ReasonNvmlUnavailable:
		return issueContent{
			title:       "GPU monitoring cannot initialize NVML",
			description: "GPU monitoring cannot initialize the NVIDIA Management Library (NVML). Without this library, the Datadog Agent cannot collect GPU metrics.",
			summary:     "Verify that the NVIDIA driver and NVML library are installed and visible to the Agent.",
			steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Run 'nvidia-smi' on the host to verify that the NVIDIA driver and NVML are working"},
				{Order: 2, Text: "Verify that libnvidia-ml.so is installed and visible in the Agent runtime environment"},
				{Order: 3, Text: "If the Agent runs in a container, verify that GPU devices and NVIDIA driver libraries are mounted into the container"},
				{Order: 4, Text: "Review GPU monitoring setup: " + gpuDocsURL},
			},
		}, nil
	default:
		return issueContent{}, fmt.Errorf("unknown reason: %s", reason)
	}
}
