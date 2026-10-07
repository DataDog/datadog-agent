// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsprofile

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/DataDog/datadog-agent/pkg/logs/profilerec"
)

func sampleWire() recommendation {
	return recommendation{
		Profile:             profilerec.ProfileHighConcurrency,
		ProfileVersion:      1,
		ProfileDescription:  "Raises send concurrency.",
		ProfileKeySource:    "default",
		ReasonCode:          profilerec.ReasonSendStageSaturatedHighLatency,
		Reason:              "Logs are being lost. The logs pipeline is bottlenecked at the network send stage, with high intake latency (420ms).",
		Bottleneck:          sendStage,
		BackpressureState:   "SATURATED",
		SenderLatencyMs:     420,
		DroppedRecently:     true,
		Saturated30mSeconds: 27 * 60,
		Current: []settingWire{
			{Key: "logs_config.batch_max_concurrent_send", Value: 0.0, Source: "default"},
		},
		Changes: []changeWire{
			{Key: "logs_config.batch_max_concurrent_send", From: 0.0, To: 10.0},
		},
	}
}

func encode(t *testing.T, w recommendation) map[string]string {
	t.Helper()
	b, err := json.Marshal(w)
	require.NoError(t, err)
	return map[string]string{contextKeyRecommendation: string(b)}
}

// roundTrip returns Extra as the intake sees it: structpb to protojson to a generic map.
func roundTrip(t *testing.T, issue *healthplatform.Issue) map[string]any {
	t.Helper()
	b, err := protojson.Marshal(issue.Extra)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func extraRecommendation(t *testing.T, issue *healthplatform.Issue) map[string]any {
	t.Helper()
	rec, ok := roundTrip(t, issue)["recommendation"].(map[string]any)
	require.True(t, ok, "Extra.recommendation must be an object")
	return rec
}

func extraStrings(t *testing.T, issue *healthplatform.Issue, key string) []string {
	t.Helper()
	list, ok := extraRecommendation(t, issue)[key].([]any)
	require.True(t, ok, "%s must be a list", key)
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		require.True(t, ok, "%s elements must be strings, got %T", key, v)
		out = append(out, s)
	}
	return out
}

func TestBuildIssue(t *testing.T) {
	tests := []struct {
		name     string
		template interface {
			BuildIssue(map[string]string) (*healthplatform.Issue, error)
		}
		issueName string
		issueType string
		severity  healthplatform.IssueSeverity
	}{
		{"recommended", RecommendedIssue{}, IssueName, IssueType, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH},
		{"suggested", SuggestedIssue{}, SuggestedIssueName, SuggestedIssueType, healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue, err := tt.template.BuildIssue(encode(t, sampleWire()))
			require.NoError(t, err)

			assert.Empty(t, issue.Id)
			assert.Equal(t, tt.issueName, issue.IssueName)
			assert.Equal(t, tt.issueType, issue.IssueType)
			assert.Equal(t, tt.severity, issue.Severity)
			assert.Equal(t, "logs_pipeline", issue.Category)
			assert.NotEmpty(t, issue.Title)
			assert.NotEmpty(t, issue.Description)
			assert.NotEmpty(t, issue.Source)
			assert.Contains(t, issue.Title, "high-concurrency")
			assert.Contains(t, issue.Description, "network send stage")
			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Summary)
			assert.Empty(t, issue.Remediation.Script)
			for i, step := range issue.Remediation.Steps {
				assert.EqualValues(t, i+1, step.Order)
			}
			assert.Contains(t, issue.Remediation.Steps[1].Text, "logs_config.profile: high-concurrency")
			assert.Contains(t, issue.Remediation.Steps[1].Text, "logs_config.profile_version: 1")
		})
	}
}

func TestBuildIssue_DescriptionDistinguishesLoss(t *testing.T) {
	w := sampleWire()
	high, err := RecommendedIssue{}.BuildIssue(encode(t, w))
	require.NoError(t, err)
	assert.Contains(t, high.Description, "Logs are being lost")

	w.Reason = "No logs are being lost right now. The logs pipeline is bottlenecked at the network send stage."
	low, err := SuggestedIssue{}.BuildIssue(encode(t, w))
	require.NoError(t, err)
	assert.Contains(t, low.Description, "No logs are being lost")
}

func TestBuildIssue_ExtraContract(t *testing.T) {
	issue, err := RecommendedIssue{}.BuildIssue(encode(t, sampleWire()))
	require.NoError(t, err)

	rec := extraRecommendation(t, issue)
	assert.Equal(t, "logs_performance_profile", rec["kind"])
	assert.Equal(t, "high-concurrency", rec["profile"])
	assert.Equal(t, float64(1), rec["profile_version"])
	assert.Equal(t, float64(1), rec["schema_version"])
	assert.Equal(t, "send_stage_saturated_high_latency", rec["reason_code"])
	assert.Equal(t, "default", rec["profile_key_source"])
	assert.Equal(t, float64(1800), rec["verify_window_s"])

	changes, ok := rec["changes"].([]any)
	require.True(t, ok)
	require.Len(t, changes, 1)
	assert.Equal(t, map[string]any{"key": "logs_config.batch_max_concurrent_send", "from": float64(0), "to": float64(10)}, changes[0])

	assert.Equal(t, []string{
		"Network send stage (destination_reliable_0) saturated for 27m in the last 30m",
		"Intake latency 420 ms",
		"Logs dropped in the last 5 minutes",
		"Logs pipeline backpressure state: SATURATED",
	}, extraStrings(t, issue, "evidence"))
	assert.Equal(t, []string{"More payloads in flight at once; higher memory use under load."}, extraStrings(t, issue, "tradeoffs"))

	current, ok := rec["current"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "", current["profile"])
	assert.Equal(t, []any{map[string]any{"key": "logs_config.batch_max_concurrent_send", "value": float64(0), "source": "default"}}, current["settings"])

	detail, ok := rec["evidence_detail"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{
		"bottleneck":          sendStage,
		"backpressure_state":  "SATURATED",
		"sender_latency_ms":   float64(420),
		"logs_dropped_recent": true,
		"bytes_missed_recent": false,
		"saturated_30m_s":     float64(1620),
	}, detail)
	assert.NotContains(t, rec, "blocked_keys")
}

func TestBuildIssue_BlockedKeysAndTradeoffs(t *testing.T) {
	w := sampleWire()
	w.Blocked = []blockedWire{{Key: "logs_config.pipelines", Source: "file"}, {Key: "logs_config.payload_channel_size", Source: "environment-variable"}}

	issue, err := RecommendedIssue{}.BuildIssue(encode(t, w))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"More payloads in flight at once; higher memory use under load.",
		"2 setting(s) are set explicitly on this host and will not change: logs_config.pipelines, logs_config.payload_channel_size.",
	}, extraStrings(t, issue, "tradeoffs"))
	blocked, ok := extraRecommendation(t, issue)["blocked_keys"].([]any)
	require.True(t, ok)
	assert.Len(t, blocked, 2)
}

func TestBuildIssue_HighThroughputTradeoff(t *testing.T) {
	w := sampleWire()
	w.Profile = profilerec.ProfileHighThroughput
	w.Bottleneck = "processor"

	issue, err := SuggestedIssue{}.BuildIssue(encode(t, w))
	require.NoError(t, err)

	assert.Equal(t, []string{"One pipeline per CPU core, up to 16; more CPU and memory under load."}, extraStrings(t, issue, "tradeoffs"))
	assert.NotContains(t, extraStrings(t, issue, "evidence"), "Intake latency 420 ms")
}

func TestBuildIssue_EnvVarProfileKeyChangesRemediation(t *testing.T) {
	w := sampleWire()
	w.ProfileKeySource = "environment-variable"

	issue, err := RecommendedIssue{}.BuildIssue(encode(t, w))
	require.NoError(t, err)

	assert.Contains(t, issue.Remediation.Steps[1].Text, "DD_LOGS_CONFIG_PROFILE=high-concurrency")
	assert.Equal(t, "environment-variable", extraRecommendation(t, issue)["profile_key_source"])
}

func TestBuildIssue_MissedBytesEvidence(t *testing.T) {
	w := sampleWire()
	w.DroppedRecently, w.MissedRecently = false, true

	issue, err := RecommendedIssue{}.BuildIssue(encode(t, w))
	require.NoError(t, err)

	assert.Contains(t, extraStrings(t, issue, "evidence"), "Log data lost to file rotation in the last 5 minutes")
}

func TestBuildIssue_WithoutRecommendationOffersNoDeploy(t *testing.T) {
	for name, ctx := range map[string]map[string]string{
		"nil":       nil,
		"empty":     {},
		"malformed": {contextKeyRecommendation: "{"},
	} {
		t.Run(name, func(t *testing.T) {
			issue, err := RecommendedIssue{}.BuildIssue(ctx)
			require.NoError(t, err)

			assert.Equal(t, IssueName, issue.IssueName)
			assert.NotEmpty(t, issue.Title)
			assert.NotEmpty(t, issue.Description)
			assert.NotEmpty(t, issue.Remediation.Steps)
			assert.NotContains(t, issue.Extra.GetFields(), "recommendation")
		})
	}
}

func TestScalar(t *testing.T) {
	assert.Equal(t, "", scalar(nil))
	assert.Equal(t, true, scalar(true))
	assert.Equal(t, "x", scalar("x"))
	assert.Equal(t, float64(3), scalar(3))
	assert.Equal(t, float64(3), scalar(uint16(3)))
	assert.Equal(t, 1.5, scalar(float32(1.5)))
	assert.Equal(t, "1m0s", scalar(time.Minute))
	assert.Equal(t, "[a b]", scalar([]string{"a", "b"}))
}
