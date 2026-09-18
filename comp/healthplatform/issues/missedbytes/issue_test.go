// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
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
			name:           "malformed backpressure only drops enrichment",
			ctx:            map[string]string{contextKeyBackpressure: "{not json", contextKeyBytes: "512"},
			title:          "Lost 512 B of logs from 0 sources in the last 24 hours",
			descNotSubstrs: []string{"pipeline"},
			extraBytes:     512,
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
			assert.Contains(t, steps[1].GetText(), "`logs_config.close_timeout`")
			assert.Contains(t, steps[1].GetText(), "from its current value (default: 60 seconds)")
			assert.NotContains(t, steps[4].GetText(), "auto_multi_line_detection",
				"multiline aggregation runs before the measured processor, so disabling it can raise its load")
			for i, step := range steps {
				assert.Equal(t, int32(i+1), step.GetOrder(), "Order must be contiguous and 1-indexed")
			}

			require.NotNil(t, issue.GetExtra())
			fields := issue.GetExtra().GetFields()
			assert.NotContains(t, fields, contextKeyBackpressure)
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

func backpressureContext(t *testing.T, bp backpressureWire) string {
	t.Helper()
	encoded, err := json.Marshal(bp)
	require.NoError(t, err)
	return string(encoded)
}

func saturatedComponent(name string, sat30m int64) *logsmetrics.ComponentBackpressure {
	return &logsmetrics.ComponentBackpressure{
		Component:           name,
		Instance:            "0",
		AvgRatio:            0.98,
		Saturated30mSeconds: sat30m,
		CurrentlySaturated:  true,
	}
}

// Step 1 names what to fix, so every claim it makes has to be backed by what was measured.
func TestBuildIssue_FirstRemediationStep(t *testing.T) {
	tests := []struct {
		name        string
		component   string
		blamed      int64
		rotations   int64
		bp          *backpressureWire
		wantStep    []string
		notWantStep []string
		wantDesc    []string
		notWantDesc []string
	}{
		{
			name:      "loss-time attribution outranks the check-time snapshot",
			component: "strategy", blamed: 7, rotations: 9,
			bp:          &backpressureWire{State: logsmetrics.BackpressureSaturated, Bottleneck: saturatedComponent("processor", 1740)},
			wantStep:    []string{"`strategy`", "7 of 9 rotations"},
			notWantStep: []string{"`processor`"},
			wantDesc:    []string{"The strategy stage of the logs pipeline was saturated during 7 of these rotations."},
			notWantDesc: []string{"processor stage"},
		},
		{
			name:      "all losses attributed to one stage",
			component: "worker", blamed: 9, rotations: 9,
			wantStep: []string{"`worker` component was saturated", "sudo datadog-agent status"},
		},
		{
			name:      "healthy at loss time points at close_timeout",
			component: logsmetrics.NoBottleneck, blamed: 4, rotations: 4,
			bp:       &backpressureWire{State: logsmetrics.BackpressureHealthy},
			wantStep: []string{"No monitored blocking stage was saturated", "`logs_config.close_timeout`"},
			wantDesc: []string{"no monitored blocking stage of the logs pipeline was saturated"},
		},
		{
			name:      "saturation after the loss is still named",
			component: logsmetrics.NoBottleneck, blamed: 4, rotations: 4,
			bp:       &backpressureWire{State: logsmetrics.BackpressureSaturated, Bottleneck: saturatedComponent("strategy", 60)},
			wantStep: []string{"No monitored blocking stage was saturated", "`logs_config.close_timeout`", "`strategy` is saturated now"},
		},
		{
			name:      "partial healthy attribution stays qualified",
			component: logsmetrics.NoBottleneck, blamed: 6, rotations: 11,
			wantStep:    []string{"6 of 11 rotations", "`logs_config.close_timeout`"},
			notWantStep: []string{"No monitored blocking stage was saturated"},
		},
		{
			name:      "unmeasured attribution falls back to the live snapshot",
			rotations: 4,
			bp:        &backpressureWire{State: logsmetrics.BackpressureSaturated, Bottleneck: saturatedComponent("destination_reliable_0", 1740)},
			wantStep:  []string{"was not measured", "`destination_reliable_0` is saturated now"},
			wantDesc:  []string{"destination_reliable_0 stage of the logs pipeline is saturated"},
		},
		{
			name:      "a recovered snapshot is described as history",
			rotations: 4,
			bp: &backpressureWire{
				State:      logsmetrics.BackpressureWarning,
				Bottleneck: &logsmetrics.ComponentBackpressure{Component: "strategy", Saturated30mSeconds: 120},
			},
			wantStep:    []string{"`strategy`", "was saturated earlier in the last 30 minutes"},
			notWantStep: []string{"`strategy` is saturated now"},
		},
		{
			name:        "unknown loss and snapshot fall back to status",
			rotations:   4,
			wantStep:    []string{"Run `sudo datadog-agent status`"},
			notWantDesc: []string{"pipeline"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := map[string]string{
				contextKeyRotations:               strconv.FormatInt(tc.rotations, 10),
				contextKeyLossBottleneck:          tc.component,
				contextKeyLossBottleneckRotations: strconv.FormatInt(tc.blamed, 10),
			}
			if tc.bp != nil {
				ctx[contextKeyBackpressure] = backpressureContext(t, *tc.bp)
			}
			issue, err := MissedBytesIssue{}.BuildIssue(ctx)
			require.NoError(t, err)
			assert.NotContains(t, issue.GetDescription(), "\n")
			step1 := issue.GetRemediation().GetSteps()[0].GetText()
			for _, substr := range tc.wantStep {
				assert.Contains(t, step1, substr)
			}
			for _, substr := range tc.notWantStep {
				assert.NotContains(t, step1, substr)
			}
			for _, substr := range tc.wantDesc {
				assert.Contains(t, issue.GetDescription(), substr)
			}
			for _, substr := range tc.notWantDesc {
				assert.NotContains(t, issue.GetDescription(), substr)
			}
		})
	}
}
