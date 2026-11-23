// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package missingclcrunners

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName = IssueName
	issueType = IssueType
	category  = "availability"
	location  = "cluster-checks"
	severity  = healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	source    = "cluster-agent"
)

// MissingCLCRunnersIssue builds a complete issue for the case where the Cluster
// Agent is dispatching cluster checks but no Cluster Check Runners are reporting.
type MissingCLCRunnersIssue struct{}

// BuildIssue creates a complete issue with metadata and remediation.
// Expected context keys:
//   - "pending_checks": the number of cluster-check configs that cannot be dispatched
func (t *MissingCLCRunnersIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	description := "The Cluster Agent has cluster checks to dispatch but no Cluster Check Runners are reporting. " +
		"Cluster checks are not being dispatched and their metrics may be lost."

	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: "Verify the Cluster Check Runner pods are running and ready: kubectl get pods -l app=datadog-clusterchecks"},
		{Order: 2, Text: "Inspect the Cluster Check Runner pod logs and events for crashes, OOMKills, or scheduling failures"},
		{Order: 3, Text: "Ensure the Cluster Check Runner deployment has at least one ready replica and is not scaled to zero"},
		{Order: 4, Text: "Confirm the runners can reach the Cluster Agent (DD_CLUSTER_AGENT_URL / service connectivity and shared auth token)"},
		{Order: 5, Text: "See docs: https://docs.datadoghq.com/containers/cluster_agent/clusterchecksrunner/"},
	}

	extraFields := map[string]any{
		"impact": "Cluster checks are not dispatched while no runners are available; the metrics those checks would emit may be lost.",
	}
	if pending := context["pending_checks"]; pending != "" {
		extraFields["pending_checks"] = pending
	}

	extra, err := structpb.NewStruct(extraFields)
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   issueName,
		IssueType:   issueType,
		Title:       "Missing Cluster Check Runners",
		Description: description,
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Restore at least one healthy Cluster Check Runner so the Cluster Agent can dispatch cluster checks",
			Steps:   steps,
		},
		Tags: []string{"cluster-checks", "cluster-check-runner", "cluster-agent"},
	}, nil
}
