// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
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
)

// eventSender formats and dispatches one Datadog event per correlation.
// When api is nil, send prints to stdout (dry-run mode) instead of calling the API.
type eventSender struct {
	api     *datadogV2.EventsApi
	ctx     context.Context
	logger  log.Component
	storage observerdef.StorageReader
}

// newEventSender creates an eventSender. It reads observer.event_reporter.sending_enabled
// from cfg; when false, api is left nil and events are only logged (dry-run mode).
// storage is used to compute windowed log rates for display in event messages.
func newEventSender(cfg config.Component, logger log.Component, storage observerdef.StorageReader) (*eventSender, error) {
	if !cfg.GetBool("observer.event_reporter.sending_enabled") {
		return &eventSender{logger: logger, storage: storage}, nil
	}
	apiKey := cfg.GetString("api_key")
	if apiKey == "" {
		return nil, errors.New("api_key is not set in configuration")
	}
	ctx := context.WithValue(
		datadog.NewDefaultContext(context.Background()),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{"apiKeyAuth": {Key: apiKey}},
	)
	return &eventSender{
		api:     datadogV2.NewEventsApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		ctx:     ctx,
		logger:  logger,
		storage: storage,
	}, nil
}

// logPatternRate returns the avg log/s over [T-60s, T]. Falls back to
// DebugInfo.CurrentValue when no storage ref is available.
func logPatternRate(a observerdef.Anomaly, storage observerdef.StorageReader) (rate float64, ok bool) {
	if a.SourceRef != nil && storage != nil {
		total := storage.SumRange(a.SourceRef.Ref, a.Timestamp-logPatternRateWindowSec, a.Timestamp, observerdef.AggregateCount)
		return total / logPatternRateWindowSec, true
	}
	if a.DebugInfo != nil {
		return a.DebugInfo.CurrentValue, true
	}
	return 0, false
}

// logPatternPrevRate returns the avg log/s over the baseline window [-6min, -1min].
// No DebugInfo fallback: CurrentValue is always present-tense.
func logPatternPrevRate(a observerdef.Anomaly, storage observerdef.StorageReader) (rate float64, ok bool) {
	if a.SourceRef != nil && storage != nil {
		start := a.Timestamp - logPatternPrevRateWindowSec - logPatternRateWindowSec
		total := storage.SumRange(a.SourceRef.Ref, start, a.Timestamp-logPatternRateWindowSec, observerdef.AggregateCount)
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

// send formats a correlation into a change event and either prints or posts it.
func (s *eventSender) send(c observerdef.ActiveCorrelation) error {
	msg := buildChangeMessage(c, s.storage)
	ts := time.Unix(c.FirstSeen, 0).UTC().Format(time.RFC3339)
	aggKey := "observer:" + c.Pattern

	s.logger.Infof("[observer] sending change event: pattern=%s title=%q aggKey=%s timestamp=%s\n%s\n", c.Pattern, c.Title, aggKey, ts, msg)

	if s.api == nil {
		fmt.Printf("[dry-run] change event: pattern=%s title=%q aggKey=%s timestamp=%s\n%s\n\n", c.Pattern, c.Title, aggKey, ts, msg)
		return nil
	}

	changeAttrs := buildChangeAttributes(c)
	attrs := datadogV2.ChangeEventCustomAttributesAsEventPayloadAttributes(&changeAttrs)
	payload := datadogV2.EventCreateRequestPayload{
		Data: datadogV2.EventCreateRequest{
			Type: datadogV2.EVENTCREATEREQUESTTYPE_EVENT,
			Attributes: datadogV2.EventPayload{
				Title:          c.Title,
				Message:        datadog.PtrString(msg),
				Category:       datadogV2.EVENTCATEGORY_CHANGE,
				Tags:           buildEventTags(c),
				Timestamp:      datadog.PtrString(ts),
				AggregationKey: datadog.PtrString(aggKey),
				Attributes:     attrs,
			},
		},
	}
	_, httpResp, err := s.api.CreateEvent(s.ctx, payload)
	if err != nil && httpResp != nil {
		body, readErr := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if readErr == nil {
			return fmt.Errorf("API error (HTTP %d): %s", httpResp.StatusCode, string(body))
		}
	}
	return err
}

// buildEventTags returns the Datadog event tags for a correlation.
// It always includes "source:agent-q-branch-observer" and "pattern:{pattern}".
// It adds "anomaly_type:metric" and/or "anomaly_type:log" depending on which
// anomaly types are present (log-derived metric anomalies count as log).
// It also propagates "service:", "env:", and "host:" dimensions collected from
// each anomaly's source tags and from Context.SplitTags (set by the log pattern
// extractor for sub-clustered log series).
func buildEventTags(c observerdef.ActiveCorrelation) []string {
	hasMetric := false
	hasLog := false
	dimensionSet := make(map[string]struct{})

	for _, a := range c.Anomalies {
		if a.Type == observerdef.AnomalyTypeLog || isLogDerivedAnomaly(a) {
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

	tags := []string{"source:agent-q-branch-observer", "pattern:" + c.Pattern}
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

// buildChangeAttributes constructs the change event attributes for a correlation.
func buildChangeAttributes(c observerdef.ActiveCorrelation) datadogV2.ChangeEventCustomAttributes {
	name := c.Pattern
	if len(name) > 128 {
		name = name[:128]
	}
	changedResource := *datadogV2.NewChangeEventCustomAttributesChangedResource(
		name,
		datadogV2.CHANGEEVENTCUSTOMATTRIBUTESCHANGEDRESOURCETYPE_CONFIGURATION,
	)
	changeAttrs := *datadogV2.NewChangeEventCustomAttributes(changedResource)

	author := *datadogV2.NewChangeEventCustomAttributesAuthor(
		"datadog-agent-observer",
		datadogV2.CHANGEEVENTCUSTOMATTRIBUTESAUTHORTYPE_AUTOMATION,
	)
	changeAttrs.SetAuthor(author)
	changeAttrs.SetImpactedResources(extractImpactedServices(c))
	changeAttrs.SetPrevValue(buildPrevValue(c))
	changeAttrs.SetNewValue(buildNewValue(c))
	changeAttrs.SetChangeMetadata(buildChangeMetadata(c))

	return changeAttrs
}

// extractImpactedServices collects unique service names from anomaly and member tags.
func extractImpactedServices(c observerdef.ActiveCorrelation) []datadogV2.ChangeEventCustomAttributesImpactedResourcesItems {
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
	var items []datadogV2.ChangeEventCustomAttributesImpactedResourcesItems
	for svc := range seen {
		items = append(items, *datadogV2.NewChangeEventCustomAttributesImpactedResourcesItems(
			svc,
			datadogV2.CHANGEEVENTCUSTOMATTRIBUTESIMPACTEDRESOURCESITEMSTYPE_SERVICE,
		))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	if len(items) > 100 {
		items = items[:100]
	}
	return items
}

// buildPrevValue creates a per-series baseline snapshot for the change event.
func buildPrevValue(c observerdef.ActiveCorrelation) map[string]interface{} {
	prev := make(map[string]interface{})
	for _, a := range c.Anomalies {
		if a.DebugInfo == nil {
			continue
		}
		key := anomalyDisplayKey(a)
		prev[key] = map[string]interface{}{
			"mean":   a.DebugInfo.BaselineMean,
			"median": a.DebugInfo.BaselineMedian,
			"stddev": a.DebugInfo.BaselineStddev,
		}
	}
	return prev
}

// buildNewValue creates a per-series current-state snapshot for the change event.
func buildNewValue(c observerdef.ActiveCorrelation) map[string]interface{} {
	newVal := make(map[string]interface{})
	for _, a := range c.Anomalies {
		if a.DebugInfo == nil {
			continue
		}
		key := anomalyDisplayKey(a)
		entry := map[string]interface{}{
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
func buildChangeMetadata(c observerdef.ActiveCorrelation) map[string]interface{} {
	var metricAnomalies, logAnomalies []interface{}
	for _, a := range c.Anomalies {
		entry := map[string]interface{}{
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
			entry["debug_info"] = map[string]interface{}{
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
			ctx := map[string]interface{}{}
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
		if a.Type == observerdef.AnomalyTypeLog || isLogDerivedAnomaly(a) {
			logAnomalies = append(logAnomalies, entry)
		} else {
			metricAnomalies = append(metricAnomalies, entry)
		}
	}

	meta := map[string]interface{}{
		"anomaly_count": len(c.Anomalies),
		"first_seen":    time.Unix(c.FirstSeen, 0).UTC().Format(time.RFC3339),
		"last_updated":  time.Unix(c.LastUpdated, 0).UTC().Format(time.RFC3339),
	}
	if len(metricAnomalies) > 0 {
		meta["metric_anomalies"] = metricAnomalies
	}
	if len(logAnomalies) > 0 {
		meta["log_anomalies"] = logAnomalies
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

// buildChangeMessage creates a compact human-readable summary for a correlation (Datadog
// change events, testbench JSON output, and replay-reported events).
func buildChangeMessage(c observerdef.ActiveCorrelation, storage observerdef.StorageReader) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Correlated behavior change detected: %d anomalies in pattern %q", len(c.Anomalies), c.Pattern))
	lines = append(lines, "")

	anomalyLines := []string{}
	for _, a := range c.Anomalies {
		if isLogDerivedAnomaly(a) {
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

	// Ensure anomalies are unique and sorted (could be duplicate if 2 anomalies on the same series at a similar timestamp)
	slices.Sort(anomalyLines)
	lines = append(lines, slices.Compact(anomalyLines)...)
	text := strings.Join(lines, "\n")
	const maxLen = 4000
	if len(text) > maxLen {
		text = text[:maxLen-3] + "..."
	}
	return text
}

// anomalyDisplayKey returns a human-readable key for an anomaly's source series.
func anomalyDisplayKey(a observerdef.Anomaly) string {
	if key := a.Source.DisplayName(); key != "" {
		return key
	}
	return a.Source.String()
}

// isLogDerivedAnomaly returns true for metric anomalies that originate from
// log pattern extraction. These should be presented as log anomalies with
// pattern/example/rate context rather than raw metric descriptions.
func isLogDerivedAnomaly(a observerdef.Anomaly) bool {
	if a.Type == observerdef.AnomalyTypeLog || a.Context == nil {
		return false
	}
	switch a.Source.Namespace {
	case LogPatternExtractorName:
		return strings.TrimSpace(a.Context.Pattern) != ""
	case LogMetricsExtractorName:
		return strings.TrimSpace(a.Context.Pattern) != "" || strings.TrimSpace(a.Context.Example) != ""
	}
	return false
}

// logDerivedDescription builds a human-readable description for a log-derived
// metric anomaly, including pattern, example, and windowed average rate.
func logDerivedDescription(a observerdef.Anomaly, storage observerdef.StorageReader) string {
	if a.Source.Namespace == LogMetricsExtractorName {
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

// sendCorrelationEvents sends one event per correlation.
func (s *eventSender) sendCorrelationEvents(correlations []observerdef.ActiveCorrelation) {
	for _, c := range correlations {
		if err := s.send(c); err != nil {
			s.logger.Errorf("[observer] failed to send event for pattern %s: %v", c.Pattern, err)
		}
	}
}

// newLiveEventSender creates an eventSender that always posts to the Datadog
// API (ignoring observer.event_reporter.sending_enabled). Used for on-demand
// sends from the testbench UI. Returns an error if api_key is not set.
func newLiveEventSender(cfg config.Component, logger log.Component, storage observerdef.StorageReader) (*eventSender, error) {
	apiKey := cfg.GetString("api_key")
	if apiKey == "" {
		return nil, errors.New("api_key is not set in configuration")
	}
	ctx := context.WithValue(
		datadog.NewDefaultContext(context.Background()),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{"apiKeyAuth": {Key: apiKey}},
	)
	return &eventSender{
		api:     datadogV2.NewEventsApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		ctx:     ctx,
		logger:  logger,
		storage: storage,
	}, nil
}

// sendReportedEvent posts a single ReportedEvent to the Datadog backend.
// It replaces the original source tag with "source:observer-testbench" and
// appends the provided extraTags (e.g. scenario and user).
func (s *eventSender) sendReportedEvent(event ReportedEvent, extraTags []string) error {
	// Rebuild tags: drop original source, replace with testbench source, add extras.
	tags := []string{"source:observer-testbench"}
	for _, t := range event.Tags {
		if strings.HasPrefix(t, "source:") {
			continue
		}
		tags = append(tags, t)
	}
	tags = append(tags, extraTags...)
	sort.Strings(tags[1:]) // keep source:observer-testbench first

	// Use current time: replay scenario timestamps are often hours/days old and
	// the Datadog API rejects events with timestamps outside its acceptance window.
	ts := time.Now().UTC().Format(time.RFC3339)
	aggKey := "testbench:" + event.Pattern

	s.logger.Infof("[observer] sending testbench event: pattern=%s title=%q", event.Pattern, event.Title)

	if s.api == nil {
		fmt.Printf("[dry-run] testbench event: pattern=%s title=%q tags=%v\n%s\n\n", event.Pattern, event.Title, tags, event.Message)
		return nil
	}

	// EventPayload.Attributes is a required union field — omitting it causes a
	// marshal error. Build a minimal ChangeEventCustomAttributes from the pattern.
	name := event.Pattern
	if len(name) > 128 {
		name = name[:128]
	}
	changedResource := *datadogV2.NewChangeEventCustomAttributesChangedResource(
		name,
		datadogV2.CHANGEEVENTCUSTOMATTRIBUTESCHANGEDRESOURCETYPE_CONFIGURATION,
	)
	changeAttrs := *datadogV2.NewChangeEventCustomAttributes(changedResource)
	changeAttrs.SetAuthor(*datadogV2.NewChangeEventCustomAttributesAuthor(
		"datadog-agent-observer-testbench",
		datadogV2.CHANGEEVENTCUSTOMATTRIBUTESAUTHORTYPE_AUTOMATION,
	))
	attrs := datadogV2.ChangeEventCustomAttributesAsEventPayloadAttributes(&changeAttrs)

	payload := datadogV2.EventCreateRequestPayload{
		Data: datadogV2.EventCreateRequest{
			Type: datadogV2.EVENTCREATEREQUESTTYPE_EVENT,
			Attributes: datadogV2.EventPayload{
				Title:          event.Title,
				Message:        datadog.PtrString(event.Message),
				Category:       datadogV2.EVENTCATEGORY_CHANGE,
				Tags:           tags,
				Timestamp:      datadog.PtrString(ts),
				AggregationKey: datadog.PtrString(aggKey),
				Attributes:     attrs,
			},
		},
	}
	_, httpResp, err := s.api.CreateEvent(s.ctx, payload)
	if err != nil && httpResp != nil {
		body, readErr := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if readErr == nil {
			return fmt.Errorf("API error (HTTP %d): %s", httpResp.StatusCode, string(body))
		}
	}
	return err
}
