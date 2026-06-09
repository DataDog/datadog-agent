// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package forwarderimpl

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSeverityString(t *testing.T) {
	tests := []struct {
		severity healthplatform.IssueSeverity
		expected string
	}{
		{healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW, "low"},
		{healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, "medium"},
		{healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, "high"},
		{healthplatform.IssueSeverity_ISSUE_SEVERITY_UNSPECIFIED, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, severityString(tt.severity))
	}
}

func TestMarshalHealthReport_SeverityAsString(t *testing.T) {
	extra, err := structpb.NewStruct(map[string]any{"key": "value"})
	require.NoError(t, err)

	report := &healthplatform.HealthReport{
		SchemaVersion: "1.0.0",
		EventType:     "agent-health",
		EmittedAt:     "2026-01-01T00:00:00Z",
		Host:          &healthplatform.HostInfo{Hostname: "my-host"},
		Issues: map[string]*healthplatform.Issue{
			"medium-issue": {
				Id:        "mid-1",
				IssueName: "check-failure",
				Severity:  healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
				Category:  "integration",
				Extra:     extra,
			},
			"high-issue": {
				Id:       "hid-1",
				Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
			},
			"low-issue": {
				Id:       "lid-1",
				Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW,
			},
		},
	}

	payload, err := marshalHealthReport(report)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))

	assert.Equal(t, "1.0.0", decoded["schema_version"])
	assert.Equal(t, "my-host", decoded["host"].(map[string]any)["hostname"])

	issues := decoded["issues"].(map[string]any)
	assert.Equal(t, "medium", issues["medium-issue"].(map[string]any)["severity"])
	assert.Equal(t, "high", issues["high-issue"].(map[string]any)["severity"])
	assert.Equal(t, "low", issues["low-issue"].(map[string]any)["severity"])

	// Extra must be a JSON object, not double-encoded.
	extra_ := issues["medium-issue"].(map[string]any)["extra"]
	assert.IsType(t, map[string]any{}, extra_)
}

func TestMarshalHealthReport_PersistedIssueOmitted(t *testing.T) {
	report := &healthplatform.HealthReport{
		Issues: map[string]*healthplatform.Issue{
			"issue-1": {
				Id:       "i1",
				Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
				PersistedIssue: &healthplatform.PersistedIssue{
					State:     healthplatform.IssueState_ISSUE_STATE_ONGOING,
					FirstSeen: "2026-01-01T00:00:00Z",
				},
			},
		},
	}

	payload, err := marshalHealthReport(report)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))

	issue := decoded["issues"].(map[string]any)["issue-1"].(map[string]any)
	assert.Nil(t, issue["persisted_issue"], "persisted_issue must not be sent to intake")
}
