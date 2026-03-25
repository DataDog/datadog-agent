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

// FlowGroup is returned by flushGroups and represents all reporters for a single 5-tuple.
// Reporters observed data this cycle; GhostReporters are 0-byte snapshots from the
// previous cycle included so the platform can use them as metadata when assigning flow_role.
type FlowGroup struct {
	Reporters      []*common.Flow
	GhostReporters []*common.Flow
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

	FlushConfig    common.FlushConfig
	flowContextTTL time.Duration
	scheduler      FlowScheduler

	portRollup          *portrollup.EndpointPairPortRollupStore
	portRollupThreshold int
	portRollupDisabled  bool

	hashCollisionFlowCount *atomic.Uint64

	// shouldUseDeduplication groups flows by 5-tuple. When true, fiveTupleGroups indexes
	// the per-reporter full-hash entries so they can be flushed as a single merged event.
	shouldUseDeduplication bool
	// fiveTupleGroups maps a FiveTupleHash to the set of full AggregationHashes (one per
	// reporter) that belong to that group. Only populated when shouldUseDeduplication is true.
	fiveTupleGroups map[uint64][]uint64
	// prevCycleReporters holds 0-byte snapshots of reporters from the most recent flush of
	// each group. These are returned as GhostReporters in the next flush so the platform can
	// use them as metadata for flow_role assignment. Keyed by FiveTupleHash.
	prevCycleReporters map[uint64][]*common.Flow

	logger      log.Component
	rdnsQuerier rdnsquerier.Component
}

func newFlowAccumulator(flushConfig common.FlushConfig, flowScheduler FlowScheduler, aggregatorFlowContextTTL time.Duration, portRollupThreshold int, portRollupDisabled bool, shouldUseDeduplication bool, logger log.Component, rdnsQuerier rdnsquerier.Component) *flowAccumulator {
	acc := &flowAccumulator{
		flows:                  make(map[uint64]flowContext),
		FlushConfig:            flushConfig,
		flowContextTTL:         aggregatorFlowContextTTL,
		portRollup:             portrollup.NewEndpointPairPortRollupStore(portRollupThreshold),
		portRollupThreshold:    portRollupThreshold,
		portRollupDisabled:     portRollupDisabled,
		shouldUseDeduplication: shouldUseDeduplication,
		hashCollisionFlowCount: atomic.NewUint64(0),
		scheduler:              flowScheduler,
		logger:                 logger,
		rdnsQuerier:            rdnsQuerier,
	}
	if shouldUseDeduplication {
		acc.fiveTupleGroups = make(map[uint64][]uint64)
		acc.prevCycleReporters = make(map[uint64][]*common.Flow)
	}
	return acc
}

// flush flushes flow contexts that are due, returning the flows to send.
// Each context flushes independently based on its scheduled nextFlush time.
// Use flushGroups instead when deduplication is enabled.
//
// flowContextTTL defines the duration we should keep a specific flowContext in `flowAccumulator.flows`
// after `lastSuccessfulFlush`. Flow context in `flowAccumulator.flows` map will be deleted if `flowContextTTL`
// is reached to avoid keeping flow context that are not seen anymore.
// We need to keep flowContext (contains `nextFlush` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flowFlushInterval`).
// Example, after a flush, flowContext will have a new nextFlush, that will be the next flush time for new flows being added.
func (f *flowAccumulator) flush(flushContext common.FlushContext) []*common.Flow {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	now := flushContext.FlushTime
	var flowsToFlush []*common.Flow
	var expiredFlowKeys []uint64

	for key, flowCtx := range f.flows {
		if flowCtx.flow == nil {
			// nil means there's no data for this flow context. Check if it's expired
			// since we're iterating through the map anyways.
			if isFlowCtxExpired(flowCtx, f.flowContextTTL, now) {
				expiredFlowKeys = append(expiredFlowKeys, key)
			}

			// Added to support legacy behavior. Keep the same cadence for flushes.
			if !flowCtx.nextFlush.After(now) {
				flowCtx.nextFlush = f.scheduler.RefreshFlushTime(flowCtx)
				f.flows[key] = flowCtx
			}

			continue
		}

		if flowCtx.nextFlush.After(now) {
			continue
		}

		flowsToFlush = append(flowsToFlush, flowCtx.flow)

		flowCtx.lastSuccessfulFlush = now
		flowCtx.flow = nil
		flowCtx.nextFlush = f.scheduler.RefreshFlushTime(flowCtx)
		f.flows[key] = flowCtx
	}

	for _, key := range expiredFlowKeys {
		delete(f.flows, key)
	}

	return flowsToFlush
}

// flushGroups flushes reporters in deduplication mode, returning one FlowGroup per 5-tuple.
// A group flushes when any active reporter's nextFlush is due; all reporters in the group
// flush together. GhostReporters are 0-byte snapshots from the previous flush cycle and
// are included so the platform can use them as metadata for flow_role assignment.
func (f *flowAccumulator) flushGroups(flushContext common.FlushContext) []FlowGroup {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	now := flushContext.FlushTime
	var result []FlowGroup
	var emptyGroups []uint64

	for fiveTupleHash, reporterHashes := range f.fiveTupleGroups {
		// Check if any active reporter in this group is due to flush.
		groupReady := false
		for _, hash := range reporterHashes {
			ctx, ok := f.flows[hash]
			if !ok || ctx.flow == nil {
				continue
			}
			if !ctx.nextFlush.After(now) {
				groupReady = true
				break
			}
		}
		if !groupReady {
			continue
		}

		var activeFlows []*common.Flow
		var liveHashes []uint64

		for _, hash := range reporterHashes {
			ctx, ok := f.flows[hash]
			if !ok {
				continue
			}

			if ctx.flow == nil {
				// Dead context: clean up if TTL expired, otherwise keep for scheduling.
				if isFlowCtxExpired(ctx, f.flowContextTTL, now) {
					delete(f.flows, hash)
				} else {
					if !ctx.nextFlush.After(now) {
						ctx.nextFlush = f.scheduler.RefreshFlushTime(ctx)
						f.flows[hash] = ctx
					}
					liveHashes = append(liveHashes, hash)
				}
				continue
			}

			liveHashes = append(liveHashes, hash)

			if !ctx.nextFlush.After(now) {
				flowCopy := *ctx.flow
				activeFlows = append(activeFlows, &flowCopy)
				ctx.lastSuccessfulFlush = now
				ctx.flow = nil
				ctx.nextFlush = f.scheduler.RefreshFlushTime(ctx)
				f.flows[hash] = ctx
			}
		}

		if len(activeFlows) > 0 {
			result = append(result, FlowGroup{
				Reporters:      activeFlows,
				GhostReporters: f.prevCycleReporters[fiveTupleHash],
			})
			// Store 0-byte snapshots so they become GhostReporters on the next flush.
			f.prevCycleReporters[fiveTupleHash] = zeroedSnapshots(activeFlows)
		}

		if len(liveHashes) == 0 {
			emptyGroups = append(emptyGroups, fiveTupleHash)
		} else {
			f.fiveTupleGroups[fiveTupleHash] = liveHashes
		}
	}

	for _, key := range emptyGroups {
		delete(f.fiveTupleGroups, key)
		delete(f.prevCycleReporters, key)
	}

	return result
}

// zeroedSnapshots returns copies of the given flows with Bytes and Packets set to zero.
// These are stored as prevCycleReporters and surfaced as GhostReporters on the next flush.
func zeroedSnapshots(flows []*common.Flow) []*common.Flow {
	snapshots := make([]*common.Flow, len(flows))
	for i, f := range flows {
		cp := *f
		cp.Bytes = 0
		cp.Packets = 0
		snapshots[i] = &cp
	}
	return snapshots
}

func isFlowCtxExpired(flowCtx flowContext, flowTTL time.Duration, now time.Time) bool {
	flowExpiresAt := flowCtx.lastSuccessfulFlush.Add(flowTTL)
	return now.After(flowExpiresAt)
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

	aggHash := flowToAdd.AggregationHash()
	aggFlow, ok := f.flows[aggHash]
	if !ok {
		nextFlush := f.scheduler.ScheduleNewFlowFlush(timeNow())

		if f.shouldUseDeduplication {
			fiveTupleHash := flowToAdd.FiveTupleHash()
			// New reporter joining an existing group: inherit the group's flush time so
			// all reporters flush together rather than at independently jittered times.
			for _, h := range f.fiveTupleGroups[fiveTupleHash] {
				if existingCtx, exists := f.flows[h]; exists && existingCtx.flow != nil {
					nextFlush = existingCtx.nextFlush
					break
				}
			}
			f.fiveTupleGroups[fiveTupleHash] = append(f.fiveTupleGroups[fiveTupleHash], aggHash)
		}

		f.flows[aggHash] = flowContext{
			flow:      flowToAdd,
			nextFlush: nextFlush,
		}
		f.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
		return
	}
	if aggFlow.flow == nil {
		// flowToAdd is for the same hash as an aggregated flow that has been flushed
		aggFlow.flow = flowToAdd
		f.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
	} else {
		// use go routine for hash collision detection to avoid blocking critical path
		go f.detectHashCollision(aggHash, *aggFlow.flow, *flowToAdd)

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
