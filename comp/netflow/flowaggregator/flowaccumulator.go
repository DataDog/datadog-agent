// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"sync"
	"time"

	"go.uber.org/atomic"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/portrollup"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
)

var timeNow = time.Now

// protected by the flowAccumulator flowsMutex
type flowAccStats struct {
	flowAccAddCount       int     // Number of flowAccumulator add() calls
	flowAccAddDurationSec float64 // duration of flowAccumulator add() calls in seconds

	getAggregationHashCount       int     // Number of getAggregationHash() calls
	getAggregationHashDurationSec float64 // duration of getAggregationHash() calls in seconds

	portRollupAddCount       int     // Number of port rollup add() calls
	portRollupAddDurationSec float64 // duration of port rollup add() calls in seconds

	flowSizeCount int64  // Number of flow sizes sampled
	flowSizeBytes uint64 // Size of the flow added (using Sizeof)
}

// flowContext contains flow information and additional flush related data
type flowContext struct {
	flow                *common.Flow
	nextFlush           time.Time
	lastSuccessfulFlush time.Time
}

// flowAccumulator is used to accumulate aggregated flows
type flowAccumulator struct {
	flows map[uint64]flowContext
	// mutex is needed to protect `flows` since `flowAccumulator.add()` and  `flowAccumulator.flush()`
	// are called by different routines.
	flowsMutex sync.Mutex

	flowFlushInterval time.Duration
	flowContextTTL    time.Duration

	portRollup          *portrollup.EndpointPairPortRollupStore
	portRollupThreshold int
	portRollupDisabled  bool

	hashCollisionFlowCount *atomic.Uint64
	memoryStatsSampleCount *atomic.Uint64

	logger      log.Component
	rdnsQuerier rdnsquerier.Component

	skipHashCollisionDetection bool
	aggregationHashUseSyncPool bool
	getMemoryStats             bool

	flowAccStats *flowAccStats
}

func newFlowContext(flow *common.Flow) flowContext {
	now := timeNow()
	return flowContext{
		flow:      flow,
		nextFlush: now,
	}
}

func newFlowAccumulator(aggregatorFlushInterval time.Duration, aggregatorFlowContextTTL time.Duration, portRollupThreshold int, portRollupDisabled bool, skipHashCollisionDetection bool, aggregationHashUseSyncPool bool, portRollupUseFixedSizeKey bool, getMemoryStats bool, logger log.Component, rdnsQuerier rdnsquerier.Component) *flowAccumulator {
	return &flowAccumulator{
		flows:                      make(map[uint64]flowContext),
		flowFlushInterval:          aggregatorFlushInterval,
		flowContextTTL:             aggregatorFlowContextTTL,
		portRollup:                 portrollup.NewEndpointPairPortRollupStore(portRollupThreshold, portRollupUseFixedSizeKey, logger),
		portRollupThreshold:        portRollupThreshold,
		portRollupDisabled:         portRollupDisabled,
		hashCollisionFlowCount:     atomic.NewUint64(0),
		memoryStatsSampleCount:     atomic.NewUint64(0),
		logger:                     logger,
		rdnsQuerier:                rdnsQuerier,
		skipHashCollisionDetection: skipHashCollisionDetection,
		aggregationHashUseSyncPool: aggregationHashUseSyncPool,
		getMemoryStats:             getMemoryStats,
		flowAccStats:               &flowAccStats{},
	}
}

// flush will flush specific flow context (distinct hash) if nextFlush is reached
// once a flow context is flushed nextFlush will be updated to the next flush time
//
// flowContextTTL:
// flowContextTTL defines the duration we should keep a specific flowContext in `flowAccumulator.flows`
// after `lastSuccessfulFlush`. // Flow context in `flowAccumulator.flows` map will be deleted if `flowContextTTL`
// is reached to avoid keeping flow context that are not seen anymore.
// We need to keep flowContext (contains `nextFlush` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flowFlushInterval`).
// Example, after a flush, flowContext will have a new nextFlush, that will be the next flush time for new flows being added.
func (f *flowAccumulator) flush() ([]*common.Flow, *flowAccStats) {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	var flowsToFlush []*common.Flow
	for key, flowCtx := range f.flows {
		now := timeNow()
		if flowCtx.flow == nil && (flowCtx.lastSuccessfulFlush.Add(f.flowContextTTL).Before(now)) {
			f.logger.Tracef("Delete flow context (key=%d, lastSuccessfulFlush=%s, nextFlush=%s)", key, flowCtx.lastSuccessfulFlush.String(), flowCtx.nextFlush.String())
			// delete flowCtx wrapper if there is no successful flushes since `flowContextTTL`
			delete(f.flows, key)
			continue
		}
		if flowCtx.nextFlush.After(now) {
			continue
		}
		if flowCtx.flow != nil {
			flowsToFlush = append(flowsToFlush, flowCtx.flow)
			flowCtx.lastSuccessfulFlush = now
			flowCtx.flow = nil
		}
		flowCtx.nextFlush = flowCtx.nextFlush.Add(f.flowFlushInterval)
		f.flows[key] = flowCtx
	}
	retFlowAccStats := f.flowAccStats
	f.flowAccStats = &flowAccStats{}
	return flowsToFlush, retFlowAccStats
}

// getAggregationHash returns the aggregation hash for a flow using the configured implementation
func (f *flowAccumulator) getAggregationHash(flow *common.Flow) uint64 {
	if f.aggregationHashUseSyncPool {
		return flow.AggregationHash()
	}
	return flow.AggregationHashOriginal()
}

// shouldSampleMemoryStats returns true for 1/100 calls to sample memory statistics
func (f *flowAccumulator) shouldSampleMemoryStats() bool {
	if !f.getMemoryStats {
		return false
	}
	// Sample every 100th call (1% sampling rate)
	return f.memoryStatsSampleCount.Inc()%100 == 0
}

func (f *flowAccumulator) add(flowToAdd *common.Flow) {
	startFull := timeNow()

	f.logger.Tracef("Add new flow: %+v", flowToAdd)

	if !f.portRollupDisabled {
		// Handle port rollup
		start := timeNow()
		f.portRollup.Add(flowToAdd.SrcAddr, flowToAdd.DstAddr, uint16(flowToAdd.SrcPort), uint16(flowToAdd.DstPort))
		f.flowAccStats.portRollupAddDurationSec += time.Since(start).Seconds()
		f.flowAccStats.portRollupAddCount++

		ephemeralStatus := f.portRollup.IsEphemeral(flowToAdd.SrcAddr, flowToAdd.DstAddr, uint16(flowToAdd.SrcPort), uint16(flowToAdd.DstPort))
		switch ephemeralStatus {
		case portrollup.IsEphemeralSourcePort:
			flowToAdd.SrcPort = portrollup.EphemeralPort
		case portrollup.IsEphemeralDestPort:
			flowToAdd.DstPort = portrollup.EphemeralPort
		}
	}

	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	defer func() {
		f.flowAccStats.flowAccAddCount++
		f.flowAccStats.flowAccAddDurationSec += time.Since(startFull).Seconds()
	}()

	start := nanoNow()
	aggHash := f.getAggregationHash(flowToAdd)
	f.flowAccStats.getAggregationHashDurationSec = float64(nanoSince(start)) / 1e9 // convert to seconds
	f.flowAccStats.getAggregationHashCount++

	aggFlow, ok := f.flows[aggHash]
	if !ok {
		f.flows[aggHash] = newFlowContext(flowToAdd)
		f.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)

		if f.shouldSampleMemoryStats() {
			// Calculate the size of the flow being added (sampled 1/100 calls)
			f.flowAccStats.flowSizeCount++
			f.flowAccStats.flowSizeBytes = common.Sizeof(f.flows[aggHash])
		}
		return
	}
	if aggFlow.flow == nil {
		// flowToAdd is for the same hash as an aggregated flow that has been flushed
		aggFlow.flow = flowToAdd
		f.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
	} else {
		// use go routine for hash collision detection to avoid blocking critical path
		if !f.skipHashCollisionDetection {
			go f.detectHashCollision(aggHash, *aggFlow.flow, *flowToAdd)
		}

		// accumulate flowToAdd with existing flow(s) with same hash
		aggFlow.flow.Bytes += flowToAdd.Bytes
		aggFlow.flow.Packets += flowToAdd.Packets
		aggFlow.flow.StartTimestamp = common.Min(aggFlow.flow.StartTimestamp, flowToAdd.StartTimestamp)
		aggFlow.flow.EndTimestamp = common.Max(aggFlow.flow.EndTimestamp, flowToAdd.EndTimestamp)
		aggFlow.flow.SequenceNum = common.Max(aggFlow.flow.SequenceNum, flowToAdd.SequenceNum)
		aggFlow.flow.TCPFlags |= flowToAdd.TCPFlags

		// keep first non-null value for custom fields
		if flowToAdd.AdditionalFields != nil {
			if aggFlow.flow.AdditionalFields == nil {
				aggFlow.flow.AdditionalFields = make(common.AdditionalFields)
			}

			for field, value := range flowToAdd.AdditionalFields {
				if _, ok := aggFlow.flow.AdditionalFields[field]; !ok {
					aggFlow.flow.AdditionalFields[field] = value
				}
			}
		}
	}
	f.flows[aggHash] = aggFlow
}

func (f *flowAccumulator) setSrcReverseDNSHostname(aggHash uint64, hostname string, acquireLock bool) {
	if hostname == "" {
		return
	}

	if acquireLock {
		f.flowsMutex.Lock()
		defer f.flowsMutex.Unlock()
	}

	aggFlow, ok := f.flows[aggHash]
	if ok && aggFlow.flow != nil {
		aggFlow.flow.SrcReverseDNSHostname = hostname
	}
}

func (f *flowAccumulator) setDstReverseDNSHostname(aggHash uint64, hostname string, acquireLock bool) {
	if hostname == "" {
		return
	}

	if acquireLock {
		f.flowsMutex.Lock()
		defer f.flowsMutex.Unlock()
	}

	aggFlow, ok := f.flows[aggHash]
	if ok && aggFlow.flow != nil {
		aggFlow.flow.DstReverseDNSHostname = hostname
	}
}

func (f *flowAccumulator) addRDNSEnrichment(aggHash uint64, srcAddr []byte, dstAddr []byte) {
	err := f.rdnsQuerier.GetHostnameAsync(
		srcAddr,
		// Sync callback, lock is already held
		func(hostname string) {
			f.setSrcReverseDNSHostname(aggHash, hostname, false)
		},
		// Async callback will reacquire the lock
		func(hostname string, err error) {
			if err != nil {
				f.logger.Debugf("Error resolving reverse DNS enrichment for source IP address: %v error: %v", srcAddr, err)
				return
			}
			f.setSrcReverseDNSHostname(aggHash, hostname, true)
		},
	)
	if err != nil {
		f.logger.Debugf("Error requesting reverse DNS enrichment for source IP address: %v error: %v", srcAddr, err)
	}

	err = f.rdnsQuerier.GetHostnameAsync(
		dstAddr,
		// Sync callback, lock is held
		func(hostname string) {
			f.setDstReverseDNSHostname(aggHash, hostname, false)
		},
		// Async callback will reacquire the lock
		func(hostname string, err error) {
			if err != nil {
				f.logger.Debugf("Error resolving reverse DNS enrichment for destination IP address: %v error: %v", dstAddr, err)
				return
			}
			f.setDstReverseDNSHostname(aggHash, hostname, true)
		},
	)
	if err != nil {
		f.logger.Debugf("Error requesting reverse DNS enrichment for destination IP address: %v error: %v", dstAddr, err)
	}
}

func (f *flowAccumulator) getFlowContextCount() int {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	return len(f.flows)
}

func (f *flowAccumulator) detectHashCollision(hash uint64, existingFlow common.Flow, flowToAdd common.Flow) {
	if !common.IsEqualFlowContext(existingFlow, flowToAdd) {
		f.logger.Warnf("Hash collision for flows with hash `%d`: existingFlow=`%+v` flowToAdd=`%+v`", hash, existingFlow, flowToAdd)
		f.hashCollisionFlowCount.Inc()
	}
}
