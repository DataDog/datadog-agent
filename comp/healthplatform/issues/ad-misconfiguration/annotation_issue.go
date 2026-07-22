// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package admisconfig provides issue types for autodiscovery misconfigurations.
// It contains two Path-B issue types whose errors are detected and reported directly
// by the container config provider (ad-annotation) and config manager (ad-template).
package admisconfig

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// AnnotationIssueName is the human-readable issue name for AD annotation misconfiguration issues.
	AnnotationIssueName = "Autodiscovery Annotation Misconfiguration"
	// AnnotationIssueType is the snake_case type key for AD annotation misconfiguration
	// issues: AnnotationIssueName lowercased with spaces replaced by underscores.
	AnnotationIssueType = "autodiscovery_annotation_misconfiguration"
	// AnnotationIssueID is the IssueID prefix for AD annotation misconfiguration issues.
	// External reporters append a per-entity suffix: AnnotationIssueID + ":" + entityName
	AnnotationIssueID = "ad-annotation"
	// Source is the reporting component identifier used in health-platform issues.
	Source = "autodiscovery"

	annotationIssueName = AnnotationIssueName
	annotationIssueType = AnnotationIssueType

	category   = "autodiscovery"
	location   = "autodiscovery"
	severity   = healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	source     = Source
	unknownVal = "unknown"
	failedMsg  = "Autodiscovery misconfiguration error detected"
	impactMsg  = "Metrics, and logs may not be collected due to misconfigured autodiscovery settings"

	containerLabelSource         = "container_label"
	kubeServiceAnnotationSource  = "kube_service_annotation"
	kubeEndpointAnnotationSource = "kube_endpoint_annotation"
)

type issueContent struct {
	title       string
	description string
	summary     string
	steps       []*healthplatform.RemediationStep
}

// ADAnnotationIssue provides the issue template for AD annotation misconfiguration issues.
type ADAnnotationIssue struct{}

// NewADAnnotationIssue creates a new AD annotation issue template.
func NewADAnnotationIssue() *ADAnnotationIssue {
	return &ADAnnotationIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for AD annotation errors.
func (t *ADAnnotationIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	entityName := context["entityName"]
	if entityName == "" {
		entityName = unknownVal
	}
	errorMessage := context["errorMessage"]
	if errorMessage == "" {
		errorMessage = failedMsg
	}
	errorSource := context["errorSource"]

	content := buildAnnotationContent(errorMessage, errorSource)

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
		IssueName:   annotationIssueName,
		IssueType:   annotationIssueType,
		Title:       content.title + " on '" + entityName + "'",
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

func buildAnnotationContent(errorMessage, errorSource string) issueContent {
	switch errorSource {
	case containerLabelSource:
		return issueContent{
			title:       "Autodiscovery Container Label Misconfiguration",
			description: "Autodiscovery container label error: " + errorMessage,
			summary:     "Review and fix autodiscovery container labels on the affected container",
			steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Validate JSON syntax in all com.datadoghq.ad.* container labels"},
				{Order: 2, Text: "Ensure check_names, init_configs, and instances arrays have matching lengths"},
				{Order: 3, Text: "Verify the container label values match the expected Autodiscovery format"},
			},
		}
	case kubeServiceAnnotationSource:
		return issueContent{
			title:       "Autodiscovery Service Annotation Misconfiguration",
			description: "Autodiscovery service annotation error: " + errorMessage,
			summary:     "Review and fix autodiscovery annotations on the affected Kubernetes service",
			steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Validate JSON syntax in all ad.datadoghq.com/service.* annotations on the service"},
				{Order: 2, Text: "Ensure check_names, init_configs, and instances arrays have matching lengths"},
				{Order: 3, Text: "Verify the annotation values match the expected Autodiscovery format for cluster checks"},
			},
		}
	case kubeEndpointAnnotationSource:
		return issueContent{
			title:       "Autodiscovery Endpoint Annotation Misconfiguration",
			description: "Autodiscovery endpoint annotation error: " + errorMessage,
			summary:     "Review and fix autodiscovery annotations on the affected Kubernetes service",
			steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Validate JSON syntax in all ad.datadoghq.com/endpoints.* annotations on the service"},
				{Order: 2, Text: "Ensure check_names, init_configs, and instances arrays have matching lengths"},
				{Order: 3, Text: "Verify the annotation values match the expected Autodiscovery format for endpoint checks"},
			},
		}
	default:
		return issueContent{
			title:       "Autodiscovery Pod Annotation Misconfiguration",
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
