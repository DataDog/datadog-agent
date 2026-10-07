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

	"github.com/stretchr/testify/require"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func waitBatchTest[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("batch operation did not complete")
		var zero T
		return zero
	}
}

func batchTestCall(f func()) <-chan struct{} {
	done := make(chan struct{})
	go func() { defer close(done); f() }()
	return done
}

func startBatchTest(t *testing.T, size int) (*BufferedAggregator, *senders, func()) {
	t.Helper()
	agg := newBatchTestAggregator(t, size)
	go agg.run()
	var once sync.Once
	stop := func() { once.Do(agg.Stop) }
	t.Cleanup(stop)
	return agg, newSenders(agg), stop
}

func flushBatchTest(t *testing.T, agg *BufferedAggregator) metrics.Series {
	t.Helper()
	var series metrics.Series
	var sketches metrics.SketchSeriesList
	done := make(chan struct{})
	agg.flushChan <- flushTrigger{
		trigger:    trigger{time: agg.batchClock.Now(), blockChan: done},
		seriesSink: &series, sketchesSink: &sketches,
	}
	waitBatchTest(t, done)
	return series
}

func TestCheckBatchBackpressure(t *testing.T) {
	agg, manager, _ := startBatchTest(t, 4)
	s, err := manager.GetSender("batch_test:test")
	require.NoError(t, err)
	done := batchTestCall(func() {
		for n := range 7 {
			s.Gauge(fmt.Sprintf("batch_test.metric.%d", n), float64(n), "", nil)
		}
		s.Commit()
	})
	waitBatchTest(t, agg.batchFlushRequested)

	draining, release := make(chan struct{}), make(chan struct{})
	var started, released sync.Once
	t.Cleanup(func() { released.Do(func() { close(release) }) })
	var series metrics.Series
	trigger := testNewFlushTrigger(agg.batchClock.Now(), false, func(s *metrics.Serie) {
		started.Do(func() { close(draining) })
		<-release
		series = append(series, s)
	})
	trigger.blockChan = make(chan struct{})
	agg.flushChan <- trigger
	waitBatchTest(t, draining)
	select {
	case <-done:
		t.Fatal("producer escaped backpressure before the batch drained")
	default:
	}
	released.Do(func() { close(release) })
	waitBatchTest(t, trigger.blockChan)
	clk := agg.batchClock.(*batchTestClock)
	waitBatchTest(t, clk.scheduled)
	clk.Add(time.Second)
	waitBatchTest(t, agg.batchFlushRequested)
	series = append(series, flushBatchTest(t, agg)...)
	waitBatchTest(t, done)
	seen := map[string]float64{}
	for _, s := range series {
		seen[s.Name] = s.Points[0].Value
	}
	for n := range 7 {
		require.Contains(t, seen, fmt.Sprintf("batch_test.metric.%d", n))
		require.Equal(t, float64(n), seen[fmt.Sprintf("batch_test.metric.%d", n)])
	}
}

// No caller outside the opted-in sender needs to know that a batch is waiting.
func TestCheckBatchDoesNotBlockOtherSenders(t *testing.T) {
	for _, timestampWait := range []bool{false, true} {
		t.Run(fmt.Sprintf("timestamp-wait=%v", timestampWait), func(t *testing.T) {
			agg, manager, stop := startBatchTest(t, 1)
			s, err := manager.GetSender("batch_test:test")
			require.NoError(t, err)
			first := batchTestCall(func() { s.Gauge("batch_test.metric", 1, "", nil) })
			waitBatchTest(t, agg.batchFlushRequested)
			pending := first
			if timestampWait {
				flushBatchTest(t, agg)
				waitBatchTest(t, first)
				pending = batchTestCall(func() { s.Gauge("batch_test.metric", 2, "", nil) })
				waitBatchTest(t, agg.batchClock.(*batchTestClock).scheduled)
			}
			for _, id := range []checkid.ID{"", "other_check:test"} {
				other, err := manager.GetSender(id)
				require.NoError(t, err)
				waitBatchTest(t, batchTestCall(func() {
					for range cap(agg.checkItems) + 1 {
						other.Gauge("other.metric", 3, "", nil)
					}
					other.Commit()
					barrier := make(batchTestBarrier)
					agg.checkItems <- barrier
					<-barrier
				}))
			}
			select {
			case <-pending:
				t.Fatal("unrelated submissions released the pending batch")
			default:
			}
			stop()
			waitBatchTest(t, pending)
		})
	}
}

func TestCheckBatchConcurrentSenders(t *testing.T) {
	agg, manager, stop := startBatchTest(t, 2)
	s, err := manager.GetSender("batch_test:first")
	require.NoError(t, err)
	first := batchTestCall(func() { s.Gauge("first", 1, "", nil); s.Gauge("first", 2, "", nil) })
	waitBatchTest(t, agg.batchFlushRequested)
	secondSender, err := manager.GetSender("batch_test:second")
	require.NoError(t, err)
	second := batchTestCall(func() { secondSender.Gauge("second", 1, "", nil); secondSender.Gauge("second", 2, "", nil) })
	waitBatchTest(t, agg.batchFlushRequested)
	// Further calls on the first sender wait behind its outstanding drain.
	concurrent := batchTestCall(func() { s.Gauge("after_first", 3, "", nil) })
	barrier := make(batchTestBarrier)
	agg.checkItems <- barrier
	waitBatchTest(t, barrier)
	agg.mu.Lock()
	firstSeries := len(agg.checkSamplers["batch_test:first"].series)
	secondSeries := len(agg.checkSamplers["batch_test:second"].series)
	agg.mu.Unlock()
	require.Equal(t, 1, firstSeries)
	require.Equal(t, 1, secondSeries)
	series := flushBatchTest(t, agg)
	waitBatchTest(t, first)
	waitBatchTest(t, second)
	waitBatchTest(t, concurrent)
	require.Len(t, series, 3) // Two check series and the Agent heartbeat.
	stop()
}

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
			agg, manager, _ := startBatchTest(t, 2)
			sender, err := manager.GetSender("batch_test:test")
			require.NoError(t, err)
			s := sender.(*checkSender)
			done := batchTestCall(func() {
				for _, value := range []float64{3, 4, 5, 6, 7} {
					s.sendMetricSample("batch_test.repeated", value, "", nil, tc.kind, true, false, 0)
				}
				s.Commit()
			})
			clk := agg.batchClock.(*batchTestClock)
			var series metrics.Series
			for batch := range 3 {
				if batch > 0 {
					waitBatchTest(t, clk.scheduled)
					// An unrelated flush cannot bypass a timestamp wait.
					for _, s := range flushBatchTest(t, agg) {
						require.NotEqual(t, "batch_test.repeated"+tc.suffix, s.Name)
					}
					clk.Add(time.Second)
				}
				waitBatchTest(t, agg.batchFlushRequested)
				series = append(series, flushBatchTest(t, agg)...)
			}
			waitBatchTest(t, done)
			var timestamps, values []float64
			for _, s := range series {
				if s.Name == "batch_test.repeated"+tc.suffix {
					for _, p := range s.Points {
						timestamps = append(timestamps, p.Ts)
						values = append(values, p.Value)
					}
				}
			}
			require.Equal(t, []float64{100, 101, 102}, timestamps)
			require.Equal(t, tc.want, values)
		})
	}
}

// Reproduce the feedback edge: the serializer submits telemetry after the next
// batch is ready, before it can return and allow that batch's flush to start.
func TestCheckBatchSerializerFeedback(t *testing.T) {
	agg, manager, _ := startBatchTest(t, 1)
	s, err := manager.GetSender("batch_test:test")
	require.NoError(t, err)
	telemetry, err := manager.GetDefaultSender()
	require.NoError(t, err)
	first := batchTestCall(func() { s.Gauge("first", 1, "", nil) })
	waitBatchTest(t, agg.batchFlushRequested)
	serialized, feedback := make(chan struct{}), make(chan struct{})
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(feedback) }) })
	flushed := batchTestCall(func() {
		metrics.Serialize(metrics.NewIterableSeries(func(*metrics.Serie) {}, 1, 1), nil,
			func(series metrics.SerieSink, sketches metrics.SketchesSink) {
				done := make(chan struct{})
				agg.flushChan <- flushTrigger{trigger: trigger{time: agg.batchClock.Now(), blockChan: done}, seriesSink: series, sketchesSink: sketches}
				<-done
			}, func(source metrics.SerieSource) {
				for source.MoveNext() {
				}
				close(serialized)
				<-feedback
				// Exceed the input capacity: queue slack must not hide the dependency.
				for range cap(agg.checkItems) + 1 {
					telemetry.Gauge("internal.telemetry", 1, "", nil)
				}
			}, func(metrics.SketchesSource) {})
	})
	waitBatchTest(t, serialized)
	waitBatchTest(t, first)
	agg.batchClock.(*batchTestClock).Add(time.Second)
	second := batchTestCall(func() { s.Gauge("second", 2, "", nil) })
	waitBatchTest(t, agg.batchFlushRequested)
	release.Do(func() { close(feedback) })
	waitBatchTest(t, flushed)
	flushBatchTest(t, agg)
	waitBatchTest(t, second)
}

func TestCheckBatchStopWithFullInput(t *testing.T) {
	stopped := make(chan struct{})
	s := &checkSender{itemsOut: make(chan senderItem), batch: &checkBatch{limit: 1, stopped: stopped}}
	// The first caller waits for input capacity; the second waits for the sender.
	first := batchTestCall(func() { s.SendRawMetricSample(&metrics.MetricSample{}) })
	second := batchTestCall(func() { s.Commit() })
	close(stopped)
	waitBatchTest(t, first)
	waitBatchTest(t, second)
}

func TestCheckBatchSamplerReuse(t *testing.T) {
	agg, manager, _ := startBatchTest(t, 1)
	const id = "batch_test:test"
	s, err := manager.GetSender(id)
	require.NoError(t, err)
	first := batchTestCall(func() { s.Gauge("first", 1, "", nil) })
	waitBatchTest(t, agg.batchFlushRequested)
	flushBatchTest(t, agg)
	waitBatchTest(t, first)
	manager.DestroySender(id)
	replacement, err := manager.GetSender(id)
	require.NoError(t, err)
	require.NotSame(t, s, replacement)
	second := batchTestCall(func() { replacement.Gauge("second", 2, "", nil) })
	clk := agg.batchClock.(*batchTestClock)
	waitBatchTest(t, clk.scheduled)
	// Re-registering a sender must not permit a duplicate commit timestamp.
	clk.Set(time.Unix(99, 0))
	clk.Set(time.Unix(101, 0))
	waitBatchTest(t, agg.batchFlushRequested)
	series := flushBatchTest(t, agg)
	waitBatchTest(t, second)
	for _, serie := range series {
		if serie.Name == "second" {
			require.Equal(t, float64(101), serie.Points[0].Ts)
			return
		}
	}
	t.Fatal("missing replacement sender's metric")
}

func TestCheckBatchSubmissionForms(t *testing.T) {
	for _, bucket := range []bool{false, true} {
		t.Run(fmt.Sprintf("bucket=%v", bucket), func(t *testing.T) {
			agg, manager, stop := startBatchTest(t, 1)
			sender, err := manager.GetSender("batch_test:test")
			require.NoError(t, err)
			s := sender.(*checkSender)
			done := batchTestCall(func() {
				if bucket {
					s.HistogramBucket("bucket", 3, 0, 1, false, "", nil, true)
				} else {
					s.SendRawMetricSample(&metrics.MetricSample{Name: "raw", Value: 1, Mtype: metrics.GaugeType})
				}
			})
			waitBatchTest(t, agg.batchFlushRequested)
			select {
			case <-done:
				t.Fatal("submission bypassed its batch limit")
			default:
			}
			stop()
			waitBatchTest(t, done)
		})
	}
}

func TestCheckBatchSenderHasOneOwner(t *testing.T) {
	agg, manager, _ := startBatchTest(t, 1)
	const id = "batch_test:test"
	existing, err := manager.GetSender(id)
	require.NoError(t, err)
	// Both GetSender callers can miss the initial lookup before either creates it.
	// The creation path itself must return the existing sender on the second call.
	same, err := manager.senderPool.mkSender(id)
	require.NoError(t, err)
	require.Same(t, existing, same)
	require.Equal(t, agg.batchSize(id), same.(*checkSender).batch.limit)
}
