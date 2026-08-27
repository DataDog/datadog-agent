// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// recentTimestamp feeds last_loss_at, which reaches Extra but no longer appears
// in the prose — the platform tracks last-seen per issue itself.
func recentTimestamp() string {
	return time.Now().Add(-90 * time.Minute).UTC().Format(time.RFC3339)
}

func TestBuildIssue(t *testing.T) {
	tests := []struct {
		name           string
		ctx            map[string]string
		title          string
		descSubstrs    []string
		descNotSubstrs []string
		extraBytes     float64
		extraRotate    float64
		extraCount     float64
		extraSource    []sourceLoss
	}{
		{
			name: "several sources are summarised and broken down",
			ctx: map[string]string{
				contextKeyBytes:          "4200512",
				contextKeyRotations:      "4",
				contextKeySourceCount:    "3",
				contextKeySourcesOmitted: "0",
				contextKeyLastLossAt:     recentTimestamp(),
				contextKeySources:        `[{"source":"nginx","service":"web","bytes":4000000,"rotations":2},{"source":"redis","service":"cache","bytes":200000,"rotations":1},{"source":"kafka","service":"queue","bytes":512,"rotations":1}]`,
			},
			title: "Lost 4.2 MB of logs from 3 sources in the last 24 hours",
			descSubstrs: []string{
				"Logs from 3 sources never reached Datadog",
				"4 log rotations closed a file",
				// Ranked by bytes, with no per-source rotation count: that
				// detail lives in Extra.
				"Most affected: nginx/web 4.0 MB, redis/cache 200 kB, kafka/queue 512 B.",
			},
			descNotSubstrs: []string{
				// The byte total belongs to the Title only.
				"4.2 MB",
				// Recency belongs to the platform's own last-seen, not the prose.
				"Last loss",
			},
			extraBytes:  4200512,
			extraRotate: 4,
			extraCount:  3,
			extraSource: []sourceLoss{
				{Source: "nginx", Service: "web", Bytes: 4000000, Rotations: 2},
				{Source: "redis", Service: "cache", Bytes: 200000, Rotations: 1},
				{Source: "kafka", Service: "queue", Bytes: 512, Rotations: 1},
			},
		},
		{
			name: "a lone source is named instead of counted",
			ctx: map[string]string{
				contextKeyBytes:          "512",
				contextKeyRotations:      "1",
				contextKeySourceCount:    "1",
				contextKeySourcesOmitted: "0",
				contextKeyLastLossAt:     recentTimestamp(),
				contextKeySources:        `[{"source":"app","service":"billing","bytes":512,"rotations":1}]`,
			},
			title: "Lost 512 B of logs from source app in the last 24 hours",
			descSubstrs: []string{
				`Logs from source "app" (service "billing") never reached Datadog`,
				// The one known file is "the file", not "a file".
				"1 log rotation closed the file",
			},
			descNotSubstrs: []string{
				// A named source needs no breakdown repeating it.
				"Most affected",
			},
			extraBytes:  512,
			extraRotate: 1,
			extraCount:  1,
			extraSource: []sourceLoss{{Source: "app", Service: "billing", Bytes: 512, Rotations: 1}},
		},
		{
			name: "omitted sources are counted in the description",
			ctx: map[string]string{
				contextKeyBytes:          "1000",
				contextKeyRotations:      "2",
				contextKeySourceCount:    "192",
				contextKeySourcesOmitted: "190",
				contextKeyLastLossAt:     recentTimestamp(),
				contextKeySources:        `[{"source":"nginx","service":"web","bytes":600,"rotations":1},{"source":"redis","service":"cache","bytes":400,"rotations":1}]`,
			},
			title: "Lost 1.0 kB of logs from 192 sources in the last 24 hours",
			descSubstrs: []string{
				"Most affected: nginx/web 600 B, redis/cache 400 B, and 190 other sources.",
			},
			extraBytes:  1000,
			extraRotate: 2,
			extraCount:  192,
			extraSource: []sourceLoss{
				{Source: "nginx", Service: "web", Bytes: 600, Rotations: 1},
				{Source: "redis", Service: "cache", Bytes: 400, Rotations: 1},
			},
		},
		{
			name:        "empty context falls back to defaults",
			ctx:         map[string]string{},
			title:       "Lost 0 B of logs from 0 sources in the last 24 hours",
			descSubstrs: []string{"Logs from 0 sources never reached Datadog"},
		},
		{
			name:        "nil context must not panic",
			ctx:         nil,
			title:       "Lost 0 B of logs from 0 sources in the last 24 hours",
			descSubstrs: []string{"Logs from 0 sources never reached Datadog"},
		},
		{
			name: "unparseable counters degrade to zero",
			ctx: map[string]string{
				contextKeyBytes:       "not-a-number",
				contextKeyRotations:   "",
				contextKeySourceCount: "3",
				contextKeySources:     `[{"source":"nginx","service":"web","bytes":1,"rotations":1}]`,
			},
			title:       "Lost 0 B of logs from 3 sources in the last 24 hours",
			descSubstrs: []string{"nginx"},
			extraCount:  3,
			extraSource: []sourceLoss{{Source: "nginx", Service: "web", Bytes: 1, Rotations: 1}},
		},
		{
			// A single source we cannot name must not claim to name one; the
			// count form is the honest fallback.
			name: "a malformed breakdown degrades to the counted form",
			ctx: map[string]string{
				contextKeyBytes:       "512",
				contextKeyRotations:   "1",
				contextKeySourceCount: "1",
				contextKeySources:     `{"not":"an array"`,
			},
			title:       "Lost 512 B of logs from 1 source in the last 24 hours",
			descSubstrs: []string{"Logs from 1 source never reached Datadog"},
			extraBytes:  512,
			extraRotate: 1,
			extraCount:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issue, err := MissedBytesIssue{}.BuildIssue(tc.ctx)
			require.NoError(t, err)
			require.NotNil(t, issue)

			assert.Empty(t, issue.GetId(), "Id is set by the runner from the report, not by the template")
			assert.Empty(t, issue.GetDetectedAt(), "DetectedAt is stamped by the store on every report")
			assert.Equal(t, IssueName, issue.GetIssueName())
			assert.Equal(t, IssueType, issue.GetIssueType())
			assert.Equal(t, tc.title, issue.GetTitle())
			for _, substr := range tc.descSubstrs {
				assert.Contains(t, issue.GetDescription(), substr)
			}
			for _, substr := range tc.descNotSubstrs {
				assert.NotContains(t, issue.GetDescription(), substr)
			}
			// Description is printed verbatim by `agent diagnose` behind a
			// fixed "  Diagnosis: " prefix, so it must stay a single block.
			assert.NotContains(t, issue.GetDescription(), "\n", "Description must not contain line breaks")
			assert.NotContains(t, issue.GetDescription(), "  ", "Description must not contain double spaces")
			assert.Equal(t, "logs_pipeline", issue.GetCategory())
			assert.Equal(t, "logs-agent", issue.GetLocation())
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.GetSeverity())
			assert.Equal(t, issueSource, issue.GetSource())
			assert.Equal(t, []string{"logs", "file-tailing", "rotation", "data-loss"}, issue.GetTags())

			require.NotNil(t, issue.GetRemediation())
			assert.Contains(t, issue.GetRemediation().GetSummary(), "Logs Agent Backpressure")
			assert.Empty(t, issue.GetRemediation().GetSteps(), "remediation content for this issue is served backend-side")
			assert.Nil(t, issue.GetRemediation().GetScript())

			require.NotNil(t, issue.GetExtra())
			fields := issue.GetExtra().GetFields()
			for _, key := range []string{
				contextKeyBytes, contextKeyRotations, contextKeySourceCount,
				contextKeySourcesOmitted, contextKeyLastLossAt, contextKeySources,
			} {
				assert.NotNil(t, fields[key], "Extra must carry %q", key)
			}
			assert.Equal(t, tc.extraBytes, fields[contextKeyBytes].GetNumberValue())
			assert.Equal(t, tc.extraRotate, fields[contextKeyRotations].GetNumberValue())
			assert.Equal(t, tc.extraCount, fields[contextKeySourceCount].GetNumberValue())

			// The breakdown must reach Extra as structured values, not as the
			// JSON string it travelled through the context as.
			list := fields[contextKeySources].GetListValue().GetValues()
			require.Len(t, list, len(tc.extraSource))
			for i, want := range tc.extraSource {
				entry := list[i].GetStructValue().GetFields()
				assert.Equal(t, want.Source, entry["source"].GetStringValue())
				assert.Equal(t, want.Service, entry["service"].GetStringValue())
				assert.Equal(t, float64(want.Bytes), entry["bytes"].GetNumberValue())
				assert.Equal(t, float64(want.Rotations), entry["rotations"].GetNumberValue())
			}
		})
	}
}

// Source and service names are free-form (user YAML, pod annotations) and ship
// twice per payload, so they must be bounded before they reach either sink.
func TestSourceNameTruncation(t *testing.T) {
	longASCII := strings.Repeat("a", 200)
	// Multi-byte, to prove the cut lands on a rune boundary.
	longUnicode := strings.Repeat("é", 200)

	issue, err := MissedBytesIssue{}.BuildIssue(map[string]string{
		contextKeyBytes:       "512",
		contextKeyRotations:   "1",
		contextKeySourceCount: "2",
		contextKeySources: `[{"source":"` + longASCII + `","service":"` + longUnicode + `","bytes":512,"rotations":1},` +
			`{"source":"short","service":"svc","bytes":1,"rotations":1}]`,
	})
	require.NoError(t, err)

	wantSource := strings.Repeat("a", maxNameLen-len(nameEllipsis)) + nameEllipsis
	wantService := strings.Repeat("é", maxNameLen-len(nameEllipsis)) + nameEllipsis

	assert.Contains(t, issue.GetDescription(), wantSource+"/"+wantService)
	assert.NotContains(t, issue.GetDescription(), longASCII, "the untruncated name must never reach the Description")

	entry := issue.GetExtra().GetFields()[contextKeySources].GetListValue().GetValues()[0].GetStructValue().GetFields()
	gotSource := entry["source"].GetStringValue()
	gotService := entry["service"].GetStringValue()

	assert.Equal(t, wantSource, gotSource, "Extra must carry the same bounded name as the Description")
	assert.Equal(t, wantService, gotService)
	assert.Equal(t, maxNameLen, utf8.RuneCountInString(gotSource))
	assert.Equal(t, maxNameLen, utf8.RuneCountInString(gotService))
	assert.True(t, utf8.ValidString(gotService), "a multi-byte name must not be split into invalid UTF-8")

	// A name already within the bound is passed through untouched.
	short := issue.GetExtra().GetFields()[contextKeySources].GetListValue().GetValues()[1].GetStructValue().GetFields()
	assert.Equal(t, "short", short["source"].GetStringValue())
}

// IssueType must stay IssueName lowercased with spaces replaced by underscores;
// the agent never derives it, so a rename can silently desynchronise the two.
func TestIssueTypeMatchesIssueName(t *testing.T) {
	assert.Equal(t, "log_data_lost_after_rotation", IssueType)
	assert.Equal(t, "Log Data Lost After Rotation", IssueName)
}
