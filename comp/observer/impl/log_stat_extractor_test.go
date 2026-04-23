// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// normalizeStatus

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// emergency/alert/critical → "critical"
		{"emergency", "emergency", "critical"},
		{"alert", "alert", "critical"},
		{"critical", "critical", "critical"},
		{"EMERGENCY", "EMERGENCY", "critical"},
		{"ALERT", "ALERT", "critical"},
		{"CRITICAL", "CRITICAL", "critical"},
		// error
		{"error", "error", "error"},
		{"ERROR", "ERROR", "error"},
		// warn/warning → "warn"
		{"warn", "warn", "warn"},
		{"warning", "warning", "warn"},
		{"WARNING", "WARNING", "warn"},
		{"Warning", "Warning", "warn"},
		// notice/info → "info"
		{"notice", "notice", "info"},
		{"info", "info", "info"},
		{"NOTICE", "NOTICE", "info"},
		{"INFO", "INFO", "info"},
		// debug
		{"debug", "debug", "debug"},
		{"DEBUG", "DEBUG", "debug"},
		// Unknown → "info" fallback
		{"trace", "trace", "info"},
		{"verbose", "verbose", "info"},
		{"fatal", "fatal", "info"},
		{"ok", "ok", "info"},
		{"empty", "", "info"},
		{"punctuation", "???", "info"},
		{"UNKNOWN", "UNKNOWN", "info"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeStatus(tc.input))
		})
	}
}

// ProcessLog core behavior

func TestLogStatExtractor_EmitsMetricForKnownStatus(t *testing.T) {
	e := NewLogStatExtractor()

	log := &mockLogView{
		content: []byte("user logged in"),
		status:  "info",
		tags:    []string{"service:web", "env:prod"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	assert.Equal(t, logStatMetricNames["info"], res.Metrics[0].Name)
	assert.Equal(t, 1.0, res.Metrics[0].Value)
}

func TestLogStatExtractor_UnknownStatusCountedAsInfo(t *testing.T) {
	e := NewLogStatExtractor()

	log := &mockLogView{
		content: []byte("some trace log"),
		status:  "trace",
		tags:    []string{"service:web"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	assert.Equal(t, logStatMetricNames["info"], res.Metrics[0].Name)
}

func TestLogStatExtractor_TagsAreForwardedAsIs(t *testing.T) {
	e := NewLogStatExtractor()

	tags := []string{"service:api", "env:staging", "version:1.2.3"}
	log := &mockLogView{
		content: []byte("starting up"),
		status:  "info",
		tags:    tags,
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	assert.Equal(t, tags, res.Metrics[0].Tags)
}

func TestLogStatExtractor_ContextKeyContainsStatus(t *testing.T) {
	e := NewLogStatExtractor()

	log := &mockLogView{
		content: []byte("disk full"),
		status:  "error",
		tags:    []string{"service:storage"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	assert.True(t, strings.Contains(res.Metrics[0].ContextKey, "error"),
		"context key %q should contain the status", res.Metrics[0].ContextKey)
}

func TestLogStatExtractor_GetContextByKeyReturnsPatternAndExample(t *testing.T) {
	e := NewLogStatExtractor()

	log := &mockLogView{
		content: []byte("connection timeout"),
		status:  "warn",
		tags:    []string{"service:db"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)

	ctx, ok := e.GetContextByKey(res.Metrics[0].ContextKey)
	require.True(t, ok)
	assert.Equal(t, "warn", ctx.Pattern)
	assert.Equal(t, "connection timeout", ctx.Example)
	assert.Equal(t, logStatExtractorName, ctx.Source)
}

// Tag-group isolation

func TestLogStatExtractor_SameStatusDifferentServiceProducesDifferentContextKey(t *testing.T) {
	e := NewLogStatExtractor()

	logA := &mockLogView{content: []byte("msg"), status: "info", tags: []string{"service:api"}}
	logB := &mockLogView{content: []byte("msg"), status: "info", tags: []string{"service:worker"}}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)

	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	assert.NotEqual(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)
}

func TestLogStatExtractor_SameStatusSameGroupProducesSameContextKey(t *testing.T) {
	e := NewLogStatExtractor()

	tags := []string{"service:api", "env:prod"}
	logA := &mockLogView{content: []byte("first"), status: "info", tags: tags}
	logB := &mockLogView{content: []byte("second"), status: "info", tags: tags}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)

	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	assert.Equal(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)
}

func TestLogStatExtractor_HostnameInjectedIntoGroupingWhenNoHostTag(t *testing.T) {
	e := NewLogStatExtractor()

	tags := []string{"service:api", "env:prod"}
	logA := &mockLogView{content: []byte("msg"), status: "info", tags: tags, hostname: "host-a"}
	logB := &mockLogView{content: []byte("msg"), status: "info", tags: tags, hostname: "host-b"}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)

	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	// Different hostnames → different group hash → different context keys.
	assert.NotEqual(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)

	ctxA, ok := e.GetContextByKey(resA.Metrics[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := e.GetContextByKey(resB.Metrics[0].ContextKey)
	require.True(t, ok)

	// The grouping tag "host" should appear in SplitTags (from hostname injection),
	// but the emitted metric tags should be the original tags without the injected host.
	assert.Equal(t, "host-a", ctxA.SplitTags["host"])
	assert.Equal(t, "host-b", ctxB.SplitTags["host"])
	// Original tags are not enriched with the hostname.
	assert.Equal(t, tags, resA.Metrics[0].Tags)
	assert.Equal(t, tags, resB.Metrics[0].Tags)
}

func TestLogStatExtractor_SplitTagsReflectGroupDimensions(t *testing.T) {
	e := NewLogStatExtractor()

	log := &mockLogView{
		content: []byte("request failed"),
		status:  "error",
		tags:    []string{"service:gateway", "env:prod", "version:2.0"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)

	ctx, ok := e.GetContextByKey(res.Metrics[0].ContextKey)
	require.True(t, ok)
	// SplitTags should only contain the grouping dimensions, not "version".
	assert.Equal(t, map[string]string{"service": "gateway", "env": "prod"}, ctx.SplitTags)
}

// Catalog registration

func TestLogStatExtractor_PresentInDefaultCatalog(t *testing.T) {
	cat := defaultCatalog()
	var found *componentEntry
	for i := range cat.entries {
		if cat.entries[i].name == logStatExtractorName {
			found = &cat.entries[i]
			break
		}
	}
	require.NotNil(t, found, "log_stat_extractor must be registered in defaultCatalog")
	assert.True(t, found.defaultEnabled, "log_stat_extractor must be enabled by default")
	assert.Equal(t, componentExtractor, found.kind)
}

func TestLogStatExtractor_InstantiateIncludesExtractor(t *testing.T) {
	_, _, extractors, _ := defaultCatalog().Instantiate(ComponentSettings{})

	var found bool
	for _, ext := range extractors {
		if ext.Name() == logStatExtractorName {
			found = true
			break
		}
	}
	assert.True(t, found, "log_stat_extractor must appear in the extractors slice from Instantiate")
}
