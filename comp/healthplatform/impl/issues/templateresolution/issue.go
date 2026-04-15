// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package templateresolution

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName  = "autodiscovery_template_resolution_failure"
	category   = "configuration"
	location   = "autodiscovery"
	severity   = "high"
	source     = "autodiscovery"
	unknownVal = "unknown"
	impactMsg  = "The check instance was silently skipped and will not collect data for this service"
)

// TemplateResolutionIssue provides complete issue template for autodiscovery template resolution failures
type TemplateResolutionIssue struct{}

// NewTemplateResolutionIssue creates a new template resolution issue template
func NewTemplateResolutionIssue() *TemplateResolutionIssue {
	return &TemplateResolutionIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for template resolution failures
func (t *TemplateResolutionIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	templateName := context["templateName"]
	if templateName == "" {
		templateName = unknownVal
	}

	serviceID := context["serviceID"]
	if serviceID == "" {
		serviceID = unknownVal
	}

	errorMessage := context["errorMessage"]
	if errorMessage == "" {
		errorMessage = "template resolution failed"
	}

	adIdentifiers := context["adIdentifiers"]
	configSource := context["source"]
	provider := context["provider"]

	title := fmt.Sprintf("Autodiscovery template '%s' skipped for service", templateName)
	desc := fmt.Sprintf(
		"Template '%s' could not be resolved for service %s: %s. The check instance was not scheduled.",
		templateName, serviceID, errorMessage,
	)

	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: "Check that all template variables (%%var%%) are supported by the autodiscovery listener for this service type"},
		{Order: 2, Text: "Run 'datadog-agent configcheck' to see all configuration resolution warnings"},
		{Order: 3, Text: "Review the AD identifiers and ensure they match the correct listener (e.g., RDS vs Aurora have different supported variables)"},
		{Order: 4, Text: "See docs: https://docs.datadoghq.com/containers/guide/template_variables/"},
		{Order: 5, Text: "Enable debug logging ('log_level: debug' in datadog.yaml) for full resolution details"},
	}

	extraMap := map[string]any{
		"template_name": templateName,
		"service_id":    serviceID,
		"error_message": errorMessage,
		"impact":        impactMsg,
	}
	if adIdentifiers != "" {
		extraMap["ad_identifiers"] = adIdentifiers
	}
	if configSource != "" {
		extraMap["config_source"] = configSource
	}
	if provider != "" {
		extraMap["provider"] = provider
	}

	extra, err := structpb.NewStruct(extraMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	tags := []string{"autodiscovery", "template-resolution", templateName}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   issueName,
		Title:       title,
		Description: desc,
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify that all template variables are supported by the autodiscovery listener for this service",
			Steps:   steps,
		},
		Tags: tags,
	}, nil
}
