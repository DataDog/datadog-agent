// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/dustin/go-humanize"
	"google.golang.org/protobuf/types/known/structpb"

	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

const (
	contextKeyBytes     = "bytes_missed_24h"
	contextKeyRotations = "rotation_count_24h"

	// Distinct source names, not tuples: this is the count that reaches the prose.
	contextKeySourceCount = "source_count"

	contextKeyPairsOmitted = "source_service_pairs_omitted"

	contextKeyLastLossAt = "last_loss_at"

	// Context is map[string]string, so the breakdown travels as encoded sourceLoss.
	contextKeySources = "sources"

	// Encoded backpressureWire. Absent when no pipeline monitor answered.
	contextKeyBackpressure = "backpressure"

	// The reporting component, not the scheduler's checkSource label.
	issueSource = "logs"

	// The failing subsystem, not where the fix lands.
	issueCategory = "logs_pipeline"

	// Bounds one free-form name, which ships in both Description and Extra.
	maxNameLen = 64

	// ASCII on purpose: `agent diagnose` prints Description on non-UTF-8 consoles.
	nameEllipsis = "..."

	unknownValue = "unknown"

	// A loss recorded while the pipeline was keeping up: the fix is close_timeout.
	noBottleneckLabel = logsmetrics.NoBottleneck
)

// sourceLoss is one tuple's share of the host total. Its JSON tags are the wire
// contract with the check.
type sourceLoss struct {
	Source    string `json:"source"`
	Service   string `json:"service"`
	Bytes     int64  `json:"bytes"`
	Rotations int64  `json:"rotations"`
	// Bottleneck is the stage saturated when most of this tuple's rotations lost data, sampled
	// at loss time rather than at check time. Empty when no monitor answered.
	Bottleneck          string `json:"bottleneck,omitempty"`
	BottleneckRotations int64  `json:"bottleneck_rotations,omitempty"`
}

// backpressureWire is the check-time snapshot as it crosses Context, and the shape the
// backend receives in Extra.
type backpressureWire struct {
	State             string                              `json:"state"`
	Bottleneck        *logsmetrics.ComponentBackpressure  `json:"bottleneck"`
	Components        []logsmetrics.ComponentBackpressure `json:"components"`
	ComponentsOmitted int                                 `json:"components_omitted"`
}

// MissedBytesIssue is the template for "log-data-lost-after-rotation" issues.
type MissedBytesIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (MissedBytesIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	// Read, not summed: the breakdown holds only the largest maxBreakdownSources.
	bytesLost, _ := strconv.ParseInt(ctx[contextKeyBytes], 10, 64)
	rotations, _ := strconv.ParseInt(ctx[contextKeyRotations], 10, 64)
	sourceCount, _ := strconv.ParseInt(ctx[contextKeySourceCount], 10, 64)
	omitted, _ := strconv.ParseInt(ctx[contextKeyPairsOmitted], 10, 64)

	lastLossAt := ctx[contextKeyLastLossAt]
	if lastLossAt == "" {
		lastLossAt = unknownValue
	}

	sources := decodeSources(ctx[contextKeySources])

	// Nameable only when one source and one service are the entire loss.
	named := sourceCount == 1 && len(sources) == 1 && omitted == 0

	scope := fmt.Sprintf("%d %s", sourceCount, pluralize(sourceCount, "source"))
	subject := "Logs from " + scope
	file := "a file"
	breakdown := describeSources(sources, omitted)
	if named {
		scope = "source " + sources[0].Source
		subject = fmt.Sprintf("Logs from source %q (service %q)", sources[0].Source, sources[0].Service)
		file = "the file"
		breakdown = ""
	}

	bp := decodeBackpressure(ctx[contextKeyBackpressure])

	sentences := []string{
		fmt.Sprintf("%s never reached Datadog: %d log %s closed %s before the Agent finished reading it.",
			subject, rotations, pluralize(rotations, "rotation"), file),
	}
	if breakdown != "" {
		sentences = append(sentences, breakdown)
	}
	if cause := describeCause(bp, sources); cause != "" {
		sentences = append(sentences, cause)
	}

	extraFields := map[string]any{
		contextKeyBytes:        bytesLost,
		contextKeyRotations:    rotations,
		contextKeySourceCount:  sourceCount,
		contextKeyPairsOmitted: omitted,
		contextKeyLastLossAt:   lastLossAt,
		contextKeySources:      sourcesAsExtra(sources),
	}
	if bp != nil {
		extraFields[contextKeyBackpressure] = backpressureAsExtra(bp)
	}

	extra, err := structpb.NewStruct(extraFields)
	if err != nil {
		return nil, fmt.Errorf("missedbytes: build issue extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName: IssueName,
		IssueType: IssueType,
		Title: fmt.Sprintf("Lost %s of logs from %s in the last 24 hours",
			humanizeBytes(bytesLost), scope),
		// One block, no newlines or markdown: `agent diagnose` prints this verbatim
		// behind a fixed prefix (comp/core/diagnose/format/format.go).
		Description: strings.Join(sentences, " "),
		Category:    issueCategory,
		Location:    "logs-agent",
		// HIGH lands in the Fleet UI's "Agent Health: Broken" bucket.
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:   issueSource,
		Extra:    extra,
		Tags:     []string{"logs", "file-tailing", "rotation", "data-loss"},
		// Ordered cheapest to most invasive, per the component's remediation rules.
		// Steps 3-5 branch on the saturated component step 1 identifies.
		Remediation: &healthplatform.Remediation{
			// Plain text: Summary is not rendered as markdown.
			Summary: "Give the Agent more time to finish reading rotated files, and relieve any saturation in the logs pipeline.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: firstRemediationStep(bp, sources)},
				{Order: 2, Text: "Raise `logs_config.close_timeout` (DD_LOGS_CONFIG_CLOSE_TIMEOUT) above its 60 second default to give the tailer longer to finish a rotated file."},
				{Order: 3, Text: "If a `destination_reliable_N` or `worker` row is saturated, check the Agent log for failed or retried submissions and resolve any proxy, DNS, authentication, or connectivity errors."},
				{Order: 4, Text: "If the `strategy` row is saturated, set `logs_config.use_compression` to false or raise `logs_config.pipelines`."},
				{Order: 5, Text: "If the `processor` row is saturated, scope global `logs_config.processing_rules` to the affected source, or set `logs_config.auto_multi_line_detection` to false."},
				{Order: 6, Text: "If the Agent still cannot keep up, drop unneeded logs with an `exclude_at_match` processing rule, or reduce the volume written to the file between rotations."},
				{Order: 7, Text: "Re-run `sudo datadog-agent status` under representative log volume and confirm no new rotation warnings appear in the Agent log."},
			},
		},
	}, nil
}

// decodeSources tolerates a malformed value: totals stay usable, so a failure only
// degrades the wording. Every consumer reads names through here, so it sanitizes.
func decodeSources(encoded string) []sourceLoss {
	if encoded == "" {
		return nil
	}
	var sources []sourceLoss
	if err := json.Unmarshal([]byte(encoded), &sources); err != nil {
		return nil
	}
	for i := range sources {
		sources[i].Source = sanitizeName(sources[i].Source)
		sources[i].Service = sanitizeName(sources[i].Service)
	}
	return sources
}

// sanitizeName bounds a name and drops control characters: names come from user
// YAML and reach Title and Description unescaped. Cuts on a rune boundary.
func sanitizeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	switch {
	case name == "":
		return unknownValue
	case utf8.RuneCountInString(name) <= maxNameLen:
		return name
	}
	return string([]rune(name)[:maxNameLen-utf8.RuneCountInString(nameEllipsis)]) + nameEllipsis
}

// describeSources renders the breakdown ranked by bytes, plus a count of what the
// cap left out. Rotation counts are left to Extra.
func describeSources(sources []sourceLoss, omitted int64) string {
	if len(sources) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sources)+1)
	for _, s := range sources {
		parts = append(parts, fmt.Sprintf("%s/%s %s", s.Source, s.Service, humanizeBytes(s.Bytes)))
	}
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("and %d other %s", omitted, pluralize(omitted, "source/service pair")))
	}
	return "Most affected: " + strings.Join(parts, ", ") + "."
}

// sourcesAsExtra reshapes the breakdown so the backend receives objects, not a string.
func sourcesAsExtra(sources []sourceLoss) []any {
	values := make([]any, 0, len(sources))
	for _, s := range sources {
		value := map[string]any{
			"source":    s.Source,
			"service":   s.Service,
			"bytes":     s.Bytes,
			"rotations": s.Rotations,
		}
		// Absent means "not attributed", which is not noBottleneckLabel's claim.
		if s.Bottleneck != "" {
			value["bottleneck"] = s.Bottleneck
			value["bottleneck_rotations"] = s.BottleneckRotations
		}
		values = append(values, value)
	}
	return values
}

// decodeBackpressure tolerates a malformed value the way decodeSources does: a failure here
// only costs the enrichment.
func decodeBackpressure(encoded string) *backpressureWire {
	if encoded == "" {
		return nil
	}
	var bp backpressureWire
	if err := json.Unmarshal([]byte(encoded), &bp); err != nil {
		return nil
	}
	if bp.State == "" {
		return nil
	}
	bp.Bottleneck = sanitizeComponent(bp.Bottleneck)
	for i := range bp.Components {
		if c := sanitizeComponent(&bp.Components[i]); c != nil {
			bp.Components[i] = *c
		}
	}
	return &bp
}

// sanitizeComponent bounds the names. Agent-generated today, but they reach Description
// unescaped, so they take the same gate as the user-supplied source names.
func sanitizeComponent(c *logsmetrics.ComponentBackpressure) *logsmetrics.ComponentBackpressure {
	if c == nil {
		return nil
	}
	c.Component = sanitizeName(c.Component)
	c.Instance = sanitizeName(c.Instance)
	return c
}

// backpressureAsExtra reshapes the snapshot so the backend receives objects, not a string.
func backpressureAsExtra(bp *backpressureWire) map[string]any {
	components := make([]any, 0, len(bp.Components))
	for _, c := range bp.Components {
		components = append(components, componentAsExtra(c))
	}
	extra := map[string]any{
		"state":              bp.State,
		"components":         components,
		"components_omitted": bp.ComponentsOmitted,
	}
	if bp.Bottleneck != nil {
		extra["bottleneck"] = componentAsExtra(*bp.Bottleneck)
	}
	return extra
}

func componentAsExtra(c logsmetrics.ComponentBackpressure) map[string]any {
	return map[string]any{
		"component":           c.Component,
		"instance":            c.Instance,
		"avg_ratio":           c.AvgRatio,
		"max_5m":              c.Max5m,
		"max_30m":             c.Max30m,
		"max_2h":              c.Max2h,
		"max_5h":              c.Max5h,
		"max_10h":             c.Max10h,
		"saturated_1m_s":      c.Saturated1mSeconds,
		"saturated_30m_s":     c.Saturated30mSeconds,
		"currently_saturated": c.CurrentlySaturated,
	}
}

// describeCause names the stage responsible, preferring the one sampled at loss time: the
// loss window is 24h and the check runs every 15m, so the two can disagree.
func describeCause(bp *backpressureWire, sources []sourceLoss) string {
	if atLoss, rotations := lossTimeBottleneck(sources); atLoss != "" {
		if atLoss == noBottleneckLabel {
			return fmt.Sprintf("The logs pipeline was keeping up during %d of these %s, so the Agent ran out of time rather than throughput.",
				rotations, pluralize(rotations, "rotation"))
		}
		return fmt.Sprintf("The %s stage of the logs pipeline was saturated during %d of these %s.",
			atLoss, rotations, pluralize(rotations, "rotation"))
	}

	if bp == nil || bp.Bottleneck == nil {
		return ""
	}
	return fmt.Sprintf("The %s stage of the logs pipeline is %s, saturated for %s of the last 30 minutes.",
		bp.Bottleneck.Component, strings.ToLower(bp.State), fmtSeconds(bp.Bottleneck.Saturated30mSeconds))
}

// lossTimeBottleneck picks the stage blamed for the most rotations across the breakdown.
func lossTimeBottleneck(sources []sourceLoss) (string, int64) {
	totals := make(map[string]int64, len(sources))
	for _, s := range sources {
		if s.Bottleneck != "" {
			totals[s.Bottleneck] += s.BottleneckRotations
		}
	}
	var name string
	var max int64
	for component, count := range totals {
		if count > max || (count == max && component < name) {
			name, max = component, count
		}
	}
	return name, max
}

// firstRemediationStep names the stage to fix so the reader can skip to the matching branch.
// Loss time wins over check time: it is what lost the data, not what is saturated right now.
func firstRemediationStep(bp *backpressureWire, sources []sourceLoss) string {
	component, _ := lossTimeBottleneck(sources)
	if component == noBottleneckLabel {
		component = ""
	}
	if component == "" && bp != nil && bp.Bottleneck != nil {
		component = bp.Bottleneck.Component
	}
	if component == "" {
		return "Run `sudo datadog-agent status` and note any saturated component in the Logs Agent Backpressure section."
	}
	return fmt.Sprintf("The Agent measured the `%s` component as the saturated one when the data was lost; skip to the step below that matches it. Run `sudo datadog-agent status` to check whether it is still saturated.",
		component)
}

func fmtSeconds(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func humanizeBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	return humanize.Bytes(uint64(n))
}

func pluralize(n int64, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
