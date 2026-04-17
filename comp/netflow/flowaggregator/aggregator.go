// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package flowaggregator defines tools for aggregating observed netflows.
package flowaggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/netflow/topn"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
)

const flushFlowsToSendInterval = 10 * time.Second
const metricPrefix = "datadog.netflow."

// StandardFlowAggregator is the concrete type for the standard (non-dedup) aggregator.
type StandardFlowAggregator = FlowAggregator[FlowBatch]

// DedupFlowAggregator is the concrete type for the deduplication aggregator.
type DedupFlowAggregator = FlowAggregator[FlowGroupBatch]

// FlowAggregatorRunner is the non-generic interface for running a FlowAggregator.
// Used by external consumers (server, listener) that don't need to know the
// accumulator's flush result type.
type FlowAggregatorRunner interface {
	Start()
	Stop()
	GetFlowInChan() chan *common.Flow

	// Testing helpers — allow tests to configure the aggregator without
	// type-asserting to a concrete generic instantiation.
	SetTimeNowFunction(fn func() time.Time)
	SetFlushTickFrequency(d time.Duration)
	GetFlushedFlowCount() *atomic.Uint64
	GetFlowContextCount() int
}

// FlowFlushFilter is an interface that can be used to filter flows before they are sent to the EP Forwarder.
type FlowFlushFilter interface {
	Filter(flushCtx common.FlushContext, flows []*common.Flow) []*common.Flow
}

// FlowAggregator is used for space and time aggregation of NetFlow flows.
// The type parameter T is the flush result type from the accumulator:
//   - FlowBatch for the standard path
//   - FlowGroupBatch for the dedup path
type FlowAggregator[T any] struct {
	flowIn                       chan *common.Flow
	FlushConfig                  common.FlushConfig
	rollupTrackerRefreshInterval time.Duration
	flowAcc                      FlowAccumulator[T]
	submitter                    EPForwarder[T]
	sender                       sender.Sender
	stopChan                     chan struct{}
	flushLoopDone                chan struct{}
	runDone                      chan struct{}
	receivedFlowCount            *atomic.Uint64
	flushedFlowCount             *atomic.Uint64
	goflowPrometheusGatherer     prometheus.Gatherer
	TimeNowFunction              func() time.Time // Allows to mock time in tests
	NewTicker                    func(duration time.Duration) <-chan time.Time
	logger                       log.Component
}

// newFlowAggregatorBase builds the shared fields for a FlowAggregator[T].
func newFlowAggregatorBase[T any](flowAcc FlowAccumulator[T], submitter EPForwarder[T], flushConfig common.FlushConfig, rollupTrackerRefreshInterval time.Duration, bufferSize int, snd sender.Sender, logger log.Component) *FlowAggregator[T] {
	return &FlowAggregator[T]{
		flowIn:                       make(chan *common.Flow, bufferSize),
		flowAcc:                      flowAcc,
		submitter:                    submitter,
		FlushConfig:                  flushConfig,
		rollupTrackerRefreshInterval: rollupTrackerRefreshInterval,
		sender:                       snd,
		stopChan:                     make(chan struct{}),
		runDone:                      make(chan struct{}),
		flushLoopDone:                make(chan struct{}),
		receivedFlowCount:            atomic.NewUint64(0),
		flushedFlowCount:             atomic.NewUint64(0),
		goflowPrometheusGatherer:     prometheus.DefaultGatherer,
		TimeNowFunction:              time.Now,
		NewTicker:                    time.Tick,
		logger:                       logger,
	}
}

// NewStandardFlowAggregator creates a FlowAggregator that flushes individual flows.
// This is the standard (non-dedup) path with optional TopN filtering and jitter scheduling.
func NewStandardFlowAggregator(snd sender.Sender, epForwarder eventplatform.Forwarder, conf *config.NetflowConfig, hostname string, logger log.Component, rdnsQuerier rdnsquerier.Component) *StandardFlowAggregator {
	flushConfig := common.FlushConfig{
		FlowCollectionDuration: time.Duration(conf.AggregatorFlushInterval) * time.Second,
		FlushTickFrequency:     flushFlowsToSendInterval,
	}

	var topNFilter FlowFlushFilter = topn.NoopFilter{}
	var flowScheduler FlowScheduler = ImmediateFlowScheduler{flushConfig: flushConfig}
	if conf.AggregatorMaxFlowsPerPeriod > 0 {
		topNFilter = topn.NewPerFlushFilter(int64(conf.AggregatorMaxFlowsPerPeriod), flushConfig, snd, logger)
		flowScheduler = JitterFlowScheduler{flushConfig: flushConfig}
	}

	flowContextTTL := time.Duration(conf.AggregatorFlowContextTTL) * time.Second
	rollupInterval := time.Duration(conf.AggregatorRollupTrackerRefreshInterval) * time.Second

	acc := newStandardFlowAccumulator(flushConfig, flowScheduler, flowContextTTL, conf.AggregatorPortRollupThreshold, conf.AggregatorPortRollupDisabled, logger, rdnsQuerier)
	submitter := &standardEPForwarder{
		filter:      topNFilter,
		seqTracker:  newSequenceTracker(snd, logger),
		epForwarder: epForwarder,
		hostname:    hostname,
		logger:      logger,
	}
	return newFlowAggregatorBase[FlowBatch](acc, submitter, flushConfig, rollupInterval, conf.AggregatorBufferSize, snd, logger)
}

// NewDedupFlowAggregator creates a FlowAggregator that flushes grouped FlowGroups.
// This is the deduplication path: flows sharing a 5-tuple are merged into a single
// event with a reporters list. TopN filtering and jitter scheduling are disabled.
func NewDedupFlowAggregator(snd sender.Sender, epForwarder eventplatform.Forwarder, conf *config.NetflowConfig, hostname string, logger log.Component, rdnsQuerier rdnsquerier.Component) *DedupFlowAggregator {
	flushConfig := common.FlushConfig{
		FlowCollectionDuration: time.Duration(conf.AggregatorFlushInterval) * time.Second,
		FlushTickFrequency:     flushFlowsToSendInterval,
	}

	flowScheduler := ImmediateFlowScheduler{flushConfig: flushConfig}
	flowContextTTL := time.Duration(conf.AggregatorFlowContextTTL) * time.Second
	rollupInterval := time.Duration(conf.AggregatorRollupTrackerRefreshInterval) * time.Second

	acc := newDedupFlowAccumulator(flushConfig, flowScheduler, flowContextTTL, conf.AggregatorPortRollupThreshold, conf.AggregatorPortRollupDisabled, logger, rdnsQuerier)
	submitter := &dedupEPForwarder{
		seqTracker:  newSequenceTracker(snd, logger),
		epForwarder: epForwarder,
		hostname:    hostname,
		logger:      logger,
	}
	return newFlowAggregatorBase[FlowGroupBatch](acc, submitter, flushConfig, rollupInterval, conf.AggregatorBufferSize, snd, logger)
}

// NewFlowAggregator returns a FlowAggregatorRunner, selecting the standard or dedup
// implementation based on the config. External consumers that don't need the type
// parameter should use this constructor.
func NewFlowAggregator(snd sender.Sender, epForwarder eventplatform.Forwarder, conf *config.NetflowConfig, hostname string, logger log.Component, rdnsQuerier rdnsquerier.Component) FlowAggregatorRunner {
	if conf.DeduplicationEnabled {
		return NewDedupFlowAggregator(snd, epForwarder, conf, hostname, logger, rdnsQuerier)
	}
	return NewStandardFlowAggregator(snd, epForwarder, conf, hostname, logger, rdnsQuerier)
}

// Start will start the FlowAggregator worker
func (agg *FlowAggregator[T]) Start() {
	agg.logger.Info("Flow Aggregator started")
	go agg.run()
	agg.flushLoop() // blocking call
}

// Stop will stop running FlowAggregator
func (agg *FlowAggregator[T]) Stop() {
	close(agg.stopChan)
	<-agg.flushLoopDone
	<-agg.runDone
}

// GetFlowInChan returns flow input chan
func (agg *FlowAggregator[T]) GetFlowInChan() chan *common.Flow {
	return agg.flowIn
}

// SetTimeNowFunction overrides the time source used by the aggregator.
func (agg *FlowAggregator[T]) SetTimeNowFunction(fn func() time.Time) {
	agg.TimeNowFunction = fn
}

// SetFlushTickFrequency overrides the flush tick interval.
func (agg *FlowAggregator[T]) SetFlushTickFrequency(d time.Duration) {
	agg.FlushConfig.FlushTickFrequency = d
}

// GetFlushedFlowCount returns the flushed flow counter.
func (agg *FlowAggregator[T]) GetFlushedFlowCount() *atomic.Uint64 {
	return agg.flushedFlowCount
}

// GetFlowContextCount returns the number of flow contexts currently tracked.
func (agg *FlowAggregator[T]) GetFlowContextCount() int {
	return agg.flowAcc.GetFlowContextCount()
}

func (agg *FlowAggregator[T]) run() {
	for {
		select {
		case <-agg.stopChan:
			agg.logger.Info("Stopping aggregator")
			agg.runDone <- struct{}{}
			return
		case flow := <-agg.flowIn:
			agg.receivedFlowCount.Inc()
			agg.flowAcc.Add(flow)
		}
	}
}

func (agg *FlowAggregator[T]) flushLoop() {
	flushFlowsToSendTicker := agg.NewTicker(agg.FlushConfig.FlushTickFrequency)
	if flushFlowsToSendTicker == nil {
		agg.logger.Debug("flushFlowsToSendInterval set to 0: will never flush automatically")
	}

	rollupTrackersRefresh := agg.NewTicker(agg.rollupTrackerRefreshInterval)
	// TODO: move rollup tracker refresh to a separate loop (separate PR) to avoid rollup tracker and flush flows impacting each other

	var lastFlushTime time.Time
	for {
		select {
		// stop sequence
		case <-agg.stopChan:
			agg.flushLoopDone <- struct{}{}
			return
		// automatic flush sequence
		case flushStartTime := <-flushFlowsToSendTicker:
			if !lastFlushTime.IsZero() {
				flushInterval := flushStartTime.Sub(lastFlushTime)
				agg.sender.Gauge("datadog.netflow.aggregator.flush_interval", flushInterval.Seconds(), "", nil)
			}

			// Calculate how many flushes should have happened since the last tick. Ticks can be missed for various reasons,
			// including CPU pauses or if this goroutine gets blocked longer than usual waiting on network requests.
			var expectedFlushes int64 = 1
			if !lastFlushTime.IsZero() {
				// add one millisecond to account for small variations in the time.Ticker
				timeSinceLast := flushStartTime.Sub(lastFlushTime) + time.Millisecond

				// We do not want this to default to 0 for small time deltas or if time.Tick fires a little early.
				// Make the expected flushes either 1 or the result of calculation
				expectedFlushes = max(1, int64(timeSinceLast/agg.FlushConfig.FlushTickFrequency))
			}
			flushCtx := common.FlushContext{
				FlushTime:     flushStartTime,
				LastFlushedAt: lastFlushTime,
				NumFlushes:    expectedFlushes,
			}

			lastFlushTime = flushStartTime
			agg.flush(flushCtx)
			agg.sender.Gauge("datadog.netflow.aggregator.flush_duration", time.Since(flushStartTime).Seconds(), "", nil)
			agg.sender.Commit()
		// refresh rollup trackers
		case <-rollupTrackersRefresh:
			agg.rollupTrackersRefresh()
		}
	}
}

// flush flushes the accumulator and submits the result via the type-specific EP forwarder.
func (agg *FlowAggregator[T]) flush(ctx common.FlushContext) int {
	flowsContexts := agg.flowAcc.GetFlowContextCount()
	result := agg.flowAcc.Flush(ctx)
	flushCount := agg.submitter.Submit(result, ctx)

	agg.emitFlushMetrics(flowsContexts, flushCount)
	agg.flushedFlowCount.Add(uint64(flushCount))
	return flushCount
}

func (agg *FlowAggregator[T]) emitFlushMetrics(flowsContexts int, flushCount int) {
	agg.sender.MonotonicCount("datadog.netflow.aggregator.hash_collisions", float64(agg.flowAcc.HashCollisionCount().Load()), "", nil)
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_received", float64(agg.receivedFlowCount.Load()), "", nil)
	agg.sender.Count("datadog.netflow.aggregator.flows_flushed", float64(flushCount), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.flows_contexts", float64(flowsContexts), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.current_store_size", float64(agg.flowAcc.PortRollup().GetCurrentStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.new_store_size", float64(agg.flowAcc.PortRollup().GetNewStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.capacity", float64(cap(agg.flowIn)), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.length", float64(len(agg.flowIn)), "", nil)

	err := agg.submitCollectorMetrics()
	if err != nil {
		agg.logger.Warnf("error submitting collector metrics: %s", err)
	}
}

func (agg *FlowAggregator[T]) rollupTrackersRefresh() {
	agg.logger.Debugf("Rollup tracker refresh: use new store as current store")
	agg.flowAcc.PortRollup().UseNewStoreAsCurrentStore()
}

func (agg *FlowAggregator[T]) submitCollectorMetrics() error {
	promMetrics, err := agg.goflowPrometheusGatherer.Gather()
	if err != nil {
		return err
	}
	for _, metricFamily := range promMetrics {
		for _, metric := range metricFamily.Metric {
			agg.logger.Tracef("Collector metric `%s`: type=`%v` value=`%v`, label=`%v`", metricFamily.GetName(), metricFamily.GetType().String(), metric.GetCounter().GetValue(), metric.GetLabel())
			metricType, name, value, tags, err := goflowlib.ConvertMetric(metric, metricFamily)
			if err != nil {
				agg.logger.Tracef("Error converting prometheus metric: %s", err)
				continue
			}
			switch metricType {
			case metrics.GaugeType:
				agg.sender.Gauge(metricPrefix+name, value, "", tags)
			case metrics.MonotonicCountType:
				agg.sender.MonotonicCount(metricPrefix+name, value, "", tags)
			default:
				agg.logger.Debugf("cannot submit unsupported type %s", metricType.String())
			}
		}
	}
	return nil
}
