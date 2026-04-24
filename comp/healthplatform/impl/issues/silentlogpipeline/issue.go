// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package silentlogpipeline

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName     = "silent_log_pipeline"
	category      = "configuration"
	location      = "logs-agent"
	severity      = "warning"
	source        = "logs-agent"
	unknownVal    = "unknown"
	defaultImpact = "Logs from this source are not being collected or forwarded to Datadog"
)

// SilentLogPipelineIssue provides complete issue template for silent log pipeline issues
type SilentLogPipelineIssue struct{}

// NewSilentLogPipelineIssue creates a new silent log pipeline issue template
func NewSilentLogPipelineIssue() *SilentLogPipelineIssue {
	return &SilentLogPipelineIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation for silent log pipelines
func (t *SilentLogPipelineIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	logSource := context["source"]
	if logSource == "" {
		logSource = unknownVal
	}

	configFile := context["configFile"]
	if configFile == "" {
		configFile = unknownVal
	}

	inactiveDuration := context["inactiveDuration"]
	if inactiveDuration == "" {
		inactiveDuration = unknownVal
	}

	description := fmt.Sprintf(
		"The log source '%s' defined in '%s' has been configured for %s but no logs have been collected. "+
			"This may indicate a misconfiguration, permission issue, or the source is genuinely quiet.",
		logSource, configFile, inactiveDuration,
	)

	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: "Verify the log file path exists: ls -la <path>"},
		{Order: 2, Text: "Check the agent has read permissions: sudo -u dd-agent cat <path>"},
		{Order: 3, Text: "Ensure logs_enabled: true in datadog.yaml"},
		{Order: 4, Text: "Run agent diagnostics: datadog-agent logs check"},
		{Order: 5, Text: "Check if the source is intentionally quiet (no action needed if so)"},
	}

	extra, err := structpb.NewStruct(map[string]any{
		"source":            logSource,
		"config_file":       configFile,
		"inactive_duration": inactiveDuration,
		"impact":            defaultImpact,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   issueName,
		Title:       "Log Source Configured But No Logs Received",
		Description: description,
		Category:    category,
		Location:    location,
		Severity:    severity,
		DetectedAt:  "",
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify source path, permissions, and agent configuration",
			Steps:   steps,
		},
		Tags: []string{"logs", "configuration", "collection"},
	}, nil
}
