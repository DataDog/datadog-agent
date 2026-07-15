// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package admisconfig

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// TemplateIssueName is the human-readable issue name for AD template resolution failure issues.
	TemplateIssueName = "Autodiscovery Template Resolution Error"
	// TemplateIssueType is the snake_case type key for AD template resolution failure
	// issues: TemplateIssueName lowercased with spaces replaced by underscores.
	TemplateIssueType = "autodiscovery_template_resolution_error"
	// TemplateIssueID is the IssueID prefix for AD template resolution failure issues.
	// External reporters append name, service-id, and digest: TemplateIssueID + ":" + name + ":" + serviceID + ":" + digest
	TemplateIssueID = "ad-template"

	templateIssueName = TemplateIssueName
	templateIssueType = TemplateIssueType
)

// ADTemplateIssue provides the issue template for AD template resolution failure issues.
type ADTemplateIssue struct{}

// NewADTemplateIssue creates a new AD template resolution issue template.
func NewADTemplateIssue() *ADTemplateIssue {
	return &ADTemplateIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for template resolution failures.
func (t *ADTemplateIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	entityName := context["entityName"]
	if entityName == "" {
		entityName = unknownVal
	}
	errorMessage := context["errorMessage"]
	if errorMessage == "" {
		errorMessage = failedMsg
	}

	extra, err := structpb.NewStruct(map[string]any{
		"entity_name":   entityName,
		"error_message": errorMessage,
		"impact":        impactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   templateIssueName,
		IssueType:   templateIssueType,
		Title:       templateIssueName + " on '" + entityName + "'",
		Description: "Autodiscovery template resolution error: " + errorMessage,
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify that all template variables in the integration configuration are supported for this service",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check that all template variables (%%var%%) in your integration configuration are supported for this service"},
				{Order: 2, Text: "Run 'datadog-agent configcheck' to see all configuration resolution warnings"},
			},
		},
		Tags: []string{"autodiscovery"},
	}, nil
}
