// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

const intake = "agent-http-intake.logs.datadoghq.com"

func encode(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func issueContext(t *testing.T, primary bool, sourceCount int, causes []cause, drops []sourceDrop) map[string]string {
	t.Helper()
	var logs, bytes int64
	for _, c := range causes {
		logs += c.Logs
		bytes += c.Bytes
	}
	p := "false"
	var primaryLogs, primaryBytes int64
	primarySources := 0
	reasons := map[string]bool{}
	for _, c := range causes {
		reasons[c.Reason] = true
	}
	if primary {
		p = "true"
		for _, c := range causes {
			if c.Kind == "primary" || c.Kind == "all" {
				primaryLogs += c.Logs
				primaryBytes += c.Bytes
			}
		}
		primarySources = sourceCount
	}
	reasonList := make([]string, 0, len(reasons))
	for r := range reasons {
		reasonList = append(reasonList, r)
	}
	// By default the drops are everything the Agent processed: a total outage.
	return map[string]string{
		contextKeyPrimaryLogs:     jsonInt(primaryLogs),
		contextKeyPrimaryBytes:    jsonInt(primaryBytes),
		contextKeyPrimarySources:  jsonInt(int64(primarySources)),
		contextKeyReasons:         strings.Join(reasonList, ","),
		contextKeyProcessed:       jsonInt(logs),
		contextKeyLogs:            jsonInt(logs),
		contextKeyBytes:           jsonInt(bytes),
		contextKeySourceCount:     jsonInt(int64(sourceCount)),
		contextKeyRowsOmitted:     "0",
		contextKeyCausesOmitted:   "0",
		contextKeyPrimaryAffected: p,
		contextKeyLastDropAt:      "2026-11-12T10:00:00Z",
		contextKeyCauses:          encode(t, causes),
		contextKeyDrops:           encode(t, drops),
	}
}

func jsonInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func stepTexts(issue *healthplatform.Issue) []string {
	texts := make([]string, 0, len(issue.Remediation.Steps))
	for _, s := range issue.Remediation.Steps {
		texts = append(texts, s.Text)
	}
	return texts
}

func assertCommon(t *testing.T, issue *healthplatform.Issue) {
	t.Helper()
	assert.Empty(t, issue.Id, "the runner sets Id, never the template")
	assert.Equal(t, IssueName, issue.IssueName)
	assert.Equal(t, IssueType, issue.IssueType)
	assert.Equal(t, "logs_pipeline", issue.Category)
	assert.Equal(t, "logs-agent", issue.Location)
	assert.Equal(t, "logs", issue.Source)
	assert.NotContains(t, issue.Description, "\n", "agent diagnose prints Description verbatim on one line")
	require.NotNil(t, issue.Remediation)
	assert.NotEmpty(t, issue.Remediation.Summary)
	for i, s := range issue.Remediation.Steps {
		assert.Equal(t, int32(i+1), s.Order, "steps must stay contiguous and 1-indexed")
	}
	assert.Contains(t, issue.Remediation.Steps[0].Text, "datadog-agent status", "step 1 is always the fastest diagnostic")

	require.NotNil(t, issue.Extra)
	fields := issue.Extra.GetFields()
	for _, key := range []string{contextKeyLogs, contextKeyBytes, contextKeySourceCount, contextKeyRowsOmitted,
		contextKeyCausesOmitted, contextKeyPrimaryAffected, contextKeyPrimaryLogs, contextKeyPrimaryBytes,
		contextKeyPrimarySources, contextKeyProcessed, contextKeyReasons, extraKeyLossRatio,
		contextKeyLastDropAt, contextKeyCauses, contextKeyDrops} {
		assert.Contains(t, fields, key)
	}
}

func TestBuildIssue_PrimaryAuthRejection(t *testing.T) {
	ctx := issueContext(t, true, 2,
		[]cause{{Reason: string(logsmetrics.DropReasonAuthRejected), Destination: intake, Kind: "primary", Logs: 12400, Bytes: 38 << 20}},
		[]sourceDrop{
			{Source: "nginx", Service: "web", Reason: "auth_rejected", Destination: intake, Kind: "primary", Logs: 12000, Bytes: 37 << 20},
			{Source: "redis", Service: "cache", Reason: "auth_rejected", Destination: intake, Kind: "primary", Logs: 400, Bytes: 1 << 20},
		})

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assertCommon(t, issue)

	assert.Equal(t, "Dropped 12,400 logs (40 MB) from 2 sources in the last 24 hours", issue.Title)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
	assert.Contains(t, issue.Description, "12,400 because the intake rejected the API key (HTTP 401/403) at the main endpoint "+intake)
	assert.Contains(t, issue.Description, "Most affected: nginx/web 12,000, redis/cache 400.")
	assert.NotContains(t, issue.Description, "still received")
	assert.Contains(t, issue.Description, "The main endpoint lost 100% of the 12,400 logs the Agent processed in this period.")
	assert.Equal(t, float64(1), issue.Extra.GetFields()[extraKeyLossRatio].GetNumberValue())
	assert.Contains(t, issue.Tags, "auth_rejected")

	steps := stepTexts(issue)
	require.Len(t, steps, 4)
	assert.Contains(t, steps[2], "api_key")
	assert.Contains(t, steps[3], "clears 24 hours after")

	causes := issue.Extra.GetFields()[contextKeyCauses].GetListValue().GetValues()
	require.Len(t, causes, 1)
	assert.Equal(t, "auth_rejected", causes[0].GetStructValue().GetFields()["reason"].GetStringValue())
	assert.True(t, issue.Extra.GetFields()[contextKeyPrimaryAffected].GetBoolValue())
}

// Only a copy was lost: the customer's own org has the logs, so it is a warning.
func TestBuildIssue_AdditionalEndpointOnlyIsMedium(t *testing.T) {
	ctx := issueContext(t, false, 1,
		[]cause{{Reason: "buffer_full", Destination: "intake.eu", Kind: "additional", Logs: 5, Bytes: 500}},
		[]sourceDrop{{Source: "nginx", Service: "web", Reason: "buffer_full", Destination: "intake.eu", Kind: "additional", Logs: 5, Bytes: 500}})

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assertCommon(t, issue)

	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
	assert.Equal(t, `Dropped 5 log copies (500 B) from source "nginx" (service "web") in the last 24 hours`, issue.Title,
		"only copies bound for an additional endpoint were lost")
	assert.Contains(t, issue.Description, "at the additional endpoint intake.eu")
	assert.Contains(t, issue.Description, "The main endpoint still received these logs")
	assert.NotContains(t, issue.Description, "Most affected", "a single named source needs no breakdown")
	assert.Contains(t, stepTexts(issue)[2], "fell behind")
}

func TestBuildIssue_DroppedBeforeSending(t *testing.T) {
	ctx := issueContext(t, true, 1,
		[]cause{{Reason: "message_too_large", Kind: "all", Logs: 2, Bytes: 2 << 20}},
		[]sourceDrop{{Source: "java", Service: "api", Reason: "message_too_large", Kind: "all", Logs: 2, Bytes: 2 << 20}})

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)

	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
	assert.Contains(t, issue.Description, "because log lines were larger than a whole batch before sending")
	assert.Contains(t, stepTexts(issue)[2], "max_message_size_bytes")
}

// Each reason adds its own step, in a fixed order, and the two support-only
// reasons share one flare step.
func TestBuildIssue_RemediationBranches(t *testing.T) {
	for _, reason := range reasonOrder {
		t.Run(string(reason), func(t *testing.T) {
			ctx := issueContext(t, true, 1,
				[]cause{{Reason: string(reason), Destination: intake, Kind: "primary", Logs: 1, Bytes: 1}}, nil)
			issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
			require.NoError(t, err)
			steps := stepTexts(issue)
			require.Len(t, steps, 4)
			assert.Equal(t, reasonSteps[reason], steps[2])
			_, phrased := reasonPhrases[reason]
			assert.True(t, phrased, "every reason needs a description phrase")
		})
	}

	ctx := issueContext(t, true, 1, []cause{
		{Reason: "encode_failed", Kind: "all", Logs: 1},
		{Reason: "bad_request", Destination: intake, Kind: "primary", Logs: 1},
		{Reason: "auth_rejected", Destination: intake, Kind: "primary", Logs: 1},
	}, nil)
	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	steps := stepTexts(issue)
	require.Len(t, steps, 5, "status, log search, auth, one flare, confirm")
	assert.Equal(t, reasonSteps[logsmetrics.DropReasonAuthRejected], steps[2], "auth comes before the flare")
	assert.Equal(t, flareStep, steps[3])
}

func TestBuildIssue_ReasonOrderCoversEveryReason(t *testing.T) {
	assert.Len(t, reasonSteps, len(reasonOrder))
	assert.Len(t, reasonPhrases, len(reasonOrder))
}

func TestBuildIssue_OmittedCountsReachDescription(t *testing.T) {
	ctx := issueContext(t, true, 12,
		[]cause{{Reason: "auth_rejected", Destination: intake, Kind: "primary", Logs: 10}},
		[]sourceDrop{{Source: "a", Service: "a", Logs: 10}})
	ctx[contextKeyRowsOmitted] = "11"
	ctx[contextKeyCausesOmitted] = "2"

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Contains(t, issue.Description, "and 2 other causes")
	assert.Contains(t, issue.Description, "and 11 other rows")
}

func TestBuildIssue_SanitizesNames(t *testing.T) {
	ctx := issueContext(t, true, 1,
		[]cause{{Reason: "auth_rejected", Destination: "evil\nhost", Kind: "primary", Logs: 1}},
		[]sourceDrop{{Source: "ngi\x00nx" + strings.Repeat("x", 200), Service: "web\r", Logs: 1}})

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.NotContains(t, issue.Description, "\n")
	assert.NotContains(t, issue.Title, "\x00")
	assert.Contains(t, issue.Description, "evilhost")
	assert.Contains(t, issue.Title, "...")
}

func TestBuildIssue_EmptyAndMalformedContext(t *testing.T) {
	for name, ctx := range map[string]map[string]string{
		"nil":       nil,
		"empty":     {},
		"malformed": {contextKeyCauses: "{not json", contextKeyDrops: "[1,2", contextKeyLogs: "x"},
	} {
		t.Run(name, func(t *testing.T) {
			issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
			require.NoError(t, err)
			assertCommon(t, issue)
			assert.Equal(t, "Dropped 0 logs (0 B) from 0 sources in the last 24 hours", issue.Title)
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity,
				"without the flag, assume the main endpoint lost logs rather than understate it")
			assert.Len(t, issue.Remediation.Steps, 3)
			assert.Equal(t, "unknown", issue.Extra.GetFields()[contextKeyLastDropAt].GetStringValue())
		})
	}
}

// A main endpoint that briefly fell behind loses a sliver of its logs: worth a
// warning, not a Broken Agent.
func TestBuildIssue_SmallMainEndpointLossIsMedium(t *testing.T) {
	ctx := issueContext(t, true, 1,
		[]cause{{Reason: "buffer_full", Destination: intake, Kind: "primary", Logs: 316, Bytes: 31600}},
		[]sourceDrop{{Source: "nginx", Service: "web", Reason: "buffer_full", Destination: intake, Kind: "primary", Logs: 316}})
	ctx[contextKeyProcessed] = "40000000"

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assertCommon(t, issue)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
	assert.Contains(t, issue.Description, "The main endpoint lost less than 0.01% of the 40,000,000 logs the Agent processed in this period.")
}

func TestBuildIssue_SeverityThreshold(t *testing.T) {
	for _, tc := range []struct {
		lost, processed int64
		want            healthplatform.IssueSeverity
	}{
		{99, 10000, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM},
		{100, 10000, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH},
		{5, 0, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH}, // no denominator: assume the worst
		{500, 100, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH},
	} {
		ctx := issueContext(t, true, 1, []cause{{Reason: "buffer_full", Destination: intake, Kind: "primary", Logs: tc.lost}}, nil)
		ctx[contextKeyProcessed] = jsonInt(tc.processed)
		issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
		require.NoError(t, err)
		assert.Equal(t, tc.want, issue.Severity, "lost %d of %d", tc.lost, tc.processed)
	}
}

func TestFormatRatio(t *testing.T) {
	for r, want := range map[float64]string{
		1: "100%", 0.25: "25%", 0.012: "1.2%", 0.0012: "0.12%", 0.00012: "0.01%", 0.0000079: "less than 0.01%", 0: "0%",
	} {
		assert.Equal(t, want, formatRatio(r), "%v", r)
	}
}

// One payload rejected at the main endpoint and at a best-effort copy is 1,000 lost
// logs, not 2,000: the title counts the main endpoint's loss, the causes show both.
func TestBuildIssue_TitleCountsLogsNotCopies(t *testing.T) {
	ctx := issueContext(t, true, 1, []cause{
		{Reason: "auth_rejected", Destination: intake, Kind: "primary", Logs: 1000, Bytes: 100000},
		{Reason: "auth_rejected", Destination: "copy.example.com", Kind: "best_effort", Logs: 1000, Bytes: 100000},
	}, []sourceDrop{{Source: "nginx", Service: "web", Logs: 1000}, {Source: "nginx", Service: "web", Logs: 1000}})

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(issue.Title, "Dropped 1,000 logs (100 kB)"), issue.Title)
	assert.Contains(t, issue.Description, "at the main endpoint "+intake)
	assert.Contains(t, issue.Description, "at the best-effort endpoint copy.example.com")
	assert.Equal(t, float64(2000), issue.Extra.GetFields()[contextKeyLogs].GetNumberValue(), "Extra keeps the copy total")
}

// The causes list is capped; a reason that only appears past the cap still gets
// its remediation step.
func TestBuildIssue_RemediationCoversReasonsPastTheCap(t *testing.T) {
	ctx := issueContext(t, true, 1, []cause{{Reason: "buffer_full", Destination: intake, Kind: "primary", Logs: 100}}, nil)
	ctx[contextKeyReasons] = "auth_rejected,buffer_full"
	ctx[contextKeyCausesOmitted] = "1"

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	steps := stepTexts(issue)
	assert.Contains(t, steps, reasonSteps[logsmetrics.DropReasonAuthRejected])
	assert.Contains(t, steps, reasonSteps[logsmetrics.DropReasonBufferFull])
	assert.Contains(t, issue.Tags, "auth_rejected")
}
