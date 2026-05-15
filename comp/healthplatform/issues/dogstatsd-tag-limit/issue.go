// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dogstatsdtaglimit

import (
	"fmt"
	"strconv"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// Issue provides the complete issue template for the DogStatsD tag count limit drop issue
type Issue struct{}

// NewIssue creates a new DogStatsD tag count limit issue template
func NewIssue() *Issue {
	return &Issue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *Issue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	maxTagsCount := context["maxTagsCount"]
	if maxTagsCount == "" {
		maxTagsCount = strconv.Itoa(defaultMaxTagsCount)
	}

	extra, err := structpb.NewStruct(map[string]any{
		"max_tags_count": maxTagsCount,
		"config_key":     "dogstatsd_max_tags_count",
		"impact":         "Metrics with more than the maximum number of tags are silently discarded, causing gaps in custom metrics dashboards",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "dogstatsd_tag_limit_drop",
		Title:       "DogStatsD Metrics Dropped Due to Tag Count Limit",
		Description: "DogStatsD silently drops metrics that exceed the maximum tag count limit (default: 100 tags). Metrics with more tags are silently discarded with no error message, causing invisible gaps in custom metrics.",
		Category:    "configuration",
		Location:    "dogstatsd",
		Severity:    "high",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "dogstatsd",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Either reduce the number of tags per metric or increase dogstatsd_max_tags_count in datadog.yaml",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check DogStatsD stats to identify metrics with many tags: datadog-agent dogstatsd-stats"},
				{Order: 2, Text: "Review how many tags are being sent per metric in your application code"},
				{Order: 3, Text: "Option A: Reduce tags per metric to stay under the limit (preferred for performance)"},
				{Order: 4, Text: "Option B: Increase the limit in datadog.yaml: dogstatsd_max_tags_count: 200"},
				{Order: 5, Text: "Restart the agent after making config changes: systemctl restart datadog-agent"},
			},
		},
		Tags: []string{"dogstatsd", "tags", "metrics-dropped", "configuration"},
	}, nil
}
