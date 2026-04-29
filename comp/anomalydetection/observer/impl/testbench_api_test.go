// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

type stubLogView struct {
	status    string
	timestamp int64
	tags      []string
	hostname  string
	content   []byte
}

func (l stubLogView) GetContent() []byte           { return l.content }
func (l stubLogView) GetTags() []string            { return l.tags }
func (l stubLogView) GetTimestampUnixMilli() int64 { return l.timestamp }
func (l stubLogView) GetStatus() string            { return l.status }
func (l stubLogView) GetHostname() string          { return l.hostname }

var _ observerdef.LogView = stubLogView{}

func TestMatchesLogTagFilter(t *testing.T) {
	filter := parseLogTagFilter("service:api env:prod -host:web-2 -team")

	assert.True(t, matchesLogTagFilter(
		[]string{"service:api", "env:prod", "host:web-1"},
		filter,
	))
	assert.False(t, matchesLogTagFilter(
		[]string{"service:api", "env:prod", "host:web-2"},
		filter,
	))
	assert.False(t, matchesLogTagFilter(
		[]string{"service:api", "env:prod", "team:infra"},
		filter,
	))
	assert.False(t, matchesLogTagFilter(
		[]string{"service:worker", "env:prod", "host:web-1"},
		filter,
	))
}

func TestCloneCompressedGroupsDeepCopiesMembers(t *testing.T) {
	original := []CompressedGroup{{
		CorrelatorName: "corr",
		GroupID:        "group-1",
		Title:          "group",
		CommonTags:     map[string]string{"env": "prod"},
		Patterns:       []MetricPattern{{Pattern: "svc.*", Matched: 1, Universe: 1, Precision: 1}},
		MemberSources:  []string{"full|metric:avg|tag:a"},
	}}

	cloned := cloneCompressedGroups(original)
	require.Len(t, cloned, 1)

	cloned[0].CommonTags["env"] = "staging"
	cloned[0].Patterns[0].Pattern = "other.*"
	cloned[0].MemberSources[0] = "rewritten"

	assert.Equal(t, "prod", original[0].CommonTags["env"])
	assert.Equal(t, "svc.*", original[0].Patterns[0].Pattern)
	assert.Equal(t, "full|metric:avg|tag:a", original[0].MemberSources[0])
}

func TestMatchesLogsQueryKind(t *testing.T) {
	rawLog := stubLogView{status: "info", timestamp: 1000, tags: []string{"service:api"}}
	telemetryLog := stubLogView{status: "info", timestamp: 1000, tags: []string{"service:api", "telemetry:true"}}

	assert.True(t, matchesLogsQuery(rawLog, logsQuery{kind: "all"}))
	assert.True(t, matchesLogsQuery(telemetryLog, logsQuery{kind: "all"}))
	assert.True(t, matchesLogsQuery(rawLog, logsQuery{kind: "raw"}))
	assert.False(t, matchesLogsQuery(telemetryLog, logsQuery{kind: "raw"}))
	assert.False(t, matchesLogsQuery(rawLog, logsQuery{kind: "telemetry"}))
	assert.True(t, matchesLogsQuery(telemetryLog, logsQuery{kind: "telemetry"}))
}

// This will verify that log pattern extractor series are marked as virtual.
func TestHandleSeriesListMarksLogPatternExtractorSeriesAsVirtual(t *testing.T) {
	tb, err := NewTestBench(TestBenchConfig{ScenariosDir: t.TempDir()})
	require.NoError(t, err)
	api := NewTestBenchAPI(tb)
	if ext := tb.getLogPatternExtractor(); ext != nil {
		ext.config.MinClusterSizeBeforeEmit = 1
	}

	_, _ = tb.engine.IngestLog("test-source", &logObs{
		content:     []byte("GET /users/123 returned 500"),
		status:      "warn",
		tags:        []string{"service:api"},
		timestampMs: 2_000,
	})
	// Non-extractor namespace — should not be tagged virtual.
	tb.engine.IngestMetric("full", &metricObs{
		name:      "cpu.usage",
		value:     42,
		timestamp: 2,
		tags:      []string{"host:host-a"},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/series", nil)
	api.handleSeriesList(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body []struct {
		Namespace string `json:"namespace"`
		Virtual   bool   `json:"virtual"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	var logPatternSeen bool
	for _, s := range body {
		switch s.Namespace {
		case "log_pattern_extractor":
			logPatternSeen = true
			assert.True(t, s.Virtual, "log_pattern_extractor series should be virtual")
		case "full":
			assert.False(t, s.Virtual, "non-extractor namespace should not be virtual")
		}
	}
	require.True(t, logPatternSeen, "expected at least one log_pattern_extractor series in /api/series")
}

// Telemetry counters used to be listed as avg+count+sum with identical tags, which produced three
// duplicate "untagged" lines in the metrics UI for metrics like observer.input_logs.count.
func TestHandleSeriesListTelemetryCountersSingleAggregate(t *testing.T) {
	tb, err := NewTestBench(TestBenchConfig{ScenariosDir: t.TempDir()})
	require.NoError(t, err)
	api := NewTestBenchAPI(tb)

	tb.engine.Storage().Add(observerdef.TelemetryNamespace, telemetryTbInputLogsCount, 1, 100, nil)
	tb.engine.Storage().Add(observerdef.TelemetryNamespace, telemetryDetectorProcessingTimeNs, 1e6, 100, []string{"detector:rrcf"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/series", nil)
	api.handleSeriesList(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body []struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	var counterSuffixes []string
	var gaugeTelemetryNames []string
	for _, s := range body {
		if s.Namespace != observerdef.TelemetryNamespace {
			continue
		}
		switch {
		case strings.HasPrefix(s.Name, telemetryTbInputLogsCount+":"):
			counterSuffixes = append(counterSuffixes, strings.TrimPrefix(s.Name, telemetryTbInputLogsCount+":"))
		case strings.HasPrefix(s.Name, telemetryDetectorProcessingTimeNs+":"):
			gaugeTelemetryNames = append(gaugeTelemetryNames, s.Name)
		}
	}
	require.ElementsMatch(t, []string{"sum"}, counterSuffixes)
	require.Len(t, gaugeTelemetryNames, 2)
}

func TestHandleNumericSeriesDataRejectsUnknownAggregation(t *testing.T) {
	tb, err := NewTestBench(TestBenchConfig{ScenariosDir: t.TempDir()})
	require.NoError(t, err)

	tb.engine.Storage().Add("test", "metric", 1, 100, nil)
	api := NewTestBenchAPI(tb)

	req := httptest.NewRequest(http.MethodGet, "/api/series/id/0:bogus", nil)
	rec := httptest.NewRecorder()

	api.handleSeriesDataByID(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid aggregation suffix", body["error"])
}
