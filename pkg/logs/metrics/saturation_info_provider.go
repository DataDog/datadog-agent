// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"strings"
	"time"
)

// pipelineStages defines the three bottleneck stages in pipeline order,
// from first-to-process to last-to-send. Backpressure propagates in reverse.
var pipelineStages = []struct {
	key          string // TlmName constant
	label        string // human-readable label for status display
	utilRatioHint string // hint shown when fill-based tracking is not valid for this stage
}{
	{ProcessorTlmName, "Processor (rules)", ""},
	{StrategyTlmName, "Compression", "monitor: logs_component_utilization.ratio{name:strategy}"},
	{SenderTlmName, "Transport", "monitor: logs_component_utilization.ratio{name:worker}"},
}

// profileRationale describes the effect of each recommended profile in one line.
var profileRationale = map[string]string{
	"max_throughput": "disables compression, freeing CPU at the cost of ~2-4x more network bandwidth",
	"wan_optimized":  "increases concurrent HTTP sends to hide WAN / intake latency",
	"performance":    "uses all CPU cores as parallel pipelines to speed up rule evaluation",
}

// SaturationInfoProvider formats pipeline health data for the agent status page.
// It implements the InfoKey/Info pattern used by the logs agent status system.
type SaturationInfoProvider struct{}

// InfoKey returns the section heading used by the status page.
func (s SaturationInfoProvider) InfoKey() string { return "Pipeline Health" }

// Info returns pre-formatted status lines for the pipeline health section.
//
// Output layout (each string is one printed line; template prepends 2 spaces):
//
//	Stage              Now  [-- Fill --]   5m    30m     2h
//	----------------------------------------------------------------
//	Processor (rules)   8%  [=         ]   12%    12%    45%
//	Compression        91%  [=========!]  100%   100%   100%  << saturated
//	Transport           3%  [          ]    5%     5%     5%
//	----------------------------------------------------------------
//	Backpressure flows upstream:  Transport -> Compression -> Processor
//
//	!! Compression is saturated (5m peak: 100%)
//	   Suggested profile: max_throughput
//	   Effect: disables compression, freeing CPU ...
//
//	Recent saturation events:
//	  14:23:15  Compression  peak 91%  3m 12s  -> max_throughput
func (s SaturationInfoProvider) Info() []string {
	sum := GlobalSaturationHistory.Summary()
	var out []string

	const (
		labelW = 22 // stage label column width
		barW   = 10 // fill-bar inner width (chars between [ and ])
		divLen = 68 // separator line length
	)

	// Column header
	// %5s for "Now" aligns with the %4.0f%% data column (4-wide number + "%" = 5 chars).
	out = append(out,
		fmt.Sprintf("%-*s  %5s  %-*s  %5s  %6s  %6s",
			labelW, "Stage", "Now", barW+2, "[-- Fill --]", "5m", "30m", "2h"))
	out = append(out, strings.Repeat("-", divLen))

	// One row per stage in pipeline order.
	// Processor uses channel fill %. Compression and Transport use CPU utilization
	// ratio (logs_component_utilization.ratio) which is not yet fed into this display —
	// those rows show placeholder data only.
	anySaturated := false
	for _, st := range pipelineStages {
		if st.key == StrategyTlmName || st.key == SenderTlmName {
			// Fill-based tracking is not valid for these stages (see pipeline.go comment).
			// Direct the user to the utilization ratio metric instead.
			note := st.utilRatioHint
			out = append(out, fmt.Sprintf("%-*s  %5s  %-*s  %5s  %6s  %6s  %s",
				labelW, st.label, "--", barW+2, "(channel fill n/a)", "--", "--", "--", note))
			continue
		}

		curr := sum.CurrentFill[st.key]
		m5 := sum.MaxFill5m[st.key]
		m30 := sum.MaxFill30m[st.key]
		m2h := sum.MaxFill2h[st.key]

		bar := fillBar(curr, barW)

		saturatedRecently := m5 >= saturationHighThreshold
		if saturatedRecently {
			anySaturated = true
		}

		if curr >= saturationHighThreshold {
			bar = bar[:len(bar)-2] + "!]"
		}

		marker := ""
		if saturatedRecently {
			marker = "  << saturated"
		}

		out = append(out, fmt.Sprintf("%-*s  %4.0f%%  %s  %5.0f%%  %6.0f%%  %6.0f%%%s",
			labelW, st.label, curr*100, bar, m5*100, m30*100, m2h*100, marker))
	}

	out = append(out, strings.Repeat("-", divLen))
	out = append(out, "Backpressure flows upstream:  Transport -> Compression -> Processor")

	// Suggestion block.
	if sum.SuggestedProfile != "" {
		rationale := profileRationale[sum.SuggestedProfile]
		out = append(out, "")
		out = append(out, "!! Bottleneck detected. Suggested profile: "+sum.SuggestedProfile)
		out = append(out, "   Set:    logs_config.logs_agent_profile: "+sum.SuggestedProfile)
		if rationale != "" {
			out = append(out, "   Effect: "+rationale)
		}
	} else if !anySaturated {
		out = append(out, "   No bottleneck detected.")
	}

	// Recent saturation events (newest first, max 5 shown).
	if len(sum.RecentEvents) > 0 {
		out = append(out, "")
		out = append(out, "Recent saturation events:")
		for i, e := range sum.RecentEvents {
			if i >= 5 {
				break
			}
			durationStr := "ongoing"
			if !e.Ongoing() {
				durationStr = formatDuration(e.Duration())
			}
			label := stageLabelFor(e.Stage)
			out = append(out, fmt.Sprintf("  %s  %-18s  peak %3.0f%%  %8s  -> %s",
				e.StartTime.Local().Format("15:04:05"),
				label, e.PeakFill*100, durationStr, e.Suggestion))
		}
	}

	return out
}

// fillBar returns an ASCII fill bar of the form "[===       ]".
// fill is in [0.0, 1.0]; width is the number of inner characters.
func fillBar(fill float64, width int) string {
	filled := int(fill * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}

// formatDuration returns a compact duration string: "3m 12s", "45s", "1h 02m".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm %02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// stageLabelFor returns the human-readable label for a stage TlmName constant.
func stageLabelFor(stage string) string {
	for _, st := range pipelineStages {
		if st.key == stage {
			return st.label
		}
	}
	return stage
}
