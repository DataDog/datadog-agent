// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package adannotation

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName  = "ad_annotation_misconfiguration"
	category   = "autodiscovery"
	location   = "autodiscovery"
	severity   = "medium"
	source     = "autodiscovery"
	unknownVal = "unknown"
	failedMsg  = "Autodiscovery annotation error detected"
	impactMsg  = "Metrics, logs, or traces from this pod may not be collected due to misconfigured annotations"
)

// ADAnnotationIssue provides complete issue template for AD annotation misconfigurations
type ADAnnotationIssue struct{}

// NewADAnnotationIssue creates a new AD annotation issue template
func NewADAnnotationIssue() *ADAnnotationIssue {
	return &ADAnnotationIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for AD annotation errors
func (t *ADAnnotationIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	entityName := context["entityName"]
	if entityName == "" {
		entityName = unknownVal
	}

	errorMessage := context["errorMessage"]
	if errorMessage == "" {
		errorMessage = failedMsg
	}

	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: "Validate JSON syntax in all ad.datadoghq.com/<container>.* annotations"},
		{Order: 2, Text: "Verify the container name in the annotation matches an actual container in the pod spec"},
		{Order: 3, Text: "Ensure check_names, init_configs, and instances arrays have matching lengths"},
		{Order: 4, Text: "Run 'agent validate-pod-annotation' to check annotation syntax"},
		{Order: 5, Text: "See docs: https://docs.datadoghq.com/containers/kubernetes/integrations/"},
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
		Id:          IssueID,
		IssueName:   issueName,
		Title:       fmt.Sprintf("AD Annotation Error on '%s'", entityName),
		Description: fmt.Sprintf("Autodiscovery annotation error on '%s': %s", entityName, errorMessage),
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Review and fix autodiscovery annotations on the affected pod",
			Steps:   steps,
		},
		Tags: []string{"ad-annotation", "autodiscovery", entityName},
	}, nil
}
