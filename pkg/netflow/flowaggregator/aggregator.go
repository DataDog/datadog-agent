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
	flowIn            chan *common.Flow
	flushInterval     time.Duration
	flowAcc           *flowAccumulator
	sender            aggregator.Sender
	stopChan          chan struct{}
	logPayload        bool
	receivedFlowCount *atomic.Uint64
	flushedFlowCount  *atomic.Uint64
	hostname          string
}

// NewFlowAggregator returns a new FlowAggregator
func NewFlowAggregator(sender aggregator.Sender, config *config.NetflowConfig, hostname string) *FlowAggregator {
	flushInterval := time.Duration(config.AggregatorFlushInterval) * time.Second
	flowContextTTL := time.Duration(config.AggregatorFlowContextTTL) * time.Second
	return &FlowAggregator{
		flowIn:            make(chan *common.Flow, config.AggregatorBufferSize),
		flowAcc:           newFlowAccumulator(flushInterval, flowContextTTL),
		flushInterval:     flowAggregatorFlushInterval,
		sender:            sender,
		stopChan:          make(chan struct{}),
		logPayload:        config.LogPayloads,
		receivedFlowCount: atomic.NewUint64(0),
		flushedFlowCount:  atomic.NewUint64(0),
		hostname:          hostname,
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
		agg.sender.EventPlatformEvent(string(payloadBytes), epforwarder.EventTypeNetworkDevicesNetFlow)

		// For debug purposes print out all flows
		if agg.logPayload {
			log.Debugf("flushed flow: %s", string(payloadBytes))
		}
	}
}

func (agg *FlowAggregator) flushLoop() {
	var flushTicker <-chan time.Time

	if agg.flushInterval > 0 {
		flushTicker = time.NewTicker(agg.flushInterval).C
	} else {
		log.Debug("flushInterval set to 0: will never flush automatically")
	}

	for {
		select {
		// stop sequence
		case <-agg.stopChan:
			return
		// automatic flush sequence
		case <-flushTicker:
			agg.flush()
		}
	}
}

// Flush flushes the aggregator
func (agg *FlowAggregator) flush() int {
	flowsToFlush := agg.flowAcc.flush()
	log.Debugf("Flushing %d flows to the forwarder", len(flowsToFlush))
	if len(flowsToFlush) == 0 {
		return 0
	}
	// TODO: Add flush stats to agent telemetry e.g. aggregator newFlushCountStats()

	agg.sendFlows(flowsToFlush)

	agg.flushedFlowCount.Add(uint64(len(flowsToFlush)))
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_received", float64(agg.receivedFlowCount.Load()), "", nil)
	agg.sender.MonotonicCount("datadog.netflow.aggregator.flows_flushed", float64(agg.flushedFlowCount.Load()), "", nil)

	return len(flowsToFlush)
}
