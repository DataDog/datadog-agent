// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	pkgstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

const (
	// logPatternRateWindowSec is the current-rate averaging window (seconds).
	logPatternRateWindowSec = 60
	// logPatternPrevRateWindowSec is the baseline window length: [-6min, -1min].
	logPatternPrevRateWindowSec = 5 * logPatternRateWindowSec

	// Thresholds for surfacing a rate-change annotation.
	// Both must be exceeded: relative change guards against noise at low rates,
	// absolute change guards against large relative swings near zero.
	logRateChangeRelThreshold = 0.3
	logRateChangeAbsThreshold = 2

	// Canonical namespace names matching the extractor implementations in observer/impl.
	logPatternExtractorNamespace = "log_pattern_extractor"
	logMetricsExtractorNamespace = "log_metrics_extractor"

	// changeEventMessageMaxLen caps the rendered change-event message below the
	// v2 Events API 4 KiB limit (4096 bytes).
	changeEventMessageMaxLen = 4000
	// impactedResourcesMaxItems caps the number of service entries we attach
	// to a change event. Matches the v2 API server-side limit.
	impactedResourcesMaxItems = 100
	// changedResourceNameMaxLen caps the changed_resource.name length.
	changedResourceNameMaxLen = 128

	// changeEventIntegrationID identifies the publishing integration. The
	// `edge-intelligence` integration is registered upstream in
	// integrations-internal-core#3240 and the event-management intake derives
	// source_type from this value. The v2 Events API schema does not accept
	// source_type_id as a field at $.data.attributes — integration_id alone
	// is sufficient for routing.
	changeEventIntegrationID = "edge-intelligence"

	// changedResourceType is the resource classification carried in
	// data.attributes.attributes.changed_resource.type.
	changedResourceType = "anomaly"

	// Change-event sub-categories carried in change_metadata.sub_category.
	// The set is intentionally small and extensible.
	subCategorySpike      = "spike"
	subCategoryDrop       = "drop"
	subCategoryNewPattern = "new_pattern"
)

// splitTagKeyOrder is the canonical ordered list of tag dimensions used to split
// log series, matching log_tagged_pattern_clusterer in observer/impl.
var splitTagKeyOrder = []string{"source", "service", "env", "host"}

// eventSender formats and dispatches one Datadog change event per correlation
// through the event-platform forwarder (event-management intake). Events are
// sent as raw JSON matching the v2 Events API shape so we don't have to depend
// on the heavyweight datadog-api-client-go module.
type eventSender struct {
	forwarder eventplatform.Forwarder
	logger    log.Component
	storage   observerdef.StorageReader
	hostname  hostname.Component
}

// newEventSender creates an eventSender backed by the given forwarder.
// storage is used to compute windowed log rates for display in event messages;
// it may be nil and will be set later via EventReporter.SetStorage.
func newEventSender(forwarder eventplatform.Forwarder, logger log.Component, storage observerdef.StorageReader, hn hostname.Component) (*eventSender, error) {
	if forwarder == nil {
		return nil, errors.New("event-platform forwarder is not available")
	}
	return &eventSender{
		forwarder: forwarder,
		logger:    logger,
		storage:   storage,
		hostname:  hn,
	}, nil
}

// logPatternRate returns the avg log/s over [T-60s, T] when the anomaly points
// at a storage-backed series.
func logPatternRate(a observerdef.Anomaly, storage observerdef.StorageReader) (rate float64, ok bool) {
	if a.SourceRef == nil || storage == nil {
		return 0, false
	}
	total := storage.SumRange(a.SourceRef.Ref, a.Timestamp-logPatternRateWindowSec, a.Timestamp, observerdef.AggregateCount)
	return total / logPatternRateWindowSec, true
}

// logPatternPrevRate returns the avg log/s over the baseline window [-6min, -1min].
// No DebugInfo fallback: CurrentValue is always present-tense.
func logPatternPrevRate(a observerdef.Anomaly, storage observerdef.StorageReader) (rate float64, ok bool) {
	if a.SourceRef != nil && storage != nil {
		start := a.Timestamp - logPatternPrevRateWindowSec - logPatternRateWindowSec
		total := storage.SumRange(a.SourceRef.Ref, start, a.Timestamp-logPatternRateWindowSec, observerdef.AggregateCount)
		if total == 0 {
			return 0, false
		}
		return total / logPatternPrevRateWindowSec, true
	}
	return 0, false
}

// isSignificantRateChange reports whether the rate shift from prev to curr
// exceeds both the absolute and relative thresholds.
func isSignificantRateChange(prev, curr float64) bool {
	diff := curr - prev
	if diff < 0 {
		diff = -diff
	}
	denom := max(prev, curr)
	return diff >= logRateChangeAbsThreshold && denom > 0 && diff/denom >= logRateChangeRelThreshold
}

// logRatePart formats the rate annotation for a log-derived anomaly description.
func logRatePart(a observerdef.Anomaly, storage observerdef.StorageReader) string {
	curr, ok := logPatternRate(a, storage)
	if !ok {
		return ""
	}
	if prev, ok := logPatternPrevRate(a, storage); ok && isSignificantRateChange(prev, curr) {
		return fmt.Sprintf("\n\trate: %.1flog/s (was %.1flog/s last minutes)", curr, prev)
	}
	return fmt.Sprintf("\n\trate: %.1flog/s", curr)
}

func (s *eventSender) send(c observerdef.ActiveCorrelation) error {
	msg := BuildChangeMessage(c, s.storage)
	ts := time.Unix(c.FirstSeen, 0).UTC().Format(time.RFC3339)
	aggKey := "observer:" + c.Pattern

	var host string
	if s.hostname != nil {
		host = s.hostname.GetSafe(context.TODO())
	}

	s.logger.Infof("[observer] sending change event: pattern=%s title=%q aggKey=%s timestamp=%s\n%s\n", c.Pattern, c.Title, aggKey, ts, msg)

	payload := buildChangeEventPayload(c, msg, ts, aggKey, host)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal change-event payload: %w", err)
	}

	epMsg := message.NewMessage(body, nil, "", time.Now().UnixNano())
	return s.forwarder.SendEventPlatformEventBlocking(epMsg, eventplatform.EventTypeEventManagement)
}

// sendEpisodeEvent sends a v2 change event for a scorer EpisodeStarted or EpisodeEnded
// lifecycle event. Unlike send(), this path is driven by the correlator's own PendingEvents
// and requires no reporter-side deduplication.
func (s *eventSender) sendEpisodeEvent(evt observerdef.CorrelatorEvent) error {
	var title, direction string
	switch evt.Kind {
	case observerdef.CorrelatorEventEpisodeStarted:
		title = fmt.Sprintf("Anomaly scorer: episode started (%s → %s)",
			severityLevelName(evt.FromLevel), severityLevelName(evt.ToLevel))
		direction = "started"
	case observerdef.CorrelatorEventEpisodeEnded:
		title = fmt.Sprintf("Anomaly scorer: episode ended (%s → %s)",
			severityLevelName(evt.FromLevel), severityLevelName(evt.ToLevel))
		direction = "ended"
	default:
		return fmt.Errorf("unsupported CorrelatorEventKind %d", evt.Kind)
	}

	ts := time.Unix(evt.Timestamp, 0).UTC().Format(time.RFC3339)
	aggKey := "observer:scorer:" + evt.CorrelatorName + ":" + evt.Correlation.Pattern
	msg := fmt.Sprintf("Anomaly scorer %q episode %s at t=%d\nPattern: %s",
		evt.CorrelatorName, direction, evt.Timestamp, evt.Correlation.Pattern)

	var host string
	if s.hostname != nil {
		host = s.hostname.GetSafe(context.TODO())
	}

	s.logger.Infof("[observer] sending scorer episode event: pattern=%s direction=%s aggKey=%s timestamp=%s",
		evt.Correlation.Pattern, direction, aggKey, ts)

	tags := []string{
		"source:edge-intelligence",
		"pattern:" + evt.Correlation.Pattern,
		"scorer:" + evt.CorrelatorName,
		"episode_direction:" + direction,
	}
	payload := map[string]any{
		"data": map[string]any{
			"type": "event",
			"attributes": map[string]any{
				"title":           title,
				"message":         msg,
				"category":        "change",
				"integration_id":  changeEventIntegrationID,
				"tags":            tags,
				"timestamp":       ts,
				"aggregation_key": aggKey,
				"attributes": map[string]any{
					"changed_resource": map[string]any{
						"name": truncateChars(evt.Correlation.Pattern, changedResourceNameMaxLen),
						"type": changedResourceType,
					},
					"author": map[string]any{
						"name": "datadog-agent-observer",
						"type": "automation",
					},
					"change_metadata": map[string]any{
						"episode_pattern":   evt.Correlation.Pattern,
						"episode_direction": direction,
						"from_level":        severityLevelName(evt.FromLevel),
						"to_level":          severityLevelName(evt.ToLevel),
						"first_seen":        time.Unix(evt.Correlation.FirstSeen, 0).UTC().Format(time.RFC3339),
						"last_updated":      time.Unix(evt.Correlation.LastUpdated, 0).UTC().Format(time.RFC3339),
					},
				},
			},
		},
	}
	if host != "" {
		payload["data"].(map[string]any)["attributes"].(map[string]any)["host"] = host
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal scorer episode event payload: %w", err)
	}

	epMsg := message.NewMessage(body, nil, "", time.Now().UnixNano())
	return s.forwarder.SendEventPlatformEventBlocking(epMsg, eventplatform.EventTypeEventManagement)
}

// severityLevelName returns a human-readable label for a SeverityLevel.
func severityLevelName(level severityeventsdef.SeverityLevel) string {
	switch level {
	case severityeventsdef.SeverityLow:
		return "low"
	case severityeventsdef.SeverityMedium:
		return "medium"
	case severityeventsdef.SeverityHigh:
		return "high"
	default:
		return fmt.Sprintf("level(%d)", int(level))
	}
}

// buildChangeEventPayload returns the v2 Events API JSON envelope for a
// correlation. Keys mirror the schema produced by datadog-api-client-go's
// EventCreateRequestPayload (data.type=event, data.attributes.{title, message,
// category, integration_id, tags, timestamp, aggregation_key, attributes}).
// integration_id pins the publisher to the `edge-intelligence` integration so
// the event-management intake can route and authorize the event.
func buildChangeEventPayload(c observerdef.ActiveCorrelation, msg, ts, aggKey, host string) map[string]any {
	attrs := map[string]any{
		"title":           c.Title,
		"message":         msg,
		"category":        "change",
		"integration_id":  changeEventIntegrationID,
		"tags":            BuildEventTags(c),
		"timestamp":       ts,
		"aggregation_key": aggKey,
		"attributes":      buildChangeAttributes(c),
	}
	if host != "" {
		attrs["host"] = host
	}
	return map[string]any{
		"data": map[string]any{
			"type":       "event",
			"attributes": attrs,
		},
	}
}

// BuildEventTags returns the Datadog event tags for a correlation.
// It always includes "source:edge-intelligence" and "pattern:{pattern}".
// It adds "anomaly_type:metric" and/or "anomaly_type:log" depending on which
// anomaly types are present (log-derived metric anomalies count as log).
// It also propagates "service:", "env:", and "host:" dimensions collected from
// each anomaly's source tags and from Context.SplitTags (set by the log pattern
// extractor for sub-clustered log series).
func BuildEventTags(c observerdef.ActiveCorrelation) []string {
	hasMetric := false
	hasLog := false
	dimensionSet := make(map[string]struct{})

	for _, a := range c.Anomalies {
		if a.Type == observerdef.AnomalyTypeLog || IsLogDerivedAnomaly(a) {
			hasLog = true
		} else {
			hasMetric = true
		}
		// Propagate dimensional tags from the source series.
		for _, t := range a.Source.Tags {
			for _, prefix := range []string{"service:", "env:", "host:"} {
				if strings.HasPrefix(t, prefix) {
					dimensionSet[t] = struct{}{}
					break
				}
			}
		}
		// For log-derived anomalies, dimensional info lives in Context.SplitTags
		// (set by the log tagged pattern clusterer).
		if a.Context != nil {
			for _, k := range []string{"service", "env", "host"} {
				if v, ok := a.Context.SplitTags[k]; ok {
					dimensionSet[k+":"+v] = struct{}{}
				}
			}
		}
	}

	tags := []string{"source:edge-intelligence", "pattern:" + c.Pattern}
	if hasMetric {
		tags = append(tags, "anomaly_type:metric")
	}
	if hasLog {
		tags = append(tags, "anomaly_type:log")
	}
	for t := range dimensionSet {
		tags = append(tags, t)
	}
	sort.Strings(tags[2:]) // keep source and pattern first; sort the rest for determinism
	return tags
}

// buildChangeAttributes constructs the nested change-event attributes block.
// The shape mirrors the v2 Events API ChangeEventCustomAttributes schema:
// changed_resource (required), author, impacted_resources, prev_value,
// new_value, change_metadata.
func buildChangeAttributes(c observerdef.ActiveCorrelation) map[string]any {
	name := truncateChars(c.Pattern, changedResourceNameMaxLen)
	attrs := map[string]any{
		"changed_resource": map[string]any{
			"name": name,
			"type": changedResourceType,
		},
		"author": map[string]any{
			"name": "datadog-agent-observer",
			"type": "automation",
		},
		"prev_value":      buildPrevValue(c),
		"new_value":       buildNewValue(c),
		"change_metadata": buildChangeMetadata(c),
	}
	if impacted := extractImpactedServices(c); len(impacted) > 0 {
		attrs["impacted_resources"] = impacted
	}
	return attrs
}

// extractImpactedServices collects unique service names from anomaly and member tags.
func extractImpactedServices(c observerdef.ActiveCorrelation) []map[string]any {
	seen := make(map[string]bool)
	for _, m := range c.Members {
		for _, tag := range m.Tags {
			if strings.HasPrefix(tag, "service:") {
				seen[strings.TrimPrefix(tag, "service:")] = true
			}
		}
	}
	for _, a := range c.Anomalies {
		for _, tag := range a.Source.Tags {
			if strings.HasPrefix(tag, "service:") {
				seen[strings.TrimPrefix(tag, "service:")] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for svc := range seen {
		names = append(names, svc)
	}
	sort.Strings(names)
	if len(names) > impactedResourcesMaxItems {
		names = names[:impactedResourcesMaxItems]
	}
	items := make([]map[string]any, 0, len(names))
	for _, svc := range names {
		items = append(items, map[string]any{
			"name": svc,
			"type": "service",
		})
	}
	return items
}

// buildPrevValue creates a per-series baseline snapshot for the change event.
func buildPrevValue(c observerdef.ActiveCorrelation) map[string]any {
	prev := make(map[string]any)
	for _, a := range c.Anomalies {
		if a.DebugInfo == nil {
			continue
		}
		key := anomalyDisplayKey(a)
		prev[key] = map[string]any{
			"mean":   a.DebugInfo.BaselineMean,
			"median": a.DebugInfo.BaselineMedian,
			"stddev": a.DebugInfo.BaselineStddev,
		}
	}
	return prev
}

// buildNewValue creates a per-series current-state snapshot for the change event.
func buildNewValue(c observerdef.ActiveCorrelation) map[string]any {
	newVal := make(map[string]any)
	for _, a := range c.Anomalies {
		if a.DebugInfo == nil {
			continue
		}
		key := anomalyDisplayKey(a)
		entry := map[string]any{
			"value":           a.DebugInfo.CurrentValue,
			"deviation_sigma": a.DebugInfo.DeviationSigma,
		}
		if a.DebugInfo.Threshold != 0 {
			entry["threshold"] = a.DebugInfo.Threshold
		}
		newVal[key] = entry
	}
	return newVal
}

// buildChangeMetadata creates the full structured anomaly inventory.
func buildChangeMetadata(c observerdef.ActiveCorrelation) map[string]any {
	var metricAnomalies, logAnomalies []any
	for _, a := range c.Anomalies {
		entry := map[string]any{
			"source":    a.Source.DisplayName(),
			"detector":  a.DetectorName,
			"title":     a.Title,
			"timestamp": a.Timestamp,
		}
		if a.Description != "" {
			entry["description"] = a.Description
		}
		if a.Score != nil {
			entry["score"] = *a.Score
		}
		if a.DebugInfo != nil {
			entry["debug_info"] = map[string]any{
				"baseline_mean":   a.DebugInfo.BaselineMean,
				"baseline_stddev": a.DebugInfo.BaselineStddev,
				"baseline_median": a.DebugInfo.BaselineMedian,
				"baseline_mad":    a.DebugInfo.BaselineMAD,
				"threshold":       a.DebugInfo.Threshold,
				"current_value":   a.DebugInfo.CurrentValue,
				"deviation_sigma": a.DebugInfo.DeviationSigma,
			}
		}
		if a.Context != nil {
			ctx := map[string]any{}
			if a.Context.Pattern != "" {
				ctx["pattern"] = a.Context.Pattern
			}
			if a.Context.Example != "" {
				ctx["example"] = a.Context.Example
			}
			if len(a.Context.SplitTags) > 0 {
				ctx["split_tags"] = a.Context.SplitTags
			}
			if len(ctx) > 0 {
				entry["context"] = ctx
			}
		}
		if a.Type == observerdef.AnomalyTypeLog || IsLogDerivedAnomaly(a) {
			logAnomalies = append(logAnomalies, entry)
		} else {
			metricAnomalies = append(metricAnomalies, entry)
		}
	}

	// Always emit both arrays (possibly empty) so the wire payload's shape
	// matches the spec's ChangeEvent entity: metric_anomalies and log_anomalies
	// are List<AnomalyInventoryEntry>, not optional.
	if metricAnomalies == nil {
		metricAnomalies = []any{}
	}
	if logAnomalies == nil {
		logAnomalies = []any{}
	}

	meta := map[string]any{
		"anomaly_count":    len(c.Anomalies),
		"first_seen":       time.Unix(c.FirstSeen, 0).UTC().Format(time.RFC3339),
		"last_updated":     time.Unix(c.LastUpdated, 0).UTC().Format(time.RFC3339),
		"sub_category":     classifyCorrelationSubCategory(c),
		"metric_anomalies": metricAnomalies,
		"log_anomalies":    logAnomalies,
	}
	if len(c.Members) > 0 {
		members := make([]string, len(c.Members))
		for i, m := range c.Members {
			members[i] = m.DisplayName()
		}
		meta["member_series"] = members
	}
	return meta
}

// BuildChangeMessage creates a compact human-readable summary for a correlation
// (Datadog change events, testbench JSON output, and replay-reported events).
// storage may be nil; when it is, log-rate annotations are omitted.
func BuildChangeMessage(c observerdef.ActiveCorrelation, storage observerdef.StorageReader) string {
	anomalyLines := []string{}
	for _, a := range c.Anomalies {
		if IsLogDerivedAnomaly(a) {
			anomalyLines = append(anomalyLines, "- "+logDerivedDescription(a, storage))
		} else if a.DebugInfo != nil {
			display := anomalyDisplayKey(a)
			anomalyLines = append(anomalyLines, fmt.Sprintf("- %s: %.2f (baseline mean: %.2f, %.1f sigma)", display, a.DebugInfo.CurrentValue, a.DebugInfo.BaselineMean, a.DebugInfo.DeviationSigma))
		} else if a.Description != "" {
			anomalyLines = append(anomalyLines, "- "+a.Description)
		} else {
			anomalyLines = append(anomalyLines, "- "+anomalyDisplayKey(a))
		}
	}

	// Ensure anomaly bullets are unique and sorted (two anomalies on the
	// same series at a similar timestamp render to the same bullet). The
	// header reports the deduplicated count so it matches the rendered list.
	slices.Sort(anomalyLines)
	anomalyLines = slices.Compact(anomalyLines)

	var lines []string
	lines = append(lines, fmt.Sprintf("Correlated behavior change detected: %d anomalies in pattern %q", len(anomalyLines), c.Pattern))
	lines = append(lines, "")
	lines = append(lines, anomalyLines...)
	text := strings.Join(lines, "\n")
	return truncateBytesValidUTF8(text, changeEventMessageMaxLen)
}

// truncateChars returns s unchanged when it is at most maxChars runes long.
// Otherwise it returns the first (maxChars-1) runes of s followed by an
// ellipsis rune, so the result is exactly maxChars characters and remains
// valid UTF-8. maxChars must be at least 1.
func truncateChars(s string, maxChars int) string {
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxChars-1]) + "…"
}

// truncateBytesValidUTF8 returns s unchanged when its byte length does not
// exceed maxBytes. Otherwise it delegates rune-boundary truncation to
// pkg/util/strings.TruncateUTF8, then appends a 3-byte ASCII ellipsis ("...")
// so the result is at most maxBytes bytes and remains valid UTF-8. maxBytes
// must be at least 4.
func truncateBytesValidUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	const ellipsis = "..."
	return pkgstrings.TruncateUTF8(s, maxBytes-len(ellipsis)) + ellipsis
}

// anomalyDisplayKey returns a human-readable key for an anomaly's source series.
func anomalyDisplayKey(a observerdef.Anomaly) string {
	if key := a.Source.DisplayName(); key != "" {
		return key
	}
	return a.Source.String()
}

// classifyCorrelationSubCategory chooses one of subCategorySpike,
// subCategoryDrop, or subCategoryNewPattern for a correlation. Event Management
// uses this to label the change event without parsing the message body.
//
// Rules, in priority order:
//  1. If any anomaly is a log-pattern detection without a meaningful baseline
//     (no DebugInfo, or DebugInfo.BaselineMean == 0), classify the whole
//     correlation as "new_pattern" — these represent novel signals that didn't
//     exist before.
//  2. Otherwise, if every anomaly with DebugInfo has CurrentValue <=
//     BaselineMean, the signal dropped — "drop".
//  3. Otherwise "spike".
func classifyCorrelationSubCategory(c observerdef.ActiveCorrelation) string {
	if len(c.Anomalies) == 0 {
		return subCategorySpike
	}
	for _, a := range c.Anomalies {
		if !(a.Type == observerdef.AnomalyTypeLog || IsLogDerivedAnomaly(a)) {
			continue
		}
		if a.DebugInfo == nil || a.DebugInfo.BaselineMean == 0 {
			return subCategoryNewPattern
		}
	}
	sawDebug := false
	allDrops := true
	for _, a := range c.Anomalies {
		if a.DebugInfo == nil {
			continue
		}
		sawDebug = true
		if a.DebugInfo.CurrentValue > a.DebugInfo.BaselineMean {
			allDrops = false
			break
		}
	}
	if sawDebug && allDrops {
		return subCategoryDrop
	}
	return subCategorySpike
}

// IsLogDerivedAnomaly returns true for metric anomalies that originate from
// log pattern extraction. These should be presented as log anomalies with
// pattern/example/rate context rather than raw metric descriptions.
func IsLogDerivedAnomaly(a observerdef.Anomaly) bool {
	if a.Type == observerdef.AnomalyTypeLog || a.Context == nil {
		return false
	}
	switch a.Source.Namespace {
	case logPatternExtractorNamespace:
		return strings.TrimSpace(a.Context.Pattern) != ""
	case logMetricsExtractorNamespace:
		return strings.TrimSpace(a.Context.Pattern) != "" || strings.TrimSpace(a.Context.Example) != ""
	}
	return false
}

// logDerivedDescription builds a human-readable description for a log-derived
// metric anomaly, including pattern, example, and windowed average rate.
func logDerivedDescription(a observerdef.Anomaly, storage observerdef.StorageReader) string {
	if a.Source.Namespace == logMetricsExtractorNamespace {
		return logFrequencyDerivedDescription(a, storage)
	}
	pattern := strings.TrimSpace(a.Context.Pattern)
	var example string
	// Don't display example if it's the same as the pattern
	if a.Context.Example != "" && strings.TrimSpace(a.Context.Example) != pattern {
		example = "\n\texample: " + strings.TrimSpace(a.Context.Example)
	}
	ratePart := logRatePart(a, storage)
	var tagsPart string
	if len(a.Context.SplitTags) > 0 {
		var parts []string
		for _, k := range splitTagKeyOrder {
			if v, ok := a.Context.SplitTags[k]; ok {
				parts = append(parts, k+"="+v)
			}
		}
		if len(parts) > 0 {
			tagsPart = "\n\ttags: " + strings.Join(parts, ", ")
		}
	}
	return fmt.Sprintf("Log pattern change rate detected:\n\tpattern: %s%s%s%s", pattern, example, ratePart, tagsPart)
}

// logFrequencyDerivedDescription builds a human-readable description for
// log.pattern.* anomalies from LogMetricsExtractor. The stored pattern is an
// internal tokenized structural signature (not human-readable), so the example
// log line is used as the primary identifier instead.
func logFrequencyDerivedDescription(a observerdef.Anomaly, storage observerdef.StorageReader) string {
	example := strings.TrimSpace(a.Context.Example)
	if example == "" {
		example = strings.TrimSpace(a.Context.Pattern)
	}
	return fmt.Sprintf("Log frequency change detected:\n\texample: %s%s", example, logRatePart(a, storage))
}
