// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// metricCall records a single metric submission made by a transformer.
type metricCall struct {
	method   string
	name     string
	value    float64
	hostname string
	tags     []string
}

// bucketCall records a single OpenmetricsBucket or HistogramBucket submission.
type bucketCall struct {
	name            string
	value           int64
	lowerBound      float64
	upperBound      float64
	monotonic       bool
	hostname        string
	tags            []string
	flushFirstValue bool
}

// serviceCheckCall records a single ServiceCheck submission.
type serviceCheckCall struct {
	name     string
	status   servicecheck.ServiceCheckStatus
	hostname string
	tags     []string
	message  string
}

// recordingSender is a lightweight mock that implements sender.Sender and
// records metric submissions. This avoids pulling in the full
// mocksender/aggregator dependency tree which requires the "test" build tag.
type recordingSender struct {
	calls         []metricCall
	bucketCalls   []bucketCall
	serviceChecks []serviceCheckCall
	committed     bool
}

func (s *recordingSender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"Gauge", metric, value, hostname, tags})
}

func (s *recordingSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"GaugeNoIndex", metric, value, hostname, tags})
}

func (s *recordingSender) Rate(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"Rate", metric, value, hostname, tags})
}

func (s *recordingSender) Count(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"Count", metric, value, hostname, tags})
}

func (s *recordingSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"MonotonicCount", metric, value, hostname, tags})
}

func (s *recordingSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	s.calls = append(s.calls, metricCall{"MonotonicCountWithFlushFirstValue", metric, value, hostname, tags})
}

func (s *recordingSender) Counter(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"Counter", metric, value, hostname, tags})
}

func (s *recordingSender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"Histogram", metric, value, hostname, tags})
}

func (s *recordingSender) Historate(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"Historate", metric, value, hostname, tags})
}

func (s *recordingSender) Distribution(metric string, value float64, hostname string, tags []string) {
	s.calls = append(s.calls, metricCall{"Distribution", metric, value, hostname, tags})
}

func (s *recordingSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, _ float64) error {
	s.calls = append(s.calls, metricCall{"GaugeWithTimestamp", metric, value, hostname, tags})
	return nil
}

func (s *recordingSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, _ float64) error {
	s.calls = append(s.calls, metricCall{"CountWithTimestamp", metric, value, hostname, tags})
	return nil
}

func (s *recordingSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
	s.serviceChecks = append(s.serviceChecks, serviceCheckCall{checkName, status, hostname, tags, message})
}

func (s *recordingSender) OpenmetricsBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	s.bucketCalls = append(s.bucketCalls, bucketCall{metric, value, lowerBound, upperBound, monotonic, hostname, tags, flushFirstValue})
}

func (s *recordingSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	s.bucketCalls = append(s.bucketCalls, bucketCall{metric, value, lowerBound, upperBound, monotonic, hostname, tags, flushFirstValue})
}

func (s *recordingSender) Event(_ event.Event)                                        {}
func (s *recordingSender) EventPlatformEvent(_ []byte, _ string)                      {}
func (s *recordingSender) Commit()                                                    { s.committed = true }
func (s *recordingSender) DisableDefaultHostname(_ bool)                              {}
func (s *recordingSender) SetCheckCustomTags(_ []string)                              {}
func (s *recordingSender) SetCheckService(_ string)                                   {}
func (s *recordingSender) SetNoIndex(_ bool)                                          {}
func (s *recordingSender) FinalizeCheckServiceTag()                                   {}
func (s *recordingSender) GetSenderStats() stats.SenderStats                          { return stats.SenderStats{} }
func (s *recordingSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}
func (s *recordingSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string)        {}

// findCalls returns all metricCalls matching the given method and metric name.
func findCalls(calls []metricCall, method, name string) []metricCall {
	var result []metricCall
	for _, c := range calls {
		if c.method == method && c.name == name {
			result = append(result, c)
		}
	}
	return result
}

// --- Transformer tests ---

func TestGaugeTransformer(t *testing.T) {
	sndr := &recordingSender{}
	tf := newGaugeTransformer()

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "my_gauge"},
				Value:  42.5,
			},
			Tags:     []string{"env:prod"},
			Hostname: "host1",
		},
	}

	tf("test.my_gauge", samples, sndr, false)

	assert.Len(t, sndr.calls, 1)
	assert.Equal(t, "Gauge", sndr.calls[0].method)
	assert.Equal(t, "test.my_gauge", sndr.calls[0].name)
	assert.Equal(t, 42.5, sndr.calls[0].value)
	assert.Equal(t, "host1", sndr.calls[0].hostname)
	assert.Equal(t, []string{"env:prod"}, sndr.calls[0].tags)
}

func TestCounterTransformer(t *testing.T) {
	sndr := &recordingSender{}
	tf := newCounterTransformer()

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "http_requests_total"},
				Value:  100,
			},
			Tags:     []string{"method:GET"},
			Hostname: "",
		},
	}

	tf("test.http_requests_total", samples, sndr, false)

	assert.Len(t, sndr.calls, 1)
	assert.Equal(t, "MonotonicCount", sndr.calls[0].method)
	assert.Equal(t, "test.http_requests_total.count", sndr.calls[0].name)
	assert.Equal(t, 100.0, sndr.calls[0].value)
	assert.Equal(t, []string{"method:GET"}, sndr.calls[0].tags)
}

func TestRateTransformer(t *testing.T) {
	sndr := &recordingSender{}
	tf := newRateTransformer()

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "cpu_seconds"},
				Value:  3.14,
			},
			Tags:     []string{"core:0"},
			Hostname: "host2",
		},
	}

	tf("test.cpu_seconds", samples, sndr, false)

	assert.Len(t, sndr.calls, 1)
	assert.Equal(t, "Rate", sndr.calls[0].method)
	assert.Equal(t, "test.cpu_seconds", sndr.calls[0].name)
	assert.Equal(t, 3.14, sndr.calls[0].value)
	assert.Equal(t, "host2", sndr.calls[0].hostname)
	assert.Equal(t, []string{"core:0"}, sndr.calls[0].tags)
}

func TestCounterGaugeTransformer(t *testing.T) {
	sndr := &recordingSender{}
	tf := newCounterGaugeTransformer()

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "events_processed"},
				Value:  500,
			},
			Tags:     []string{"queue:main"},
			Hostname: "",
		},
	}

	tf("test.events_processed", samples, sndr, false)

	assert.Len(t, sndr.calls, 2)

	gauges := findCalls(sndr.calls, "Gauge", "test.events_processed.total")
	assert.Len(t, gauges, 1)
	assert.Equal(t, 500.0, gauges[0].value)

	monotonics := findCalls(sndr.calls, "MonotonicCount", "test.events_processed.count")
	assert.Len(t, monotonics, 1)
	assert.Equal(t, 500.0, monotonics[0].value)
}

func TestSummaryTransformer(t *testing.T) {
	sndr := &recordingSender{}
	tf := newSummaryTransformer(false)

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "rpc_duration_seconds_sum"},
				Value:  17560.473,
			},
			Tags: []string{"service:web"},
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "rpc_duration_seconds_count"},
				Value:  2693,
			},
			Tags: []string{"service:web"},
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "rpc_duration_seconds", "quantile": "0.5"},
				Value:  4773,
			},
			Tags: []string{"service:web", "quantile:0.5"},
		},
	}

	tf("test.rpc_duration_seconds", samples, sndr, false)

	assert.Len(t, sndr.calls, 3)

	sums := findCalls(sndr.calls, "MonotonicCount", "test.rpc_duration_seconds.sum")
	assert.Len(t, sums, 1)
	assert.Equal(t, 17560.473, sums[0].value)

	counts := findCalls(sndr.calls, "MonotonicCount", "test.rpc_duration_seconds.count")
	assert.Len(t, counts, 1)
	assert.Equal(t, 2693.0, counts[0].value)

	quantiles := findCalls(sndr.calls, "Gauge", "test.rpc_duration_seconds.quantile")
	assert.Len(t, quantiles, 1)
	assert.Equal(t, 4773.0, quantiles[0].value)
}

func TestNaNAndInfValuesAreSkipped(t *testing.T) {
	sndr := &recordingSender{}
	tf := newGaugeTransformer()

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "m1"},
				Value:  math.NaN(),
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "m2"},
				Value:  math.Inf(1),
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "m3"},
				Value:  math.Inf(-1),
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "m4"},
				Value:  99,
			},
			Tags: nil,
		},
	}

	tf("test.metric", samples, sndr, false)

	assert.Len(t, sndr.calls, 1, "only the finite value should be submitted")
	assert.Equal(t, 99.0, sndr.calls[0].value)
}

func TestHistogramPath4DefaultCumulative(t *testing.T) {
	// Path 4: CollectHistogramBuckets=true, no distributions, not non-cumulative.
	// _sum and _count submitted as MonotonicCount.
	// _bucket submitted as MonotonicCount with ".bucket" suffix, +Inf bucket skipped.
	sndr := &recordingSender{}
	tf := newHistogramTransformer(HistogramOptions{
		CollectHistogramBuckets: true,
	})

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "http_request_duration_seconds_sum"},
				Value:  53423,
			},
			Tags: []string{"handler:api"},
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "http_request_duration_seconds_count"},
				Value:  144320,
			},
			Tags: []string{"handler:api"},
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "http_request_duration_seconds_bucket", "le": "0.05"},
				Value:  24054,
			},
			Tags: []string{"handler:api", "upper_bound:0.05"},
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "http_request_duration_seconds_bucket", "le": "0.1"},
				Value:  33444,
			},
			Tags: []string{"handler:api", "upper_bound:0.1"},
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "http_request_duration_seconds_bucket", "le": "+Inf"},
				Value:  144320,
			},
			Tags: []string{"handler:api", "upper_bound:+Inf"},
		},
	}

	tf("test.http_request_duration_seconds", samples, sndr, false)

	// _sum and _count
	sums := findCalls(sndr.calls, "MonotonicCount", "test.http_request_duration_seconds.sum")
	assert.Len(t, sums, 1)
	assert.Equal(t, 53423.0, sums[0].value)

	counts := findCalls(sndr.calls, "MonotonicCount", "test.http_request_duration_seconds.count")
	assert.Len(t, counts, 1)
	assert.Equal(t, 144320.0, counts[0].value)

	// Two finite buckets, +Inf is skipped.
	buckets := findCalls(sndr.calls, "MonotonicCount", "test.http_request_duration_seconds.bucket")
	assert.Len(t, buckets, 2, "+Inf bucket should be skipped")
	assert.Equal(t, 24054.0, buckets[0].value)
	assert.Equal(t, 33444.0, buckets[1].value)
}

func TestHistogramPath5NoBuckets(t *testing.T) {
	// Path 5: CollectHistogramBuckets=false -- only _sum and _count are submitted.
	sndr := &recordingSender{}
	tf := newHistogramTransformer(HistogramOptions{
		CollectHistogramBuckets: false,
	})

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "req_duration_sum"},
				Value:  100,
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "req_duration_count"},
				Value:  10,
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "req_duration_bucket", "le": "0.5"},
				Value:  8,
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "req_duration_bucket", "le": "+Inf"},
				Value:  10,
			},
			Tags: nil,
		},
	}

	tf("test.req_duration", samples, sndr, false)

	// Only _sum and _count should be submitted.
	assert.Len(t, sndr.calls, 2)

	sums := findCalls(sndr.calls, "MonotonicCount", "test.req_duration.sum")
	assert.Len(t, sums, 1)
	assert.Equal(t, 100.0, sums[0].value)

	counts := findCalls(sndr.calls, "MonotonicCount", "test.req_duration.count")
	assert.Len(t, counts, 1)
	assert.Equal(t, 10.0, counts[0].value)

	// No bucket calls at all.
	buckets := findCalls(sndr.calls, "MonotonicCount", "test.req_duration.bucket")
	assert.Len(t, buckets, 0, "buckets should not be submitted when CollectHistogramBuckets is false")
}

func TestDecumulateHistogramBuckets(t *testing.T) {
	// Input: cumulative buckets for a single histogram series.
	// le=0.1 -> 10, le=0.5 -> 30, le=1.0 -> 50, le=+Inf -> 60
	// Expected deltas: 10, 20, 20, 10
	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "0.1", "job": "api"},
				Value:  10,
			},
			Tags:     []string{"job:api", "upper_bound:0.1"},
			Hostname: "",
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "0.5", "job": "api"},
				Value:  30,
			},
			Tags:     []string{"job:api", "upper_bound:0.5"},
			Hostname: "",
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "1.0", "job": "api"},
				Value:  50,
			},
			Tags:     []string{"job:api", "upper_bound:1.0"},
			Hostname: "",
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "+Inf", "job": "api"},
				Value:  60,
			},
			Tags:     []string{"job:api", "upper_bound:+Inf"},
			Hostname: "",
		},
	}

	result := decumulateHistogramBuckets(samples)

	// All 4 buckets should be present (no non-bucket samples here).
	assert.Len(t, result, 4)

	expectedValues := []float64{10, 20, 20, 10}
	expectedLowerBounds := []string{"-Inf", "0.1", "0.5", "1.0"}
	expectedUpperBounds := []string{"0.1", "0.5", "1.0", "+Inf"}

	for i, sd := range result {
		assert.Equal(t, expectedValues[i], sd.Sample.Value, "bucket %d delta value", i)
		assert.Equal(t, expectedLowerBounds[i], sd.Sample.Metric["lower_bound"], "bucket %d lower_bound", i)
		assert.Equal(t, expectedUpperBounds[i], getUpperBound(sd.Sample.Metric), "bucket %d upper_bound", i)
	}
}

func TestDecumulateHistogramBucketsPreservesNonBuckets(t *testing.T) {
	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_sum"},
				Value:  500,
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_count"},
				Value:  100,
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "1.0"},
				Value:  80,
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "+Inf"},
				Value:  100,
			},
			Tags: nil,
		},
	}

	result := decumulateHistogramBuckets(samples)

	// 2 non-bucket + 2 decumulated bucket samples.
	assert.Len(t, result, 4)

	// Non-bucket samples come first and are unchanged.
	assert.Equal(t, 500.0, result[0].Sample.Value)
	assert.Equal(t, "h_sum", result[0].Sample.Metric["__name__"])
	assert.Equal(t, 100.0, result[1].Sample.Value)
	assert.Equal(t, "h_count", result[1].Sample.Metric["__name__"])

	// Decumulated buckets.
	assert.Equal(t, 80.0, result[2].Sample.Value)  // first bucket, no subtraction
	assert.Equal(t, 20.0, result[3].Sample.Value)  // 100 - 80
}

func TestDecumulateHistogramBucketsDoesNotMutateOriginals(t *testing.T) {
	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "1.0"},
				Value:  50,
			},
			Tags: nil,
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "h_bucket", "le": "5.0"},
				Value:  80,
			},
			Tags: nil,
		},
	}

	// Capture original values.
	origVal0 := samples[0].Sample.Value
	origVal1 := samples[1].Sample.Value

	_ = decumulateHistogramBuckets(samples)

	// Original samples must not be mutated.
	assert.Equal(t, origVal0, samples[0].Sample.Value, "original sample 0 should be unchanged")
	assert.Equal(t, origVal1, samples[1].Sample.Value, "original sample 1 should be unchanged")

	// Original metrics should not have lower_bound added.
	_, hasLB0 := samples[0].Sample.Metric["lower_bound"]
	assert.False(t, hasLB0, "original sample 0 should not have lower_bound")
}

func TestGaugeTransformerMultipleSamples(t *testing.T) {
	sndr := &recordingSender{}
	tf := newGaugeTransformer()

	samples := []SampleData{
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "temp", "zone": "us-east"},
				Value:  72.1,
			},
			Tags:     []string{"zone:us-east"},
			Hostname: "",
		},
		{
			Sample: &prometheus.Sample{
				Metric: prometheus.Metric{"__name__": "temp", "zone": "eu-west"},
				Value:  68.3,
			},
			Tags:     []string{"zone:eu-west"},
			Hostname: "",
		},
	}

	tf("test.temp", samples, sndr, false)

	assert.Len(t, sndr.calls, 2)
	assert.Equal(t, 72.1, sndr.calls[0].value)
	assert.Equal(t, 68.3, sndr.calls[1].value)
}
