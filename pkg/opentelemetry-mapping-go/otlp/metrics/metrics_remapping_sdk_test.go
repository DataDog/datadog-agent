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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// sdkTraceMetric builds a delta histogram named like the DD-SDK trace metric with a
// single datapoint carrying the given attributes. It populates explicit buckets so
// the duration DDSketch can be constructed from the distribution.
func sdkTraceMetric(unit string, count uint64, sum float64, attrs map[string]string) pmetric.Metric {
	m := pmetric.NewMetric()
	m.SetName(sdkTraceMetricName)
	m.SetUnit(unit)
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := h.DataPoints().AppendEmpty()
	dp.SetCount(count)
	dp.SetSum(sum)
	dp.SetMin(0)
	dp.SetMax(sum)
	// Single (-inf, +inf) bucket holding the whole population so the sketch has data.
	dp.BucketCounts().FromRaw([]uint64{count})
	for k, v := range attrs {
		dp.Attributes().PutStr(k, v)
	}
	return m
}

// series indexes a remapped Sum output metric by name, capturing the single
// datapoint value and attributes.
type series struct {
	value float64
	tags  map[string]string
}

// remapSDKResult holds the remapped delta Sum series plus the durations captured
// as sketches through the Consumer.
type remapSDKResult struct {
	sums      map[string]series
	durations map[string]*Dimensions
}

func remapSDK(t testing.TB, m pmetric.Metric) remapSDKResult {
	t.Helper()
	out := pmetric.NewMetricSlice()
	consumer := newTestConsumer()
	baseDims := &Dimensions{name: m.Name()}
	remapSDKTraceMetrics(context.Background(), zap.NewNop(), &consumer, baseDims, out, m)

	res := remapSDKResult{sums: map[string]series{}, durations: map[string]*Dimensions{}}
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
		res.sums[metric.Name()] = s
	}
	for i := range consumer.data.Metrics.Sketches {
		sk := consumer.data.Metrics.Sketches[i]
		res.durations[sk.Name] = &Dimensions{name: sk.Name, tags: sk.Tags}
	}
	return res
}

func (r remapSDKResult) durationTags(name string) map[string]string {
	dims, ok := r.durations[name]
	if !ok {
		return nil
	}
	tags := map[string]string{}
	for _, tag := range dims.tags {
		for i := 0; i < len(tag); i++ {
			if tag[i] == ':' {
				tags[tag[:i]] = tag[i+1:]
				break
			}
		}
	}
	return tags
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
	got := remapSDK(t, m)

	require.Contains(t, got.sums, "trace.http.request.hits")
	require.Contains(t, got.sums, "trace.http.request.errors")
	// Duration is emitted as a sketch, not a Sum series.
	require.NotContains(t, got.sums, "trace.http.request.duration")
	require.Contains(t, got.durations, "trace.http.request.duration")

	assert.Equal(t, float64(5), got.sums["trace.http.request.hits"].value)
	assert.Equal(t, float64(5), got.sums["trace.http.request.errors"].value)
	assert.Equal(t, float64(5), got.sums["trace.http.request.hits.by_type"].value)

	tags := got.sums["trace.http.request.hits"].tags
	assert.Equal(t, "users.lookup", tags["resource"])
	// span.kind is lowercased for Datadog APM.
	assert.Equal(t, "server", tags["span.kind"])
	assert.Equal(t, "web", tags["span.type"])
	assert.Equal(t, "synthetics", tags["origin"])
	// Non-HTTP span: http.status_code left unset.
	assert.NotContains(t, tags, "http.status_code")

	// The duration sketch carries the same identifying tags.
	assert.Equal(t, "server", got.durationTags("trace.http.request.duration")["span.kind"])
}

func TestRemapSDKTraceMetric_NotTopLevel(t *testing.T) {
	m := sdkTraceMetric("s", 3, 1.0, map[string]string{
		"datadog.operation.name": "op",
		"span.name":              "res",
	})
	got := remapSDK(t, m)
	assert.Contains(t, got.sums, "trace.op.hits")
	assert.NotContains(t, got.sums, "trace.op.hits.by_type")
	assert.NotContains(t, got.sums, "trace.op.errors")
	assert.Contains(t, got.durations, "trace.op.duration")
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
			got := remapSDK(t, sdkTraceMetric("s", 4, 1.0, attrs))
			if tc.wantErr {
				require.Contains(t, got.sums, "trace.op.errors")
				assert.Equal(t, float64(4), got.sums["trace.op.errors"].value)
			} else {
				assert.NotContains(t, got.sums, "trace.op.errors")
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
	got := remapSDK(t, m)
	assert.Equal(t, "200", got.sums["trace.http.server.request.hits"].tags["http.status_code"])
	// Duration is a sketch carrying the http.status_code tag.
	require.Contains(t, got.durations, "trace.http.server.request.duration")
	assert.Equal(t, "200", got.durationTags("trace.http.server.request.duration")["http.status_code"])
}

func TestRemapSDKTraceMetric_OTelSemanticsFallback(t *testing.T) {
	// No datadog.operation.name: operation resolved from semconv.
	m := sdkTraceMetric("s", 2, 1.0, map[string]string{
		"span.name":           "GET /users/:id",
		"span.kind":           "SERVER",
		"http.request.method": "GET",
	})
	got := remapSDK(t, m)
	require.Contains(t, got.sums, "trace.http.server.request.hits")
	assert.Equal(t, "GET /users/:id", got.sums["trace.http.server.request.hits"].tags["resource"])
}

// TestRemapSDKTraceMetric_SpanKindCasing verifies every span kind is lowercased.
func TestRemapSDKTraceMetric_SpanKindCasing(t *testing.T) {
	for in, want := range map[string]string{
		"SERVER":   "server",
		"CLIENT":   "client",
		"PRODUCER": "producer",
		"CONSUMER": "consumer",
		"INTERNAL": "internal",
		"":         "unspecified",
	} {
		got := remapSDK(t, sdkTraceMetric("s", 1, 1.0, map[string]string{
			"datadog.operation.name": "op",
			"span.kind":              in,
		}))
		assert.Equal(t, want, got.sums["trace.op.hits"].tags["span.kind"], "input %q", in)
	}
}

// The SDK trace metric must never be prefixed by renameMetrics: it matches none
// of the host/kafka/internal rename rules.
func TestRenameMetrics_SDKTraceMetricUnchanged(t *testing.T) {
	m := sdkTraceMetric("s", 1, 1.0, nil)
	renameMetrics(m)
	assert.Equal(t, sdkTraceMetricName, m.Name())
}

// TestSDKTraceMetric_DurationIsSketch verifies the end-to-end translator path:
// duration is emitted as a DDSketch (not a Sum timeseries) and the counts remain
// delta Sum series.
func TestSDKTraceMetric_DurationIsSketch(t *testing.T) {
	translator := NewTestTranslator(t, WithRemapping())

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sdkTraceMetric("s", 3, 1.5, map[string]string{
		"datadog.operation.name": "http.request",
		"span.name":              "checkout",
	}).CopyTo(sm.Metrics().AppendEmpty())

	consumer := newTestConsumer()
	_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
	require.NoError(t, err)

	require.Len(t, consumer.data.Metrics.Sketches, 1, "duration must be a single DDSketch series")
	assert.Equal(t, "trace.http.request.duration", consumer.data.Metrics.Sketches[0].Name)

	var names []string
	for _, ts := range consumer.data.Metrics.TimeSeries {
		names = append(names, ts.Name)
		assert.NotEqual(t, "trace.http.request.duration", ts.Name, "duration must not be a timeseries")
	}
	assert.Contains(t, names, "trace.http.request.hits")
}

// TestSDKTraceMetric_NotBillableHost verifies that a payload containing only the
// SDK trace metric does not mark the host as billable (no ConsumeHost call).
func TestSDKTraceMetric_NotBillableHost(t *testing.T) {
	translator := NewTestTranslator(t, WithRemapping())

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("host.name", "my-host")
	sm := rm.ScopeMetrics().AppendEmpty()
	sdkTraceMetric("s", 1, 1.0, map[string]string{
		"datadog.operation.name": "op",
		"span.name":              "res",
	}).CopyTo(sm.Metrics().AppendEmpty())

	consumer := newTestConsumer()
	_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
	require.NoError(t, err)

	assert.Empty(t, consumer.data.Hosts, "SDK-trace-only payload must not consume a billable host")
}
