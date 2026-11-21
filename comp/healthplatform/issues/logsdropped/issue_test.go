// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

var lastDrop = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

// authOutage is the common case: a rejected API key loses everything the main
// endpoint was sent.
func authOutage() facts {
	return facts{
		MainLogs:    12400,
		MainBytes:   38 << 20,
		Processed:   12400,
		StartedAt:   lastDrop.Add(-2 * time.Hour),
		SourceCount: 2,
		TopSources:  []sourceLoss{{"nginx", "web", 12000}, {"redis", "cache", 400}},
		Reasons:     []logsmetrics.DropReason{logsmetrics.DropReasonAuth},
		TopReason:   logsmetrics.DropReasonAuth,
		Endpoints:   map[logsmetrics.DropReason][]string{logsmetrics.DropReasonAuth: {intake}},
		LastDropAt:  lastDrop,
	}
}

func build(t *testing.T, f facts) *healthplatform.Issue {
	t.Helper()
	issue, err := LogsDroppedIssue{}.BuildIssue(f.context())
	require.NoError(t, err)
	assertCommon(t, issue)
	return issue
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
	assert.Equal(t, remediationSummary, issue.Remediation.Summary)
	for i, s := range issue.Remediation.Steps {
		assert.Equal(t, int32(i+1), s.Order, "steps must stay contiguous and 1-indexed")
	}
	assert.Equal(t, firstStep, issue.Remediation.Steps[0].Text)

	// Fleet drops lists of objects, so Extra holds only scalars and string lists.
	require.NotNil(t, issue.Extra)
	for key, v := range issue.Extra.GetFields() {
		if list := v.GetListValue(); list != nil {
			for _, item := range list.GetValues() {
				assert.NotNil(t, item.GetKind(), key)
				assert.Nil(t, item.GetStructValue(), "%s must be a list of strings", key)
			}
		}
	}
	assert.NotEmpty(t, issue.Extra.GetFields()["impact"].GetStringValue())
}

func TestBuildIssue_MainEndpointAuthRejection(t *testing.T) {
	issue := build(t, authOutage())

	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
	assert.Equal(t, "Dropped 12,400 logs (40 MB) from 2 sources", issue.Title)
	assert.Contains(t, issue.Description, "Since 2026-11-12 08:00 UTC, logs from 2 sources were read by the Agent but never delivered because the intake rejected the API key")
	assert.Contains(t, issue.Description, "Most affected: nginx/web: 12,000, redis/cache: 400.")
	assert.Contains(t, issue.Description, "lost 100% of the 12,400 logs the Agent processed since then")
	assert.Equal(t, "Lost 12,400 logs from nginx/web and redis/cache: the intake rejected the API key, or the Agent had none (HTTP 401/403).",
		issue.Extra.GetFields()["impact"].GetStringValue())
	assert.Contains(t, issue.Tags, "auth")

	steps := stepTexts(issue)
	require.Len(t, steps, 4, "status, log search, one per reason, recheck")
	assert.Contains(t, steps[2], "The intake ("+intake+") rejected the API key", "the step names the real endpoint")
	assert.Equal(t, lastStep, steps[3])
}

func TestBuildIssue_CopiesOnlyIsMedium(t *testing.T) {
	f := facts{
		CopyLogs:    10,
		CopyBytes:   1000,
		Processed:   1000,
		SourceCount: 1,
		TopSources:  []sourceLoss{{"nginx", "web", 10}},
		Reasons:     []logsmetrics.DropReason{logsmetrics.DropReasonBufferFull},
		TopReason:   logsmetrics.DropReasonBufferFull,
		Endpoints:   map[logsmetrics.DropReason][]string{logsmetrics.DropReasonBufferFull: {"intake.eu"}},
	}
	issue := build(t, f)

	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
	assert.Equal(t, `Dropped 10 log copies (1.0 kB) from source "nginx" (service "web")`, issue.Title)
	assert.Contains(t, issue.Description, "only copies bound for other endpoints were lost")
	assert.True(t, strings.HasSuffix(issue.Extra.GetFields()["impact"].GetStringValue(), "The main endpoint was not affected."))
}

func TestBuildIssue_DroppedBeforeSendingNamesNoEndpoint(t *testing.T) {
	f := authOutage()
	f.Reasons = []logsmetrics.DropReason{logsmetrics.DropReasonTooLarge}
	f.TopReason = logsmetrics.DropReasonTooLarge
	f.Endpoints = nil
	issue := build(t, f)

	assert.Contains(t, stepTexts(issue)[2], "Payloads were too large for the intake, or")
}

// Every reason has a phrase and its own step, and a step per reason seen.
func TestBuildIssue_EveryReasonHasPhraseAndStep(t *testing.T) {
	f := authOutage()
	f.Reasons = reasonOrder
	issue := build(t, f)

	steps := stepTexts(issue)
	require.Len(t, steps, len(reasonOrder)+3)
	seen := map[string]bool{}
	for i, r := range reasonOrder {
		assert.NotEmpty(t, reasonPhrases[r], r)
		assert.False(t, seen[steps[i+2]], "%s duplicates another reason's step", r)
		seen[steps[i+2]] = true
		assert.Contains(t, issue.Tags, string(r))
	}
}

func TestBuildIssue_SeverityThreshold(t *testing.T) {
	for _, tc := range []struct {
		lost, processed int64
		want            healthplatform.IssueSeverity
	}{
		{lost: 1, processed: 1000, want: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM},
		{lost: 10, processed: 1000, want: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH},
		{lost: 1, processed: 0, want: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH},
	} {
		f := authOutage()
		f.MainLogs, f.Processed = tc.lost, tc.processed
		assert.Equal(t, tc.want, build(t, f).Severity, "lost %d of %d", tc.lost, tc.processed)
	}
}

func TestFormatRatio(t *testing.T) {
	assert.Equal(t, "0%", formatRatio(0))
	assert.Equal(t, "less than 0.01%", formatRatio(0.00001))
	assert.Equal(t, "0.25%", formatRatio(0.0025))
	assert.Equal(t, "2.5%", formatRatio(0.025))
	assert.Equal(t, "25%", formatRatio(0.25))
}

func TestFacts_ContextRoundTrip(t *testing.T) {
	f := authOutage()
	f.CopyLogs, f.CopyBytes = 7, 700
	f.Reasons = []logsmetrics.DropReason{logsmetrics.DropReasonAuth, logsmetrics.DropReasonBufferFull}
	f.Endpoints[logsmetrics.DropReasonBufferFull] = []string{"a.example", "b.example"}

	assert.Equal(t, f, parseFacts(f.context()))
}

// The context is the trust boundary: names are bounded and stripped of control
// characters, and unknown reasons never reach remediation.
func TestBuildIssue_SanitizesContext(t *testing.T) {
	raw := authOutage()
	raw.TopSources = []sourceLoss{{strings.Repeat("x", 300), "svc\x1b[31m", 5}}
	raw.Reasons = []logsmetrics.DropReason{logsmetrics.DropReasonAuth, "made_up"}
	raw.TopReason = "made_up"
	raw.MainLogs = -5
	ctx := raw.context()

	f := parseFacts(ctx)
	require.Len(t, f.TopSources, 1)
	assert.Len(t, []rune(f.TopSources[0].Source), 64)
	assert.NotContains(t, f.TopSources[0].Service, "\x1b")
	assert.Equal(t, []logsmetrics.DropReason{logsmetrics.DropReasonAuth}, f.Reasons)
	assert.Equal(t, logsmetrics.DropReasonAuth, f.TopReason)
	assert.Zero(t, f.MainLogs, "negative counts read as zero")

	issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assertCommon(t, issue)
}

func TestBuildIssue_EmptyAndMalformedContext(t *testing.T) {
	for _, ctx := range []map[string]string{
		{},
		{keyFacts: "{not json"},
		{keyFacts: `{"MainLogs":"nope","TopSources":"garbage","Reasons":{}}`},
	} {
		issue, err := LogsDroppedIssue{}.BuildIssue(ctx)
		require.NoError(t, err)
		assertCommon(t, issue)
		assert.Equal(t, "unknown", issue.Extra.GetFields()[keyLastDropAt].GetStringValue())
		assert.Equal(t, "unknown", issue.Extra.GetFields()[keyStartedAt].GetStringValue())
		assert.True(t, strings.HasPrefix(issue.Description, "Logs from "), "no start time, no 'Since'")
	}
}
