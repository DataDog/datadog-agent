// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflowlib"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
)

const flushFlowsToSendInterval = 10 * time.Second
const metricPrefix = "datadog.netflow."

// FlowAggregator is used for space and time aggregation of NetFlow flows
type FlowAggregator struct {
	flowIn                       chan *common.Flow
	flushFlowsToSendInterval     time.Duration // interval for checking flows to flush and send them to EP Forwarder
	rollupTrackerRefreshInterval time.Duration
	flowAcc                      *flowAccumulator
	sender                       aggregator.Sender
	stopChan                     chan struct{}
	receivedFlowCount            *atomic.Uint64
	flushedFlowCount             *atomic.Uint64
	hostname                     string
	goflowPrometheusGatherer     prometheus.Gatherer
}

// NewFlowAggregator returns a new FlowAggregator
func NewFlowAggregator(sender aggregator.Sender, config *config.NetflowConfig, hostname string) *FlowAggregator {
	flushInterval := time.Duration(config.AggregatorFlushInterval) * time.Second
	flowContextTTL := time.Duration(config.AggregatorFlowContextTTL) * time.Second
	rollupTrackerRefreshInterval := time.Duration(config.AggregatorRollupTrackerRefreshInterval) * time.Second
	return &FlowAggregator{
		flowIn:                       make(chan *common.Flow, config.AggregatorBufferSize),
		flowAcc:                      newFlowAccumulator(flushInterval, flowContextTTL, config.AggregatorPortRollupThreshold, config.AggregatorPortRollupDisabled),
		flushFlowsToSendInterval:     flushFlowsToSendInterval,
		rollupTrackerRefreshInterval: rollupTrackerRefreshInterval,
		sender:                       sender,
		stopChan:                     make(chan struct{}),
		receivedFlowCount:            atomic.NewUint64(0),
		flushedFlowCount:             atomic.NewUint64(0),
		hostname:                     hostname,
		goflowPrometheusGatherer:     prometheus.DefaultGatherer,
	}
}

// Start will start the FlowAggregator worker
func (agg *FlowAggregator) Start() {
	log.Info("Flow Aggregator started")
	go agg.run()
	agg.flushLoop() // blocking call
}

// Stop will stop running FlowAggregator
func (agg *FlowAggregator) Stop() {
	close(agg.stopChan)
}

// GetFlowInChan returns flow input chan
func (agg *FlowAggregator) GetFlowInChan() chan *common.Flow {
	return agg.flowIn
}

func (agg *FlowAggregator) run() {
	for {
		select {
		case <-agg.stopChan:
			log.Info("Stopping aggregator")
			return
		case flow := <-agg.flowIn:
			agg.receivedFlowCount.Inc()
			agg.flowAcc.add(flow)
		}
	}
}

func (agg *FlowAggregator) sendFlows(flows []*common.Flow) {
	for _, flow := range flows {
		flowPayload := buildPayload(flow, agg.hostname)
		payloadBytes, err := json.Marshal(flowPayload)
		if err != nil {
			log.Errorf("Error marshalling device metadata: %s", err)
			continue
		}

		log.Tracef("flushed flow: %s", string(payloadBytes))
		agg.sender.EventPlatformEvent(payloadBytes, epforwarder.EventTypeNetworkDevicesNetFlow)
	}
}

func (agg *FlowAggregator) flushLoop() {
	var flushFlowsToSendTicker <-chan time.Time

	if agg.flushFlowsToSendInterval > 0 {
		flushFlowsToSendTicker = time.NewTicker(agg.flushFlowsToSendInterval).C
	} else {
		log.Debug("flushFlowsToSendInterval set to 0: will never flush automatically")
	}

	rollupTrackersRefresh := time.NewTicker(agg.rollupTrackerRefreshInterval).C

	for {
		select {
		// stop sequence
		case <-agg.stopChan:
			return
		// automatic flush sequence
		case <-flushFlowsToSendTicker:
			agg.flush()
		// refresh rollup trackers
		case <-rollupTrackersRefresh:
			agg.rollupTrackersRefresh()
		}
	}
}

// Flush flushes the aggregator
func (agg *FlowAggregator) flush() int {
	flowsContexts := agg.flowAcc.getFlowContextCount()
	now := time.Now()
	flowsToFlush := agg.flowAcc.flush()
	log.Debugf("Flushing %d flows to the forwarder (flush_duration=%d, flow_contexts_before_flush=%d)", len(flowsToFlush), time.Since(now).Milliseconds(), flowsContexts)

	// TODO: Add flush stats to agent telemetry e.g. aggregator newFlushCountStats()
	if len(flowsToFlush) > 0 {
		agg.sendFlows(flowsToFlush)
	}

	flushCount := len(flowsToFlush)

	agg.sender.MonotonicCount("datadog.netflow.aggregator.hash_collisions", float64(agg.flowAcc.hashCollisionFlowCount.Load()), "", nil)
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_received", float64(agg.receivedFlowCount.Load()), "", nil)
	agg.sender.Count("datadog.netflow.aggregator.flows_flushed", float64(flushCount), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.flows_contexts", float64(flowsContexts), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.current_store_size", float64(agg.flowAcc.portRollup.GetCurrentStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.port_rollup.new_store_size", float64(agg.flowAcc.portRollup.GetNewStoreSize()), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.capacity", float64(cap(agg.flowIn)), "", nil)
	agg.sender.Gauge("datadog.netflow.aggregator.input_buffer.length", float64(len(agg.flowIn)), "", nil)

	err := agg.submitCollectorMetrics()
	if err != nil {
		log.Warnf("error submitting collector metrics: %s", err)
	}

	// We increase `flushedFlowCount` at the end to be sure that the metrics are submitted before hand.
	// Tests will wait for `flushedFlowCount` to be increased before asserting the metrics.
	agg.flushedFlowCount.Add(uint64(flushCount))
	return len(flowsToFlush)
}

func (agg *FlowAggregator) rollupTrackersRefresh() {
	log.Debugf("Rollup tracker refresh: use new store as current store")
	agg.flowAcc.portRollup.UseNewStoreAsCurrentStore()
}

func (agg *FlowAggregator) submitCollectorMetrics() error {
	promMetrics, err := agg.goflowPrometheusGatherer.Gather()
	if err != nil {
		return err
	}
	for _, metricFamily := range promMetrics {
		for _, metric := range metricFamily.Metric {
			log.Tracef("Collector metric `%s`: type=`%v` value=`%v`, label=`%v`", metricFamily.GetName(), metricFamily.GetType().String(), metric.GetCounter().GetValue(), metric.GetLabel())
			metricType, name, value, tags, err := goflowlib.ConvertMetric(metric, metricFamily)
			if err != nil {
				log.Tracef("Error converting prometheus metric: %s", err)
				continue
			}
			switch metricType {
			case metrics.GaugeType:
				agg.sender.Gauge(metricPrefix+name, value, "", tags)
			case metrics.MonotonicCountType:
				agg.sender.MonotonicCount(metricPrefix+name, value, "", tags)
			default:
				log.Debugf("cannot submit unsupported type %s", metricType.String())
			}
		}
	}
	return nil
}
