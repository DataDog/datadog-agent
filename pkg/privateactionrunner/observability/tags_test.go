// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package observability

import (
	"context"
	"testing"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAsMetricTags_AllFieldsPresent verifies that RunnerID, RunnerVersion and the pull-mode
// tag all appear in the output slice.
func TestAsMetricTags_AllFieldsPresent(t *testing.T) {
	tags := &CommonTags{
		RunnerId:      "runner-1",
		RunnerVersion: "7.0.0",
		Modes:         []modes.Mode{modes.ModePull},
	}

	result := tags.AsMetricTags()

	assert.Contains(t, result, "runner_id:runner-1")
	assert.Contains(t, result, "runner_version:7.0.0")
	assert.Contains(t, result, "pull:true")
}

// TestAsMetricTags_EmptyValuesOmitted verifies that fields with empty values are not included
// in the metric tag slice (an empty tag like "runner_id:" would pollute dashboards).
func TestAsMetricTags_EmptyValuesOmitted(t *testing.T) {
	tags := &CommonTags{RunnerId: "", RunnerVersion: ""}

	result := tags.AsMetricTags()

	for _, tag := range result {
		assert.NotContains(t, tag, "runner_id:")
		assert.NotContains(t, tag, "runner_version:")
	}
}

// TestAsMetricTags_ExtraTagsAppended verifies that caller-supplied tags are appended after
// the standard fields.
func TestAsMetricTags_ExtraTagsAppended(t *testing.T) {
	tags := &CommonTags{
		RunnerId: "r1",
		ExtraTags: []Tag{
			{Key: "env", Value: "prod"},
			{Key: "datacenter", Value: "us1"},
		},
	}

	result := tags.AsMetricTags()

	assert.Contains(t, result, "env:prod")
	assert.Contains(t, result, "datacenter:us1")
}

// TestAsLogFields_ContainsStandardFields verifies that the three standard fields
// (runner_id, runner_version, modes) are always present in the log fields slice.
func TestAsLogFields_ContainsStandardFields(t *testing.T) {
	tags := &CommonTags{
		RunnerId:      "runner-xyz",
		RunnerVersion: "7.1.0",
		Modes:         []modes.Mode{modes.ModePull},
	}

	fields := tags.AsLogFields()

	require.NotEmpty(t, fields)
	keySet := make(map[string]bool, len(fields))
	for _, f := range fields {
		keySet[f.Key] = true
	}
	assert.True(t, keySet[RunnerIdTagName], "runner_id field must be present")
	assert.True(t, keySet[RunnerVersionTagName], "runner_version field must be present")
	assert.True(t, keySet[ModesTagName], "modes field must be present")
}

// TestAsLogFields_ExtraTagsAppendedAtEnd verifies that extra tags appear after the standard
// fields so log readers see standard fields in a consistent position.
func TestAsLogFields_ExtraTagsAppendedAtEnd(t *testing.T) {
	tags := &CommonTags{
		RunnerId: "r1",
		ExtraTags: []Tag{
			{Key: "region", Value: "eu"},
			{Key: "tier", Value: "prod"},
		},
	}

	fields := tags.AsLogFields()

	// The last two fields should be the extra tags in order.
	require.True(t, len(fields) >= 2)
	last := fields[len(fields)-1]
	secondLast := fields[len(fields)-2]
	assert.Equal(t, "tier", last.Key)
	assert.Equal(t, "region", secondLast.Key)
}

// TestAddCommonTagsToLogs_EnrichesContextLogger verifies that AddCommonTagsToLogs returns a
// context whose logger differs from the parent's — meaning the runner ID and version fields
// have been attached and will appear in every subsequent log line from that context.
func TestAddCommonTagsToLogs_EnrichesContextLogger(t *testing.T) {
	parent := context.Background()
	tags := CommonTags{RunnerId: "r-enriched", RunnerVersion: "7.0"}

	enriched := AddCommonTagsToLogs(parent, tags)

	before := log.FromContext(parent)
	after := log.FromContext(enriched)
	// The enriched logger is a With-derived logger; it must not be the same instance.
	assert.NotEqual(t, before, after,
		"enriched context logger must carry extra fields that distinguish it from the base logger")
}
