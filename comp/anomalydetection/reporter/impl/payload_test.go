// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// TestBuildChangeEventPayload_WireShape asserts the JSON envelope produced for
// the event-management intake matches the v2 Events API ChangeEvent schema we
// used to obtain via datadog-api-client-go, including the edge-intelligence
// routing metadata (integration_id) and the anomaly resource type. The
// contract here is the wire format, not the helper functions: if the intake
// changes field names or the SME-agreed values shift, we want this test to
// fail loudly.
func TestBuildChangeEventPayload_WireShape(t *testing.T) {
	c := observerdef.ActiveCorrelation{
		Pattern: "kernel_bottleneck",
		Title:   "kernel bottleneck detected",
		Anomalies: []observerdef.Anomaly{
			{
				Type:   observerdef.AnomalyTypeMetric,
				Source: observerdef.SeriesDescriptor{Namespace: "dogstatsd", Tags: []string{"service:web", "env:prod"}},
				DebugInfo: &observerdef.AnomalyDebugInfo{
					CurrentValue: 10,
					BaselineMean: 1,
				},
			},
		},
	}

	payload := buildChangeEventPayload(c, "hello", "2024-01-01T00:00:00Z", "observer:kernel_bottleneck", "my-test-host")

	// Round-trip through JSON so we exercise the same marshalling path as send().
	blob, err := json.Marshal(payload)
	assert.NoError(t, err)

	var decoded map[string]any
	assert.NoError(t, json.Unmarshal(blob, &decoded))

	data, ok := decoded["data"].(map[string]any)
	assert.True(t, ok, "missing data envelope")
	assert.Equal(t, "event", data["type"])

	attrs, ok := data["attributes"].(map[string]any)
	assert.True(t, ok, "missing data.attributes")
	assert.Equal(t, "kernel bottleneck detected", attrs["title"])
	assert.Equal(t, "hello", attrs["message"])
	assert.Equal(t, "change", attrs["category"])
	assert.Equal(t, "2024-01-01T00:00:00Z", attrs["timestamp"])
	assert.Equal(t, "observer:kernel_bottleneck", attrs["aggregation_key"])

	// host is required by the event-management intake (mirrors notableevents and logonduration).
	assert.Equal(t, "my-test-host", attrs["host"])

	// edge-intelligence routing: must be present and locked to the registered
	// value from integrations-internal-core#3240.
	assert.Equal(t, "edge-intelligence", attrs["integration_id"])
	assert.NotContains(t, attrs, "source_type_id", "source_type_id must not be sent; routing relies on integration_id alone")

	tags, ok := attrs["tags"].([]any)
	assert.True(t, ok, "tags must be a JSON array")
	assert.NotEmpty(t, tags)

	inner, ok := attrs["attributes"].(map[string]any)
	assert.True(t, ok, "missing nested change-event attributes")

	changed, ok := inner["changed_resource"].(map[string]any)
	assert.True(t, ok, "missing changed_resource")
	assert.Equal(t, "kernel_bottleneck", changed["name"])
	// Resource type is `anomaly` (Event Management validation was updated to
	// accept this value); the previous `configuration` value would be rejected.
	assert.Equal(t, "anomaly", changed["type"])

	author, ok := inner["author"].(map[string]any)
	assert.True(t, ok, "missing author")
	assert.Equal(t, "datadog-agent-observer", author["name"])
	assert.Equal(t, "automation", author["type"])

	impacted, ok := inner["impacted_resources"].([]any)
	assert.True(t, ok, "missing impacted_resources")
	assert.Len(t, impacted, 1)
	item := impacted[0].(map[string]any)
	assert.Equal(t, "web", item["name"])
	assert.Equal(t, "service", item["type"])

	assert.Contains(t, inner, "prev_value")
	assert.Contains(t, inner, "new_value")

	meta, ok := inner["change_metadata"].(map[string]any)
	assert.True(t, ok, "missing change_metadata")
	assert.Equal(t, "spike", meta["sub_category"], "current > baseline should classify as spike")
}

// TestBuildChangeEventPayload_TruncatesChangedResourceName ensures we don't
// emit a name longer than the v2 API accepts (128 chars), that truncated
// names end with an ellipsis to signal the cut, and that the result remains
// valid UTF-8 even when the cut would land mid-rune.
func TestBuildChangeEventPayload_TruncatesChangedResourceName(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	c := observerdef.ActiveCorrelation{Pattern: string(long), Title: "t"}

	payload := buildChangeEventPayload(c, "m", "2024-01-01T00:00:00Z", "k", "")

	inner := payload["data"].(map[string]any)["attributes"].(map[string]any)["attributes"].(map[string]any)
	changed := inner["changed_resource"].(map[string]any)
	name := changed["name"].(string)
	assert.Equal(t, changedResourceNameMaxLen, utf8.RuneCountInString(name), "truncated name should be exactly maxChars runes long")
	assert.True(t, strings.HasSuffix(name, "…"), "truncated name should end with an ellipsis")
}

// TestBuildChangeEventPayload_TruncatesAtRuneBoundary checks that a pattern
// whose final rune (before maxChars) is multi-byte is not cut mid-codepoint:
// the resulting name must stay valid UTF-8 and contain exactly maxChars runes.
func TestBuildChangeEventPayload_TruncatesAtRuneBoundary(t *testing.T) {
	// Build a pattern of 200 three-byte runes; any byte-indexed truncation at
	// changedResourceNameMaxLen would land inside a rune.
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteRune('☃') // U+2603, 3 bytes
	}
	c := observerdef.ActiveCorrelation{Pattern: b.String(), Title: "t"}

	payload := buildChangeEventPayload(c, "m", "2024-01-01T00:00:00Z", "k", "")

	inner := payload["data"].(map[string]any)["attributes"].(map[string]any)["attributes"].(map[string]any)
	name := inner["changed_resource"].(map[string]any)["name"].(string)
	assert.True(t, utf8.ValidString(name), "truncated name must remain valid UTF-8")
	assert.Equal(t, changedResourceNameMaxLen, utf8.RuneCountInString(name), "truncated name should be exactly maxChars runes long")
	assert.True(t, strings.HasSuffix(name, "…"), "truncated name should end with an ellipsis")
}

// TestBuildChangeEventPayload_AnomalyInventoryAlwaysPresent asserts that both
// metric_anomalies and log_anomalies are present in change_metadata regardless
// of which categories the correlation contains. The spec's ChangeEvent entity
// declares both as List<AnomalyInventoryEntry> (always-present, possibly
// empty); intake consumers should not need to handle field absence.
func TestBuildChangeEventPayload_AnomalyInventoryAlwaysPresent(t *testing.T) {
	cases := map[string]observerdef.ActiveCorrelation{
		"metric only": {
			Pattern: "p",
			Anomalies: []observerdef.Anomaly{{
				Type:      observerdef.AnomalyTypeMetric,
				Source:    observerdef.SeriesDescriptor{Namespace: "dogstatsd"},
				DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 5, BaselineMean: 1},
			}},
		},
		"log only": {
			Pattern: "p",
			Anomalies: []observerdef.Anomaly{{
				Type:   observerdef.AnomalyTypeLog,
				Source: observerdef.SeriesDescriptor{Namespace: "log_detector"},
			}},
		},
		"empty": {Pattern: "p"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			payload := buildChangeEventPayload(c, "m", "2024-01-01T00:00:00Z", "k", "")
			inner := payload["data"].(map[string]any)["attributes"].(map[string]any)["attributes"].(map[string]any)
			meta := inner["change_metadata"].(map[string]any)
			assert.Contains(t, meta, "metric_anomalies", "metric_anomalies must be present")
			assert.Contains(t, meta, "log_anomalies", "log_anomalies must be present")
			// Round-trip through JSON to confirm both arrays serialise as [] rather
			// than being elided or marshalled to null when empty.
			blob, err := json.Marshal(payload)
			assert.NoError(t, err)
			var decoded map[string]any
			assert.NoError(t, json.Unmarshal(blob, &decoded))
			rtMeta := decoded["data"].(map[string]any)["attributes"].(map[string]any)["attributes"].(map[string]any)["change_metadata"].(map[string]any)
			assert.NotNil(t, rtMeta["metric_anomalies"], "metric_anomalies must serialise as an array, not null")
			assert.NotNil(t, rtMeta["log_anomalies"], "log_anomalies must serialise as an array, not null")
		})
	}
}

// TestBuildChangeEventPayload_NoImpactedResourcesWhenEmpty makes sure we omit
// the impacted_resources key entirely (rather than emitting an empty array)
// when no service tags are present, matching the upstream omitempty behaviour.
func TestBuildChangeEventPayload_NoImpactedResourcesWhenEmpty(t *testing.T) {
	c := observerdef.ActiveCorrelation{Pattern: "p", Title: "t"}

	payload := buildChangeEventPayload(c, "m", "2024-01-01T00:00:00Z", "k", "")

	inner := payload["data"].(map[string]any)["attributes"].(map[string]any)["attributes"].(map[string]any)
	_, present := inner["impacted_resources"]
	assert.False(t, present, "impacted_resources should be omitted when no services are impacted")
}

// TestBuildChangeEventPayload_HostOmittedWhenEmpty verifies that when no host
// is available (empty string), the field is not present in the payload so the
// intake does not receive a blank host value.
func TestBuildChangeEventPayload_HostOmittedWhenEmpty(t *testing.T) {
	c := observerdef.ActiveCorrelation{Pattern: "p", Title: "t"}

	payload := buildChangeEventPayload(c, "m", "2024-01-01T00:00:00Z", "k", "")

	attrs := payload["data"].(map[string]any)["attributes"].(map[string]any)
	_, present := attrs["host"]
	assert.False(t, present, "host should be omitted when empty")
}

// --- classifyCorrelationSubCategory ---

func TestClassifySubCategory_SpikeOnIncrease(t *testing.T) {
	c := observerdef.ActiveCorrelation{Anomalies: []observerdef.Anomaly{{
		Type:      observerdef.AnomalyTypeMetric,
		DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 10, BaselineMean: 2},
	}}}
	assert.Equal(t, subCategorySpike, classifyCorrelationSubCategory(c))
}

func TestClassifySubCategory_DropWhenAllBelowBaseline(t *testing.T) {
	c := observerdef.ActiveCorrelation{Anomalies: []observerdef.Anomaly{
		{
			Type:      observerdef.AnomalyTypeMetric,
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 1, BaselineMean: 5},
		},
		{
			Type:      observerdef.AnomalyTypeMetric,
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 0.5, BaselineMean: 3},
		},
	}}
	assert.Equal(t, subCategoryDrop, classifyCorrelationSubCategory(c))
}

func TestClassifySubCategory_SpikeWhenMixedDirections(t *testing.T) {
	// One spike, one drop — not a uniform drop, so we default to spike
	// (the more eye-catching framing for a heterogeneous correlation).
	c := observerdef.ActiveCorrelation{Anomalies: []observerdef.Anomaly{
		{
			Type:      observerdef.AnomalyTypeMetric,
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 1, BaselineMean: 5},
		},
		{
			Type:      observerdef.AnomalyTypeMetric,
			DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 10, BaselineMean: 2},
		},
	}}
	assert.Equal(t, subCategorySpike, classifyCorrelationSubCategory(c))
}

func TestClassifySubCategory_NewPatternForLogWithoutBaseline(t *testing.T) {
	c := observerdef.ActiveCorrelation{Anomalies: []observerdef.Anomaly{{
		Type:   observerdef.AnomalyTypeLog,
		Source: observerdef.SeriesDescriptor{Namespace: "log_detector"},
	}}}
	assert.Equal(t, subCategoryNewPattern, classifyCorrelationSubCategory(c))
}

func TestClassifySubCategory_NewPatternForLogDerivedZeroBaseline(t *testing.T) {
	// A log-pattern metric anomaly with BaselineMean=0 means the pattern
	// didn't exist before — classify as new_pattern, not spike.
	c := observerdef.ActiveCorrelation{Anomalies: []observerdef.Anomaly{{
		Type:   observerdef.AnomalyTypeMetric,
		Source: observerdef.SeriesDescriptor{Namespace: logPatternExtractorNamespace},
		Context: &observerdef.MetricContext{
			Pattern: "connection refused",
		},
		DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 5, BaselineMean: 0},
	}}}
	assert.Equal(t, subCategoryNewPattern, classifyCorrelationSubCategory(c))
}

func TestClassifySubCategory_LogDerivedWithBaselineStaysAsSpikeOrDrop(t *testing.T) {
	// Log-pattern metric anomaly with a real baseline behaves like a normal
	// rate change: classified by direction, not as new_pattern.
	c := observerdef.ActiveCorrelation{Anomalies: []observerdef.Anomaly{{
		Type:   observerdef.AnomalyTypeMetric,
		Source: observerdef.SeriesDescriptor{Namespace: logPatternExtractorNamespace},
		Context: &observerdef.MetricContext{
			Pattern: "connection refused",
		},
		DebugInfo: &observerdef.AnomalyDebugInfo{CurrentValue: 50, BaselineMean: 5},
	}}}
	assert.Equal(t, subCategorySpike, classifyCorrelationSubCategory(c))
}

func TestClassifySubCategory_EmptyCorrelationDefaultsToSpike(t *testing.T) {
	assert.Equal(t, subCategorySpike, classifyCorrelationSubCategory(observerdef.ActiveCorrelation{}))
}
