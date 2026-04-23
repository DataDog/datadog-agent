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

// containerLabelSource matches the string value of types.ContainerLabelSource
// to avoid a cross-package import. The value is passed as a string in the issue context.
const containerLabelSource = "container_label"

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
