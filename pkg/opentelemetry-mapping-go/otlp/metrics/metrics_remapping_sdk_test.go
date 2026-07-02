// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// sdkTraceMetric builds a delta histogram named like the DD-SDK trace metric with a
// single datapoint carrying the given attributes.
func sdkTraceMetric(unit string, count uint64, sum float64, attrs map[string]string) pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetName(sdkTraceMetricName)
	m.SetUnit(unit)
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := h.DataPoints().AppendEmpty()
	dp.SetCount(count)
	dp.SetSum(sum)
	for k, v := range attrs {
		dp.Attributes().PutStr(k, v)
	}
	return m
}

// seriesByName indexes the remapped output metrics by name, capturing the single
// datapoint value and attributes of each.
type series struct {
	value float64
	tags  map[string]string
}

func remapSDK(m pmetric.Metric) map[string]series {
	out := pmetric.NewMetricSlice()
	remapMetrics(out, m)
	got := map[string]series{}
	for i := 0; i < out.Len(); i++ {
		metric := out.At(i)
		dp := metric.Sum().DataPoints().At(0)
		attrs := dp.Attributes()
		s := series{value: dp.DoubleValue(), tags: map[string]string{}}
		for _, key := range []string{"resource", "span.kind", "http.status_code", "span.type", "origin"} {
			if val, ok := attrs.Get(key); ok {
				s.tags[key] = val.AsString()
			}
		}
		got[metric.Name()] = s
	}
	return got
}

func TestRemapSDKTraceMetric_DefaultMode(t *testing.T) {
	m := sdkTraceMetric("s", 5, 2.0, map[string]string{
		"datadog.operation.name": "http.request",
		"datadog.span.type":      "web",
		"datadog.span.top_level": "true",
		"datadog.origin":         "synthetics",
		"span.name":              "users.lookup",
		"span.kind":              "SERVER",
		"status.code":            "STATUS_CODE_ERROR",
	})
	got := remapSDK(m)

	require.Contains(t, got, "trace.http.request.hits")
	require.Contains(t, got, "trace.http.request.errors")
	require.Contains(t, got, "trace.http.request.duration")

	assert.Equal(t, float64(5), got["trace.http.request.hits"].value)
	assert.Equal(t, float64(5), got["trace.http.request.errors"].value)
	assert.Equal(t, float64(5), got["trace.http.request.hits.by_type"].value)
	// 2s scaled to nanoseconds.
	assert.Equal(t, 2.0*1e9, got["trace.http.request.duration"].value)

	tags := got["trace.http.request.hits"].tags
	assert.Equal(t, "users.lookup", tags["resource"])
	assert.Equal(t, "Server", tags["span.kind"])
	assert.Equal(t, "web", tags["span.type"])
	assert.Equal(t, "synthetics", tags["origin"])
	// Non-HTTP span: http.status_code left unset.
	assert.NotContains(t, tags, "http.status_code")
}

func TestRemapSDKTraceMetric_NotTopLevel(t *testing.T) {
	m := sdkTraceMetric("s", 3, 1.0, map[string]string{
		"datadog.operation.name": "op",
		"span.name":              "res",
	})
	got := remapSDK(m)
	assert.Contains(t, got, "trace.op.hits")
	assert.NotContains(t, got, "trace.op.hits.by_type")
	assert.NotContains(t, got, "trace.op.errors")
}

func TestRemapSDKTraceMetric_ErrorGating(t *testing.T) {
	for _, tc := range []struct {
		name    string
		attrs   map[string]string
		wantErr bool
	}{
		{"status_code_full", map[string]string{"status.code": "STATUS_CODE_ERROR"}, true},
		{"status_code_short", map[string]string{"status.code": "ERROR"}, true},
		{"status_code_int", map[string]string{"status.code": "2"}, true},
		{"status_code_ok", map[string]string{"status.code": "OK"}, false},
		{"error_type_ignored", map[string]string{"error.type": "sql.timeout"}, false},
		{"no_status", map[string]string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attrs := map[string]string{"datadog.operation.name": "op"}
			for k, v := range tc.attrs {
				attrs[k] = v
			}
			got := remapSDK(sdkTraceMetric("s", 4, 1.0, attrs))
			if tc.wantErr {
				require.Contains(t, got, "trace.op.errors")
				assert.Equal(t, float64(4), got["trace.op.errors"].value)
			} else {
				assert.NotContains(t, got, "trace.op.errors")
			}
		})
	}
}

func TestRemapSDKTraceMetric_HTTPStatusSet(t *testing.T) {
	m := sdkTraceMetric("ms", 1, 500.0, map[string]string{
		"datadog.operation.name": "http.server.request",
		"span.kind":              "SERVER",
	})
	m.Histogram().DataPoints().At(0).Attributes().PutInt("http.response.status_code", 200)
	got := remapSDK(m)
	assert.Equal(t, "200", got["trace.http.server.request.hits"].tags["http.status_code"])
	// 500ms scaled to nanoseconds.
	assert.Equal(t, 500.0*1e6, got["trace.http.server.request.duration"].value)
}

func TestRemapSDKTraceMetric_OTelSemanticsFallback(t *testing.T) {
	// No datadog.operation.name: operation resolved from semconv.
	m := sdkTraceMetric("s", 2, 1.0, map[string]string{
		"span.name":           "GET /users/:id",
		"span.kind":           "SERVER",
		"http.request.method": "GET",
	})
	got := remapSDK(m)
	require.Contains(t, got, "trace.http.server.request.hits")
	assert.Equal(t, "GET /users/:id", got["trace.http.server.request.hits"].tags["resource"])
}

// The SDK trace metric must never be renamed (prefixed) by renameMetrics.
func TestRenameMetrics_SDKTraceMetricUnchanged(t *testing.T) {
	m := sdkTraceMetric("s", 1, 1.0, nil)
	renameMetrics(m)
	assert.Equal(t, sdkTraceMetricName, m.Name())
}
