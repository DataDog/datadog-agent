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

// last_loss_at reaches Extra but not the prose: the platform tracks last-seen.
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
				contextKeyBytes:        "4200512",
				contextKeyRotations:    "4",
				contextKeySourceCount:  "3",
				contextKeyPairsOmitted: "0",
				contextKeyLastLossAt:   recentTimestamp(),
				contextKeySources:      `[{"source":"nginx","service":"web","bytes":4000000,"rotations":2},{"source":"redis","service":"cache","bytes":200000,"rotations":1},{"source":"kafka","service":"queue","bytes":512,"rotations":1}]`,
			},
			title: "Lost 4.2 MB of logs from 3 sources in the last 24 hours",
			descSubstrs: []string{
				"Logs from 3 sources never reached Datadog",
				"4 log rotations closed a file",
				"Most affected: nginx/web 4.0 MB, redis/cache 200 kB, kafka/queue 512 B.",
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
				contextKeyBytes:        "512",
				contextKeyRotations:    "1",
				contextKeySourceCount:  "1",
				contextKeyPairsOmitted: "0",
				contextKeyLastLossAt:   recentTimestamp(),
				contextKeySources:      `[{"source":"app","service":"billing","bytes":512,"rotations":1}]`,
			},
			title: "Lost 512 B of logs from source app in the last 24 hours",
			descSubstrs: []string{
				`Logs from source "app" (service "billing") never reached Datadog`,
				"1 log rotation closed the file",
			},
			descNotSubstrs: []string{"Most affected"},
			extraBytes:     512,
			extraRotate:    1,
			extraCount:     1,
			extraSource:    []sourceLoss{{Source: "app", Service: "billing", Bytes: 512, Rotations: 1}},
		},
		{
			name: "a source with two services stays counted, not named",
			ctx: map[string]string{
				contextKeyBytes:        "4200000",
				contextKeyRotations:    "2",
				contextKeySourceCount:  "1",
				contextKeyPairsOmitted: "0",
				contextKeyLastLossAt:   recentTimestamp(),
				contextKeySources:      `[{"source":"nginx","service":"web","bytes":4000000,"rotations":1},{"source":"nginx","service":"api","bytes":200000,"rotations":1}]`,
			},
			title: "Lost 4.2 MB of logs from 1 source in the last 24 hours",
			descSubstrs: []string{
				"Logs from 1 source never reached Datadog",
				"Most affected: nginx/web 4.0 MB, nginx/api 200 kB.",
			},
			descNotSubstrs: []string{"(service "},
			extraBytes:     4200000,
			extraRotate:    2,
			extraCount:     1,
			extraSource: []sourceLoss{
				{Source: "nginx", Service: "web", Bytes: 4000000, Rotations: 1},
				{Source: "nginx", Service: "api", Bytes: 200000, Rotations: 1},
			},
		},
		{
			// Hostile name here too: the breakdown interpolates with %s, unlike the
			// named case's %q, and %q would escape a newline rather than strip it.
			name: "omitted tuples are counted in the description",
			ctx: map[string]string{
				contextKeyBytes:        "1000",
				contextKeyRotations:    "2",
				contextKeySourceCount:  "192",
				contextKeyPairsOmitted: "190",
				contextKeyLastLossAt:   recentTimestamp(),
				contextKeySources:      `[{"source":"nginx\nDiagnosis: fake","service":"web","bytes":600,"rotations":1},{"source":"redis","service":"cache","bytes":400,"rotations":1}]`,
			},
			title: "Lost 1.0 kB of logs from 192 sources in the last 24 hours",
			descSubstrs: []string{
				"Most affected: nginxDiagnosis: fake/web 600 B, redis/cache 400 B, and 190 other source/service pairs.",
			},
			extraBytes:  1000,
			extraRotate: 2,
			extraCount:  192,
			extraSource: []sourceLoss{
				{Source: "nginxDiagnosis: fake", Service: "web", Bytes: 600, Rotations: 1},
				{Source: "redis", Service: "cache", Bytes: 400, Rotations: 1},
			},
		},
		{
			name:        "nil context falls back to defaults",
			ctx:         nil,
			title:       "Lost 0 B of logs from 0 sources in the last 24 hours",
			descSubstrs: []string{"Logs from 0 sources never reached Datadog"},
		},
		{
			// A source we cannot name must not claim to name one.
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
		{
			// Names come from user YAML and reach the Title unescaped.
			name: "control characters are stripped from a named source",
			ctx: map[string]string{
				contextKeyBytes:       "512",
				contextKeyRotations:   "1",
				contextKeySourceCount: "1",
				contextKeySources:     `[{"source":"nginx\nDiagnosis: fake","service":"web","bytes":512,"rotations":1}]`,
			},
			title:       "Lost 512 B of logs from source nginxDiagnosis: fake in the last 24 hours",
			descSubstrs: []string{`Logs from source "nginxDiagnosis: fake" (service "web")`},
			extraBytes:  512,
			extraRotate: 1,
			extraCount:  1,
			extraSource: []sourceLoss{{Source: "nginxDiagnosis: fake", Service: "web", Bytes: 512, Rotations: 1}},
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
			// `agent diagnose` prints this verbatim behind a fixed prefix.
			assert.NotContains(t, issue.GetDescription(), "\n", "Description must not contain line breaks")
			assert.NotContains(t, issue.GetDescription(), "  ", "Description must not contain double spaces")
			assert.Equal(t, "logs_pipeline", issue.GetCategory())
			assert.Equal(t, "logs-agent", issue.GetLocation())
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.GetSeverity())
			assert.Equal(t, issueSource, issue.GetSource())
			assert.Equal(t, []string{"logs", "file-tailing", "rotation", "data-loss"}, issue.GetTags())

			require.NotNil(t, issue.GetRemediation())
			assert.NotEmpty(t, issue.GetRemediation().GetSummary())
			assert.NotContains(t, issue.GetRemediation().GetSummary(), "`", "Summary is not rendered as markdown")
			assert.Nil(t, issue.GetRemediation().GetScript())

			steps := issue.GetRemediation().GetSteps()
			require.NotEmpty(t, steps)
			assert.Contains(t, steps[0].GetText(), "agent status", "step 1 is the fastest diagnostic command")
			for i, step := range steps {
				assert.Equal(t, int32(i+1), step.GetOrder(), "Order must be contiguous and 1-indexed")
			}

			require.NotNil(t, issue.GetExtra())
			fields := issue.GetExtra().GetFields()
			for _, key := range []string{
				contextKeyBytes, contextKeyRotations, contextKeySourceCount,
				contextKeyPairsOmitted, contextKeyLastLossAt, contextKeySources,
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

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short names pass through", "nginx", "nginx"},
		{"long ASCII is truncated", strings.Repeat("a", 200),
			strings.Repeat("a", maxNameLen-len(nameEllipsis)) + nameEllipsis},
		{"multi-byte cuts on a rune boundary", strings.Repeat("é", 200),
			strings.Repeat("é", maxNameLen-len(nameEllipsis)) + nameEllipsis},
		{"control characters are dropped", "ng\ninx\tweb\r", "nginxweb"},
		{"a name of only control characters degrades", "\n\t\r", unknownValue},
		{"an empty name degrades", "", unknownValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeName(tc.in)
			assert.Equal(t, tc.want, got)
			assert.LessOrEqual(t, utf8.RuneCountInString(got), maxNameLen)
			assert.True(t, utf8.ValidString(got), "a multi-byte name must not be split into invalid UTF-8")
		})
	}
}
