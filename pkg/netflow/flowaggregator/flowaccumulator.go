// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

var timeNow = time.Now

// floWrapper contains flow information and additional flush related data
type flowWrapper struct {
	flow                *common.Flow
	nextFlush           time.Time
	lastSuccessfulFlush time.Time
}

// flowAccumulator is used to accumulate aggregated flows
type flowAccumulator struct {
	flows             map[uint64]flowWrapper
	mu                sync.Mutex
	flowFlushInterval time.Duration
	flowContextTTL    time.Duration
}

func newFlowWrapper(flow *common.Flow) flowWrapper {
	now := timeNow()
	return flowWrapper{
		flow:      flow,
		nextFlush: now,
	}
}

func newFlowAccumulator(aggregatorFlushInterval time.Duration) *flowAccumulator {
	return &flowAccumulator{
		flows:             make(map[uint64]flowWrapper),
		flowFlushInterval: aggregatorFlushInterval,
		flowContextTTL:    aggregatorFlushInterval * 5,
	}
}

// flush will flush specific flow context (distinct hash) if nextFlush is reached
// once a flow context is flushed nextFlush will be updated to the next flush time
// Specific flow context in `flowAccumulator.flows` map will be deleted if `flowContextTTL`
// is reached to avoid keeping flow context that are not seen anymore.
func (f *flowAccumulator) flush() []*common.Flow {
	f.mu.Lock()
	defer f.mu.Unlock()

	var flowsToFlush []*common.Flow
	for key, flow := range f.flows {
		now := timeNow()
		if flow.nextFlush.After(now) {
			continue
		}
		if flow.flow != nil {
			flowsToFlush = append(flowsToFlush, flow.flow)
			flow.lastSuccessfulFlush = now
			flow.flow = nil
		} else if flow.lastSuccessfulFlush.Add(f.flowContextTTL).Before(now) {
			// delete flow wrapper if there is no successful flushes since `flowContextTTL`
			delete(f.flows, key)
			continue
		}
		flow.nextFlush = flow.nextFlush.Add(f.flowFlushInterval)
		f.flows[key] = flow
	}
	return flowsToFlush
}

func (f *flowAccumulator) add(flowToAdd *common.Flow) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// TODO: handle port direction (see network-http-logger)
	// TODO: ignore ephemeral ports

	aggHash := flowToAdd.AggregationHash()
	log.Tracef("New Flow (digest=%d): %+v", aggHash, flowToAdd)

	aggFlow, ok := f.flows[aggHash]
	if !ok {
		f.flows[aggHash] = newFlowWrapper(flowToAdd)
		return
	}
	if aggFlow.flow == nil {
		aggFlow.flow = flowToAdd
	} else {
		// accumulate flowToAdd with existing flow(s) with same hash
		aggFlow.flow.Bytes += flowToAdd.Bytes
		aggFlow.flow.Packets += flowToAdd.Packets
		aggFlow.flow.StartTimestamp = common.MinUint64(aggFlow.flow.StartTimestamp, flowToAdd.StartTimestamp)
		aggFlow.flow.EndTimestamp = common.MaxUint64(aggFlow.flow.EndTimestamp, flowToAdd.EndTimestamp)
		aggFlow.flow.TCPFlags |= flowToAdd.TCPFlags
	}
	f.flows[aggHash] = aggFlow
}
