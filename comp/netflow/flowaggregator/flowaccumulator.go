// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/portrollup"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"go.uber.org/atomic"
)

var timeNow = time.Now

// flowContext contains flow information and additional flush related data
type flowContext struct {
	flow                *common.Flow
	nextFlush           time.Time
	lastSuccessfulFlush time.Time
	numberOfUses        uint64
	flowsAggregated     uint64
}

// flowStat tracks statistics about flows being processed
type flowStat struct {
	numberOfUses    uint64
	flowsAggregated uint64
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

	logger      log.Component
	rdnsQuerier rdnsquerier.Component
}

func newFlowContext(flow *common.Flow) flowContext {
	now := timeNow()
	return flowContext{
		flow:      flow,
		nextFlush: now, // JMW this is causing the first flush for a new flow to be within 10 seconds (the next time flushFlowsToSendInterval triggers)
		//nextFlush: now.Add(300 * time.Second), //JMWJMW - breaks unit tests
		// JMW add a config option, true or false, true = use now + aggregatorFlushInterval for nextFlush, false = use now for nextFlush
		// JMW add a config option, number of seconds to wait before flushing a new flow (set nextFlush to now + this config option)
		numberOfUses:    1,
		flowsAggregated: 1,
	}
}

func newFlowAccumulator(aggregatorFlushInterval time.Duration, aggregatorFlowContextTTL time.Duration, portRollupThreshold int, portRollupDisabled bool, logger log.Component, rdnsQuerier rdnsquerier.Component) *flowAccumulator {
	return &flowAccumulator{
		flows:                  make(map[uint64]flowContext),
		flowFlushInterval:      aggregatorFlushInterval,
		flowContextTTL:         aggregatorFlowContextTTL,
		portRollup:             portrollup.NewEndpointPairPortRollupStore(portRollupThreshold),
		portRollupThreshold:    portRollupThreshold,
		portRollupDisabled:     portRollupDisabled,
		hashCollisionFlowCount: atomic.NewUint64(0),
		logger:                 logger,
		rdnsQuerier:            rdnsQuerier,
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
func (f *flowAccumulator) flush() ([]*common.Flow, []flowStat) {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	var flowsToFlush []*common.Flow
	var flowStats []flowStat

	for key, flowCtx := range f.flows {
		now := timeNow()
		if flowCtx.flow == nil && (flowCtx.lastSuccessfulFlush.Add(f.flowContextTTL).Before(now)) {
			f.logger.Tracef("Delete flow context (key=%d, lastSuccessfulFlush=%s, nextFlush=%s)", key, flowCtx.lastSuccessfulFlush.String(), flowCtx.nextFlush.String())
			// delete flowCtx wrapper if there is no successful flushes since `flowContextTTL`
			f.logger.Infof("JMW Deleting flow context (key=0x%x, lastSuccessfulFlush=%s, nextFlush=%s, numberOfUses=%d)", key, flowCtx.lastSuccessfulFlush.String(), flowCtx.nextFlush.String(), flowCtx.numberOfUses)
			// JMW metric
			delete(f.flows, key)
			continue
		}
		if flowCtx.nextFlush.After(now) {
			continue
		}
		if flowCtx.flow != nil {
			flowsToFlush = append(flowsToFlush, flowCtx.flow)

			// Create stats for this flow
			stats := flowStat{
				numberOfUses:    flowCtx.numberOfUses,
				flowsAggregated: flowCtx.flowsAggregated,
				// JMW for each flow to flush, observe a histogram metric for duration of the flow (end time - start time), and/or current time - lastFlush
			}
			flowStats = append(flowStats, stats)

			f.logger.Infof("JMW Sending flow (key=0x%x, lastSuccessfulFlush=%s, nextFlush=%s), duration of flow: %d seconds", key, flowCtx.lastSuccessfulFlush.String(), flowCtx.nextFlush.String(), flowCtx.flow.EndTimestamp-flowCtx.flow.StartTimestamp)

			flowCtx.lastSuccessfulFlush = now
			flowCtx.flow = nil
		}
		flowCtx.nextFlush = flowCtx.nextFlush.Add(f.flowFlushInterval)
		f.flows[key] = flowCtx
	}

	return flowsToFlush, flowStats
}

func (f *flowAccumulator) add(flowToAdd *common.Flow) {
	f.logger.Tracef("Add new flow: %+v", flowToAdd)

	if !f.portRollupDisabled {
		// Handle port rollup
		f.portRollup.Add(flowToAdd.SrcAddr, flowToAdd.DstAddr, uint16(flowToAdd.SrcPort), uint16(flowToAdd.DstPort))
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

	aggHash := flowToAdd.AggregationHash() // JMW make use of aggHash/key consistent
	aggFlow, ok := f.flows[aggHash]        // JMW rename aggFlow to flowCtx
	if !ok {
		f.flows[aggHash] = newFlowContext(flowToAdd)
		f.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
		return
	}
	if aggFlow.flow == nil {
		// flowToAdd is for the same hash as an aggregated flow that has been flushed
		// JMW add metric to see how many flow contexts are reused, how many are new when this method is called
		aggFlow.numberOfUses++
		aggFlow.flowsAggregated = 1
		f.logger.Infof("JMW Reusing flow (key=0x%x, lastSuccessfulFlush=%s, nextFlush=%s, numberOfUses=%d)", aggHash, aggFlow.lastSuccessfulFlush.String(), aggFlow.nextFlush.String(), aggFlow.numberOfUses)
		aggFlow.flow = flowToAdd
		f.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
	} else {
		// use go routine for hash collision detection to avoid blocking critical path
		go f.detectHashCollision(aggHash, *aggFlow.flow, *flowToAdd)

		// JMW add metric for this - add to flow - when flushing flow track/log total number of flows that have been aggreagated into the flow being flushed
		// accumulate flowToAdd with existing flow(s) with same hash
		aggFlow.flow.Bytes += flowToAdd.Bytes
		aggFlow.flow.Packets += flowToAdd.Packets
		aggFlow.flow.StartTimestamp = common.Min(aggFlow.flow.StartTimestamp, flowToAdd.StartTimestamp)
		aggFlow.flow.EndTimestamp = common.Max(aggFlow.flow.EndTimestamp, flowToAdd.EndTimestamp)
		aggFlow.flow.SequenceNum = common.Max(aggFlow.flow.SequenceNum, flowToAdd.SequenceNum)
		aggFlow.flow.TCPFlags |= flowToAdd.TCPFlags
		aggFlow.flowsAggregated++

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
	f.flows[aggHash] = aggFlow // JMWJMW instead of this, modify it in place
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

// getFlowContextCounts returns the total number of flow contexts, number of nil flow contexts,
// and counts of flows with different port rollup states
func (f *flowAccumulator) getFlowContextCounts() (total int, nilCount int, noRollupCount int, srcRollupCount int, dstRollupCount int) {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()
	total = len(f.flows)
	for _, flowCtx := range f.flows {
		if flowCtx.flow == nil {
			nilCount++
			continue
		}
		// Count flows based on port rollup state
		if flowCtx.flow.SrcPort == portrollup.EphemeralPort && flowCtx.flow.DstPort == portrollup.EphemeralPort {
			// Both ports are rolled up
			f.logger.Errorf("Unexpected state: both source and destination ports are rolled up for flow (src=%v, dst=%v)", flowCtx.flow.SrcAddr, flowCtx.flow.DstAddr)
			continue
		} else if flowCtx.flow.SrcPort == portrollup.EphemeralPort {
			srcRollupCount++
		} else if flowCtx.flow.DstPort == portrollup.EphemeralPort {
			dstRollupCount++
		} else {
			noRollupCount++
		}
	}
	return total, nilCount, noRollupCount, srcRollupCount, dstRollupCount
}

func (f *flowAccumulator) detectHashCollision(hash uint64, existingFlow common.Flow, flowToAdd common.Flow) {
	if !common.IsEqualFlowContext(existingFlow, flowToAdd) {
		f.logger.Warnf("Hash collision for flows with hash `%d`: existingFlow=`%+v` flowToAdd=`%+v`", hash, existingFlow, flowToAdd)
		f.hashCollisionFlowCount.Inc()
	}
}
