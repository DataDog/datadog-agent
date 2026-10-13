// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package highmemoryk8s reports when the node Agent uses most of its Kubernetes container memory limit.
package highmemoryk8s

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// IssueName is the issue type key of the high memory issue.
	IssueName = "High Memory K8s"
	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "high_memory_k8s"
	// IssueIDPrefix is the base of the issue id. The full id is suffixed with the reporting Agent and the source of
	// the issue, see issueID.
	IssueIDPrefix = "high-memory-k8s"

	issueName = IssueName
	issueType = IssueType

	contextKeyUsagePercent     = "usage_percent"
	contextKeyThresholdPercent = "threshold_percent"
	contextKeyWindow           = "window"

	unknownValue = "unknown"
)

// HighMemoryK8sIssue builds an Agent Health issue for a node Agent close to its container memory limit.
type HighMemoryK8sIssue struct{}

// BuildIssue creates a complete issue with remediation for a node Agent close to its container memory limit.
func (HighMemoryK8sIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	usage := valueOrUnknown(context, contextKeyUsagePercent)
	threshold := valueOrUnknown(context, contextKeyThresholdPercent)
	window := valueOrUnknown(context, contextKeyWindow)

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyUsagePercent:     usage,
		contextKeyThresholdPercent: threshold,
		contextKeyWindow:           window,
		"impact":                   "The Agent may be OOM killed by the kubelet, interrupting data collection",
	})
	if err != nil {
		return nil, fmt.Errorf("create issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   issueName,
		IssueType:   issueType,
		Title:       fmt.Sprintf("Datadog Agent memory usage is at %s%% of its container limit", usage),
		Description: fmt.Sprintf("The node Agent used %s%% of its container memory limit on average over the last %s, above the %s%% threshold. The kubelet OOM kills the Agent if it reaches the limit.", usage, window, threshold),
		Category:    "resources",
		Location:    "agent",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      "agent",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Raise the Agent container memory limit or reduce the Agent's memory usage.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Run `agent status` in the Agent pod and check which checks and integrations are running."},
				{Order: 2, Text: "Check the Agent container for recent OOM kills with `kubectl describe pod <agent-pod>` (Last State: OOMKilled)."},
				{Order: 3, Text: "Raise the Agent container memory limit: `agents.containers.agent.resources.limits.memory` in the Helm chart, or `spec.override.nodeAgent.containers.agent.resources` in the DatadogAgent resource."},
				{Order: 4, Text: "Reduce memory usage by disabling unused checks or lowering high cardinality settings, then confirm with `agent status`."},
			},
		},
		Tags: []string{"memory", "kubernetes", "resources"},
	}, nil
}

func valueOrUnknown(context map[string]string, key string) string {
	if v := context[key]; v != "" {
		return v
	}
	return unknownValue
}
