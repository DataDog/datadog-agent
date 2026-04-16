// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package admisconfig

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName  = "ad_misconfiguration"
	category   = "autodiscovery"
	location   = "autodiscovery"
	severity   = "medium"
	source     = "autodiscovery"
	unknownVal = "unknown"
	failedMsg  = "Autodiscovery misconfiguration error detected"
	impactMsg  = "Metrics, and logs may not be collected due to misconfigured autodiscovery settings"
)

// These constants match the string values of types.ErrorSource to avoid a
// cross-package import. The values are passed as strings in the issue context.
const (
	containerLabelSource     = "container_label"
	templateResolutionSource = "template_resolution"
)

type issueContent struct {
	title       string
	description string
	summary     string
	steps       []*healthplatform.RemediationStep
}

// ADMisconfigurationIssue provides complete issue template for AD annotation misconfigurations
type ADMisconfigurationIssue struct{}

// NewADMisconfigurationIssue creates a new AD annotation issue template
func NewADMisconfigurationIssue() *ADMisconfigurationIssue {
	return &ADMisconfigurationIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for AD annotation errors
func (t *ADMisconfigurationIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	entityName := context["entityName"]
	if entityName == "" {
		entityName = unknownVal
	}

	errorMessage := context["errorMessage"]
	if errorMessage == "" {
		errorMessage = failedMsg
	}

	errorSource := context["errorSource"]

	content := buildSourceSpecificContent(entityName, errorMessage, errorSource)

	extra, err := structpb.NewStruct(map[string]any{
		"entity_name":   entityName,
		"error_message": errorMessage,
		"error_source":  errorSource,
		"impact":        impactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          healthplatformdef.ADMisconfigurationIssueID,
		IssueName:   issueName,
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
		Tags: []string{"autodiscovery"},
	}, nil
}

// buildSourceSpecificContent returns title, description, remediation summary, and steps
// tailored to the error source (container labels vs pod annotations).
func buildSourceSpecificContent(entityName, errorMessage, errorSource string) issueContent {
	title := fmt.Sprintf("AD Misconfiguration on '%s'", entityName)
	switch errorSource {
	case templateResolutionSource:
		return issueContent{
			title:       title,
			description: "Autodiscovery template resolution error: " + errorMessage,
			summary:     "Verify that all template variables are supported by the autodiscovery listener for this service",
			steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check that all template variables (%%var%%) are supported by the listener type for this service"},
				{Order: 2, Text: "Review the AD identifiers and ensure they match the correct listener (e.g., RDS vs Aurora have different supported variables)"},
				{Order: 3, Text: "Run 'datadog-agent configcheck' to see all configuration resolution warnings"},
				{Order: 4, Text: "See docs: https://docs.datadoghq.com/containers/guide/template_variables/"},
			},
		}
	case containerLabelSource:
		return issueContent{
			title:       title,
			description: "Autodiscovery container label error: " + errorMessage,
			summary:     "Review and fix autodiscovery container labels on the affected container",
			steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Validate JSON syntax in all com.datadoghq.ad.* container labels"},
				{Order: 2, Text: "Ensure check_names, init_configs, and instances arrays have matching lengths"},
				{Order: 3, Text: "Verify the container label values match the expected Autodiscovery format"},
			},
		}
	default:
		return issueContent{
			title:       title,
			description: "Autodiscovery pod annotation error: " + errorMessage,
			summary:     "Review and fix autodiscovery annotations on the affected pod",
			steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Validate JSON syntax in all ad.datadoghq.com/<container>.* annotations"},
				{Order: 2, Text: "Verify the container name in the annotation matches an actual container in the pod spec"},
				{Order: 3, Text: "Ensure check_names, init_configs, and instances arrays have matching lengths"},
				{Order: 4, Text: "Run 'agent validate-pod-annotation' to check annotation syntax"},
			},
		}
	}
}
