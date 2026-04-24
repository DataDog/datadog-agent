// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package jmxmetriccap

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueTitle    = "JMX Metric Collection Limit Reached"
	issueCategory = "configuration"
	issueLocation = "jmxfetch"
	issueSeverity = "warning"
	unknownVal    = "unknown"
)

// JMXMetricCapIssue provides complete issue template for JMX metric cap exceeded
type JMXMetricCapIssue struct{}

// NewJMXMetricCapIssue creates a new JMX metric cap issue template
func NewJMXMetricCapIssue() *JMXMetricCapIssue {
	return &JMXMetricCapIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for JMX metric cap exceeded
func (t *JMXMetricCapIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	checkName := context["checkName"]
	if checkName == "" {
		checkName = unknownVal
	}

	limit := context["limit"]
	if limit == "" {
		limit = unknownVal
	}

	collected := context["collected"]
	if collected == "" {
		collected = unknownVal
	}

	description := "The JMX check " + checkName + " has reached the maximum metric collection limit of " + limit + ". Only " + collected + " metrics were collected and some may have been silently dropped. Increase the limit or reduce the bean scope."

	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: "Increase the limit in conf.yaml: max_returned_metrics: 1000"},
		{Order: 2, Text: "Narrow the bean scope using include_match / exclude_match filters"},
		{Order: 3, Text: "Review which beans are contributing the most metrics"},
	}

	extra, err := structpb.NewStruct(map[string]any{
		"check_name": checkName,
		"limit":      limit,
		"collected":  collected,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		Title:       issueTitle,
		Description: description,
		Category:    issueCategory,
		Location:    issueLocation,
		Severity:    issueSeverity,
		DetectedAt:  "",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Increase metric limit or reduce bean scope",
			Steps:   steps,
		},
		Tags: []string{"jmx", "metrics", "limit"},
	}, nil
}
