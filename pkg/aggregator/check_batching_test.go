// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type batchTestBarrier chan struct{}

func (b batchTestBarrier) handle(_ *BufferedAggregator) { close(b) }

type batchTestClock struct {
	*clock.Mock
	scheduled chan struct{}
}

func (c *batchTestClock) Timer(d time.Duration) *clock.Timer {
	timer := c.Mock.Timer(d)
	c.scheduled <- struct{}{}
	return timer
}

func newBatchTestAggregator(t *testing.T, batchSize int) *BufferedAggregator {
	t.Helper()
	cfg := configmock.New(t)
	if batchSize != 0 {
		cfg.SetInTest("check_sampler_batch_sizes", map[string]int{"batch_test": batchSize})
	}
	cfg.SetInTest("aggregator_buffer_size", 2)
	s := &MockSerializerIterableSerie{}
	s.On("SendServiceChecks", mock.Anything).Return(nil)
	agg := NewBufferedAggregator(s, nil, haagentmock.NewMockHaAgent(), nooptagger.NewComponent(), "test", time.Hour, filterlistmock.NewMockFilterList())
	agg.tlmContainerTagsEnabled = false
	clk := &batchTestClock{Mock: clock.NewMock(), scheduled: make(chan struct{}, 1)}
	clk.Set(time.Unix(100, 0))
	agg.batchClock = clk
	return agg
}

// Opting in one check must not change other checks or the disabled control.
func TestCheckBatchScope(t *testing.T) {
	for _, tc := range []struct {
		name        string
		id          checkid.ID
		size        int
		manualFlush bool
	}{
		{"default", "batch_test:test", 0, false},
		{"negative", "batch_test:test", -1, false},
		{"other-check", "postgres:test", 1, false},
		{"unconfigured-sqlserver", "sqlserver:test", 1, false},
		{"manual-flush", "batch_test:test", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agg := newBatchTestAggregator(t, tc.size)
			defer agg.health.Deregister()
			if tc.manualFlush {
				agg.flushInterval = 0
			}
			agg.handleRegisterSampler(tc.id)
			agg.handleSenderSample(senderMetricSample{id: tc.id, metricSample: &metrics.MetricSample{
				Name: "metric", Value: 7, Mtype: metrics.GaugeType,
			}})
			series, _ := agg.GetSeriesAndSketches(time.Now())
			require.Empty(t, series)
			agg.handleSenderSample(senderMetricSample{id: tc.id, commit: true})
			series, _ = agg.GetSeriesAndSketches(time.Now())
			require.Len(t, series, 1)
			require.Equal(t, float64(7), series[0].Points[0].Value)
		})
	}
}

// Splitting a collection must retain counter/rate state, including resets, even
// when unrelated batches expire the original metric's context between samples.
func TestCheckBatchStatefulMetrics(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       metrics.MetricType
		firstValue bool
		want       []float64
	}{
		{"monotonic", metrics.MonotonicCountType, false, []float64{25, 0, 10}},
		{"monotonic-first-value", metrics.MonotonicCountType, true, []float64{100, 25, 20, 10}},
		{"rate", metrics.RateType, false, []float64{5, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agg := newBatchTestAggregator(t, 1)
			defer agg.health.Deregister()
			id := checkid.ID("batch_test:test")
			agg.handleRegisterSampler(id)
			var got []float64
			for n, value := range []float64{100, 125, 20, 30} {
				agg.batchClock.(*batchTestClock).Add(time.Second)
				agg.handleSenderSample(senderMetricSample{id: id, metricSample: &metrics.MetricSample{
					Name: "batch_test.metric", Value: value, Mtype: tc.kind,
					Timestamp: float64(10 + n*5), FlushFirstValue: tc.firstValue,
				}})
				agg.checkSamplers[id].commitBatch(float64(agg.batchClock.Now().Unix()), &agg.flushFilterList)
				series, _ := agg.GetSeriesAndSketches(time.Now())
				for _, s := range series {
					if s.Name == "batch_test.metric" {
						got = append(got, s.Points[0].Value)
					}
				}
				for range 3 {
					agg.batchClock.(*batchTestClock).Add(time.Second)
					agg.handleSenderSample(senderMetricSample{id: id, metricSample: &metrics.MetricSample{
						Name: "batch_test.other", Value: 1, Mtype: metrics.GaugeType,
					}})
					agg.checkSamplers[id].commitBatch(float64(agg.batchClock.Now().Unix()), &agg.flushFilterList)
				}
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// Bucket baselines outlive batch-expired contexts, but still expire after the
// configured number of check runs without a bucket submission.
func TestCheckBatchBucketBaselines(t *testing.T) {
	for _, tc := range []struct {
		multiple, firstValue bool
		expiry               int
	}{
		{false, false, 2}, {false, true, 1},
		{true, false, 1}, {true, true, 2},
	} {
		t.Run(fmt.Sprintf("multiple=%v/first-value=%v", tc.multiple, tc.firstValue), func(t *testing.T) {
			agg := newBatchTestAggregator(t, 1)
			defer agg.health.Deregister()
			id := checkid.ID("batch_test:test")
			agg.handleRegisterSampler(id)
			cs := agg.checkSamplers[id]
			cs.contextResolver.expireCountInterval = int64(tc.expiry)
			clk := agg.batchClock.(*batchTestClock)
			bucketCount, weight := 1, int64(1)
			if tc.multiple {
				bucketCount, weight = 2, 3 // Two bounds contribute value and 2*value.
			}
			commit := func() {
				clk.Add(time.Second)
				agg.checkSamplers[id].commit(float64(agg.batchClock.Now().Unix()), &agg.flushFilterList)
			}
			for run := range 2 {
				for step := range 2 {
					for bound := range bucketCount {
						agg.handleSenderBucket(senderHistogramBucket{id: id, bucket: &metrics.HistogramBucket{
							Name: "batch_test.bucket", Value: int64((100 + 25*(2*run+step)) * (bound + 1)),
							LowerBound: float64(bound * 10), UpperBound: float64((bound + 1) * 10),
							Timestamp: float64(clk.Now().Unix() - 1), Monotonic: true,
							MultipleBuckets: tc.multiple, FlushFirstValue: tc.firstValue,
						}})
					}
					// Enough unrelated batches to expire the bucket's context.
					for n := range tc.expiry + 1 {
						clk.Add(time.Second)
						agg.handleSenderSample(senderMetricSample{id: id, metricSample: &metrics.MetricSample{
							Name: fmt.Sprintf("batch_test.other.%d", n), Value: 1, Mtype: metrics.GaugeType,
						}})
						agg.checkSamplers[id].commitBatch(float64(agg.batchClock.Now().Unix()), &agg.flushFilterList)
					}
					_, found := cs.contextResolver.get(generateContextKey(&metrics.HistogramBucket{Name: "batch_test.bucket"}))
					require.False(t, found)
					require.LessOrEqual(t, cs.contextResolver.length(), tc.expiry)
					_, sketches := agg.GetSeriesAndSketches(clk.Now())
					var count int64
					for _, s := range sketches {
						for _, p := range s.Points {
							count += p.Sketch.Basic.Cnt
						}
					}
					want := 25 * weight
					if run == 0 && step == 0 {
						want = 0
						if tc.firstValue {
							want = 100 * weight
						}
					}
					require.Equal(t, want, count, "run %d, step %d", run, step)
				}
				commit()
			}
			for missed := range tc.expiry {
				require.Equal(t, 1, len(cs.lastBucketValue)+len(cs.lastBucketValueByBound))
				commit()
				if missed == tc.expiry-1 {
					require.Empty(t, cs.lastBucketValue)
					require.Empty(t, cs.lastBucketValueByBound)
					require.Empty(t, cs.bucketLastSeen)
				}
			}
		})
	}
}

// Intermediate commits preserve historate's previous sample, but the end of a
// collection still resets it, including when the last sample filled a batch.
func TestCheckBatchHistorate(t *testing.T) {
	for _, batchSize := range []int{0, 1, 2, 3, 4} {
		t.Run(fmt.Sprintf("batch-size=%d", batchSize), func(t *testing.T) {
			agg := newBatchTestAggregator(t, batchSize)
			defer agg.health.Deregister()
			id := checkid.ID("batch_test:test")
			agg.handleRegisterSampler(id)
			clk := agg.batchClock.(*batchTestClock)
			for run := range 2 {
				for n, value := range []float64{10, 30, 60, 100} {
					clk.Add(time.Second)
					agg.handleSenderSample(senderMetricSample{id: id, metricSample: &metrics.MetricSample{
						Name: "batch_test.historate", Value: value, Mtype: metrics.HistorateType,
						Timestamp: float64(10 + run*10 + n),
					}})
					if batchSize > 0 && (n+1)%batchSize == 0 {
						agg.checkSamplers[id].commitBatch(float64(clk.Now().Unix()), &agg.flushFilterList)
					}
				}
				clk.Add(time.Second)
				agg.checkSamplers[id].commit(float64(agg.batchClock.Now().Unix()), &agg.flushFilterList)
				series, _ := agg.GetSeriesAndSketches(clk.Now())
				counts, averages := map[float64]float64{}, map[float64]float64{}
				for _, s := range series {
					for _, p := range s.Points {
						switch s.Name {
						case "batch_test.historate.count":
							counts[p.Ts] = p.Value
						case "batch_test.historate.avg":
							averages[p.Ts] = p.Value
						}
					}
				}
				var count, sum float64
				for ts, n := range counts {
					require.Contains(t, averages, ts)
					count += n
					sum += n * averages[ts]
				}
				require.Equal(t, float64(3), count, "run %d", run)
				require.Equal(t, float64(90), sum, "run %d", run)
			}
		})
	}
}
