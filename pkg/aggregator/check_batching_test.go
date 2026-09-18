// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"sync"
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

func (c *batchTestClock) After(d time.Duration) <-chan time.Time {
	timer := c.Mock.After(d)
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

// A producer must stop at a full batch, then resume without losing metrics when
// the downstream flush drains; otherwise batching merely relocates the backlog.
func TestCheckBatchBackpressure(t *testing.T) {
	agg := newBatchTestAggregator(t, 4)
	id := checkid.ID("batch_test:test")
	agg.handleRegisterSampler(id)
	go agg.run()
	defer agg.Stop()
	send := func(n int) {
		agg.checkItems <- &senderMetricSample{id: id, metricSample: &metrics.MetricSample{
			Name: fmt.Sprintf("batch_test.batch.%d", n), Value: float64(n), Mtype: metrics.GaugeType,
		}}
	}
	for n := 0; n < 4; n++ {
		send(n)
	}
	select {
	case <-agg.batchFlushRequested:
	case <-time.After(5 * time.Second):
		t.Fatal("full batch did not request a flush")
	}
	send(4)
	send(5)
	draining := make(chan struct{})
	release := make(chan struct{})
	var started, released sync.Once
	defer released.Do(func() { close(release) })
	output := make(chan *metrics.Serie, 16)
	trigger := testNewFlushTrigger(time.Now(), true, func(s *metrics.Serie) {
		started.Do(func() { close(draining) })
		<-release
		output <- s
	})
	agg.flushChan <- trigger
	<-draining
	select {
	case agg.checkItems <- &senderMetricSample{id: id, commit: true}:
		t.Fatal("producer advanced beyond the bounded queue while the flush was blocked")
	default:
	}
	released.Do(func() { close(release) })
	produced := make(chan struct{})
	go func() {
		send(6)
		close(produced)
	}()
	select {
	case <-produced:
	case <-time.After(5 * time.Second):
		t.Fatal("producer did not resume after drain")
	}
	agg.batchClock.(*batchTestClock).Add(time.Second)
	agg.checkItems <- &senderMetricSample{id: id, commit: true}
	committed := make(batchTestBarrier)
	agg.checkItems <- committed
	<-committed
	var remainder metrics.Series
	var sketches metrics.SketchSeriesList
	agg.getSeriesAndSketches(time.Now(), &remainder, &sketches)
	require.Len(t, remainder, 3)
	seen := map[string]float64{}
	for len(output) > 0 {
		s := <-output
		seen[s.Name] = s.Points[0].Value
	}
	for _, s := range remainder {
		seen[s.Name] = s.Points[0].Value
	}
	for n := 0; n < 7; n++ {
		require.Contains(t, seen, fmt.Sprintf("batch_test.batch.%d", n))
		require.Equal(t, float64(n), seen[fmt.Sprintf("batch_test.batch.%d", n)])
	}
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

// A full input queue must not keep an aggregator waiting for a batch flush from
// accepting shutdown. No extra flush-loop lifetime is required to stop it.
func TestCheckBatchStopPending(t *testing.T) {
	for _, waitingForTimestamp := range []bool{false, true} {
		t.Run(fmt.Sprintf("timestamp-wait=%v", waitingForTimestamp), func(t *testing.T) {
			agg := newBatchTestAggregator(t, 1)
			id := checkid.ID("batch_test:test")
			agg.handleRegisterSampler(id)
			clk := agg.batchClock.(*batchTestClock)
			if waitingForTimestamp {
				agg.checkSamplers[id].nextBatchCommit = clk.Now().Add(time.Second)
			}
			go agg.run()
			agg.checkItems <- &senderMetricSample{id: id, metricSample: &metrics.MetricSample{
				Name: "batch_test.gauge", Value: 1, Mtype: metrics.GaugeType,
			}}
			if waitingForTimestamp {
				<-clk.scheduled
			} else {
				<-agg.batchFlushRequested
			}
			for range cap(agg.checkItems) {
				agg.checkItems <- &senderMetricSample{id: id, commit: true}
			}
			done := make(chan struct{})
			go func() { agg.Stop(); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown waited for a batch flush")
			}
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
				}
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// Rapid batches and the final remainder must survive whole-second serialization
// without duplicate timestamps that would overwrite count contributions at intake.
func TestCheckBatchTimestamps(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   metrics.MetricType
		suffix string
		want   []float64
	}{
		{"count", metrics.CountType, "", []float64{7, 11, 7}},
		{"monotonic", metrics.MonotonicCountType, "", []float64{4, 2, 1}},
		{"histogram-count", metrics.HistogramType, ".count", []float64{2, 2, 1}},
		{"gauge", metrics.GaugeType, "", []float64{4, 6, 7}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agg := newBatchTestAggregator(t, 2)
			clk := agg.batchClock.(*batchTestClock)
			id := checkid.ID("batch_test:test")
			agg.handleRegisterSampler(id)
			go agg.run()
			defer agg.Stop()
			var series metrics.Series
			flush := func() {
				var sketches metrics.SketchSeriesList
				done := make(chan struct{})
				agg.flushChan <- flushTrigger{trigger: trigger{time: clk.Now(), blockChan: done}, seriesSink: &series, sketchesSink: &sketches}
				<-done
			}
			send := func(value float64) {
				agg.checkItems <- &senderMetricSample{id: id, metricSample: &metrics.MetricSample{
					Name: "batch_test.repeated", Value: value, Mtype: tc.kind, FlushFirstValue: true,
				}}
			}
			send(3)
			send(4)
			<-agg.batchFlushRequested
			flush()
			send(5)
			send(6)
			<-clk.scheduled
			// A regular flush must neither commit the waiting batch nor resume input.
			flush()
			require.NotEmpty(t, series)
			for _, s := range series {
				require.Equal(t, float64(100), s.Points[0].Ts)
			}
			send(7)
			agg.checkItems <- &senderMetricSample{id: id, commit: true}
			clk.Add(time.Second)
			<-agg.batchFlushRequested
			flush()
			<-clk.scheduled // The final partial batch also needs a distinct timestamp.
			clk.Add(time.Second)
			committed := make(batchTestBarrier)
			agg.checkItems <- committed
			<-committed
			flush()

			var timestamps []float64
			var values []float64
			for _, serie := range series {
				if serie.Name == "batch_test.repeated"+tc.suffix {
					for _, point := range serie.Points {
						timestamps = append(timestamps, point.Ts)
						values = append(values, point.Value)
					}
				}
			}
			require.Equal(t, []float64{100, 101, 102}, timestamps)
			require.Equal(t, tc.want, values)
		})
	}
}
