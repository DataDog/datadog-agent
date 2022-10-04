// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"encoding/json"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
)

const flowAggregatorFlushInterval = 10 * time.Second

// FlowAggregator is used for space and time aggregation of NetFlow flows
type FlowAggregator struct {
	flowIn                       chan *common.Flow
	flushInterval                time.Duration
	rollupTrackerRefreshInterval time.Duration
	flowAcc                      *flowAccumulator
	sender                       aggregator.Sender
	stopChan                     chan struct{}
	receivedFlowCount            *atomic.Uint64
	flushedFlowCount             *atomic.Uint64
	hostname                     string
}

// NewFlowAggregator returns a new FlowAggregator
func NewFlowAggregator(sender aggregator.Sender, config *config.NetflowConfig, hostname string) *FlowAggregator {
	flushInterval := time.Duration(config.AggregatorFlushInterval) * time.Second
	flowContextTTL := time.Duration(config.AggregatorFlowContextTTL) * time.Second
	rollupTrackerRefreshInterval := time.Duration(config.AggregatorRollupTrackerRefreshInterval) * time.Second
	return &FlowAggregator{
		flowIn:                       make(chan *common.Flow, config.AggregatorBufferSize),
		flowAcc:                      newFlowAccumulator(flushInterval, flowContextTTL, config.AggregatorPortRollupThreshold),
		flushInterval:                flowAggregatorFlushInterval,
		rollupTrackerRefreshInterval: rollupTrackerRefreshInterval,
		sender:                       sender,
		stopChan:                     make(chan struct{}),
		receivedFlowCount:            atomic.NewUint64(0),
		flushedFlowCount:             atomic.NewUint64(0),
		hostname:                     hostname,
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

		payloadStr := string(payloadBytes)
		log.Tracef("flushed flow: %s", payloadStr)
		agg.sender.EventPlatformEvent(payloadStr, epforwarder.EventTypeNetworkDevicesNetFlow)
	}
}

func (agg *FlowAggregator) flushLoop() {
	var flushTicker <-chan time.Time

	if agg.flushInterval > 0 {
		flushTicker = time.NewTicker(agg.flushInterval).C
	} else {
		log.Debug("flushInterval set to 0: will never flush automatically")
	}

	rollupTrackersRefresh := time.NewTicker(agg.rollupTrackerRefreshInterval).C

	for {
		select {
		// stop sequence
		case <-agg.stopChan:
			return
		// automatic flush sequence
		case <-flushTicker:
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

	agg.flushedFlowCount.Add(uint64(len(flowsToFlush)))
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_received", float64(agg.receivedFlowCount.Load()), "", nil)
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_flushed", float64(agg.flushedFlowCount.Load()), "", nil)

	return len(flowsToFlush)
}

func (agg *FlowAggregator) rollupTrackersRefresh() {
	log.Debugf("Rollup tracker refresh: use new store as current store")
	agg.flowAcc.portRollup.UseNewStoreAsCurrentStore()
}
