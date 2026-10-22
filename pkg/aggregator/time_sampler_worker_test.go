// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

// A sample sent right before WaitForPendingSamples must be processed by the
// time the call returns, and the call must be repeatable.
func TestWaitForPendingSamplesDrainsBeforeReturning(t *testing.T) {
	require := require.New(t)

	deps := createDemultiplexerAgentTestDeps(t)
	opts := demuxTestOptions()
	// InitAndStartAgentDemultiplexer (not initAgentDemultiplexer) is required
	// here: WaitForPendingSamples needs the worker's run() goroutine actually
	// running to service drainChan.
	demux := InitAndStartAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	defer demux.Stop()

	worker := demux.statsd.workers[0]
	require.Equal(0, worker.sampler.contextResolver.length())

	demux.AggregateSample(metrics.MetricSample{
		Name:      "test.metric",
		Value:     1,
		Mtype:     metrics.GaugeType,
		Timestamp: 1657099120.0,
		Tags:      []string{"tag:1"},
	})

	demux.WaitForPendingSamples()

	require.Len(worker.samplesChan, 0, "the sample must have been drained from samplesChan")
	require.Equal(1, worker.sampler.contextResolver.length(), "the sample must have been aggregated into a context, not just dequeued")

	// Calling it again with nothing pending must not block or panic.
	demux.WaitForPendingSamples()
}

// fillTimeSampler samples numContexts contexts at timestamp 1000: 5% distributions,
// 25% counters and 70% gauges.
func fillTimeSampler(sampler *TimeSampler, numContexts int) {
	matcher := filterlist.NewNoopTagMatcher()
	for i := 0; i < numContexts; i++ {
		mtype := metrics.GaugeType
		switch {
		case i%20 == 0:
			mtype = metrics.DistributionType
		case i%10 < 3:
			mtype = metrics.CounterType
		}
		sample := metrics.MetricSample{Name: fmt.Sprintf("bench.metric.%d", i%1000), Value: 1, Mtype: mtype,
			Tags: []string{"ctx:" + strconv.Itoa(i)}, SampleRate: 1, Timestamp: 1000.0}
		sampler.sample(&sample, 1000.0, matcher)
	}
}

// queueDepthSink records, each time the flush emits a series, the intake queue depth and
// the number of contexts in the sampler. It also keeps the names of "intake." series.
type queueDepthSink struct {
	samplesChan chan []metrics.MetricSample
	sampler     *TimeSampler
	depths      []int
	contexts    []int
	intake      []string
}

func (s *queueDepthSink) Append(serie *metrics.Serie) {
	s.depths = append(s.depths, len(s.samplesChan))
	s.contexts = append(s.contexts, s.sampler.contextResolver.length())
	if strings.HasPrefix(serie.Name, "intake.") {
		s.intake = append(s.intake, serie.Name)
	}
}

// newYieldTestWorker returns a worker whose sampler holds numContexts contexts at timestamp 1000.
func newYieldTestWorker(t *testing.T, budgetMs int, numContexts int) (*timeSamplerWorker, *TimeSampler) {
	configmock.New(t).SetInTest("aggregator_flush_yield_budget_ms", budgetMs)
	store := tags.NewStore(true, "test")
	sampler := testTimeSampler(store)
	fillTimeSampler(sampler, numContexts)
	w := newTimeSamplerWorker(sampler, DefaultFlushInterval, 16, nil, FlushAndSerializeInParallel{},
		store, metricname.Matcher{}, filterlist.NewNoopTagMatcher())
	return w, sampler
}

// intakeBatch returns 10 gauge samples with new contexts and no timestamp.
func intakeBatch(b int) []metrics.MetricSample {
	batch := make([]metrics.MetricSample, 10)
	for i := range batch {
		batch[i] = metrics.MetricSample{Name: fmt.Sprintf("intake.%d.%d", b, i), Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}
	}
	return batch
}

func newSinkTrigger(w *timeSamplerWorker, sampler *TimeSampler, ts time.Time, forceFlushAll bool) (*queueDepthSink, flushTrigger) {
	sink := &queueDepthSink{samplesChan: w.samplesChan, sampler: sampler}
	return sink, flushTrigger{
		trigger:      trigger{time: ts, blockChan: make(chan struct{}, 1), forceFlushAll: forceFlushAll},
		seriesSink:   sink,
		sketchesSink: &metrics.SketchSeriesList{},
	}
}

// flushWorker runs one flush on the calling goroutine, as run() would.
func flushWorker(w *timeSamplerWorker, sampler *TimeSampler, ts time.Time, forceFlushAll bool) *queueDepthSink {
	sink, trigger := newSinkTrigger(w, sampler, ts, forceFlushAll)
	w.triggerFlush(trigger)
	return sink
}

// flushWithQueuedIntake queues 3 batches of 10 new contexts, then flushes 200k contexts.
func flushWithQueuedIntake(t *testing.T, budgetMs int, forceFlushAll bool) (*queueDepthSink, *TimeSampler) {
	w, sampler := newYieldTestWorker(t, budgetMs, 200_000)
	for b := 0; b < 3; b++ {
		w.samplesChan <- intakeBatch(b)
	}
	return flushWorker(w, sampler, time.Unix(1020, 0), forceFlushAll), sampler
}

func TestFlushYieldsToIntakeWhileEmittingSeries(t *testing.T) {
	sink, sampler := flushWithQueuedIntake(t, 1, false)
	require.Len(t, sink.depths, 200_000*95/100, "the flush must emit every series")
	require.Equal(t, 3, sink.depths[0], "nothing is read before the first series")
	require.Zero(t, sink.depths[len(sink.depths)-1], "the flush must read the intake channel before it ends")
	require.Equal(t, 200_000+30, sampler.contextResolver.length(), "the 30 queued samples must be aggregated")
}

func TestFlushDoesNotYieldWhenBudgetIsZero(t *testing.T) {
	sink, _ := flushWithQueuedIntake(t, 0, false)
	require.Equal(t, 3, sink.depths[len(sink.depths)-1], "budget 0 keeps today's behavior")
}

func TestFlushDoesNotYieldOnForceFlushAll(t *testing.T) {
	sink, _ := flushWithQueuedIntake(t, 1, true)
	require.Equal(t, 3, sink.depths[len(sink.depths)-1], "a forceFlushAll flush must not read intake")
}

func TestFlushYieldReadsAtMostOncePerBudget(t *testing.T) {
	w, sampler := newYieldTestWorker(t, 0, 0)
	w.flushYieldBudget = time.Hour

	w.samplesChan <- intakeBatch(0)
	w.yieldToIntake() // lastYield is zero, so a full budget has passed
	require.Empty(t, w.samplesChan, "the first yield must read the queued batch")
	require.Equal(t, 10, sampler.contextResolver.length())

	w.samplesChan <- intakeBatch(1)
	w.yieldToIntake() // less than one budget since the last read
	require.Len(t, w.samplesChan, 1, "a second yield within the budget must not read intake")
	require.Equal(t, 10, sampler.contextResolver.length())
}

func TestFlushYieldSamplesGoToTheNextFlush(t *testing.T) {
	w, sampler := newYieldTestWorker(t, 1, 200_000)
	for b := 0; b < 3; b++ {
		w.samplesChan <- intakeBatch(b)
	}

	first := flushWorker(w, sampler, time.Unix(1020, 0), false)
	require.Zero(t, first.depths[len(first.depths)-1], "the flush must read the queued samples")
	require.Empty(t, first.intake, "samples read mid-flush must not be in the current flush")

	next := flushWorker(w, sampler, time.Now().Add(time.Minute), false)
	require.Len(t, next.intake, 30, "samples read mid-flush must be in the next flush")
}

func TestFlushYieldClosesSamplesDrainedOnce(t *testing.T) {
	w, sampler := newYieldTestWorker(t, 1, 200_000)
	w.samplesChan <- intakeBatch(0)
	w.samplesChan <- nil // the shutdown sentinel arrives during a regular flush

	sink := flushWorker(w, sampler, time.Unix(1020, 0), false)
	require.Zero(t, sink.depths[len(sink.depths)-1], "the flush must read the sentinel")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	w.waitForShutdown(ctx)
	require.NoError(t, ctx.Err(), "the yield must close samplesDrained")

	// A second close would panic. The worker must keep running and stop cleanly.
	go w.run()
	w.addSamples(intakeBatch(1))
	w.waitForPendingSamples()
	_, trigger := newSinkTrigger(w, sampler, time.Now().Add(time.Minute), false)
	w.flushChan <- trigger
	<-trigger.blockChan
	w.stop()
}

func TestFlushYieldInRunLoopWithConcurrentProducer(t *testing.T) {
	w, sampler := newYieldTestWorker(t, 1, 200_000)
	go w.run()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for b := 0; ; b++ {
			select {
			case w.samplesChan <- intakeBatch(b):
			case <-done:
				return
			}
		}
	}()

	sink, trigger := newSinkTrigger(w, sampler, time.Unix(1020, 0), false)
	w.flushChan <- trigger
	<-trigger.blockChan
	close(done)
	wg.Wait()
	w.stop()

	require.Greater(t, sink.contexts[len(sink.contexts)-1], sink.contexts[0],
		"the worker must aggregate new samples while it flushes")
}
