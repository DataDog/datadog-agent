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
	// isGhost is true when the flow has been flushed but is being kept alive for one
	// additional cycle in deduplication mode. Ghost reporters have Bytes == 0 and
	// Packets == 0 and are included in the next merged event as metadata. A ghost
	// becomes active again if new data arrives before the next flush.
	isGhost bool
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

	// deduplicationEnabled groups flows by 5-tuple. When true, fiveTupleGroups indexes
	// the per-reporter full-hash entries so they can be flushed as a single merged event.
	deduplicationEnabled bool
	// fiveTupleGroups maps a FiveTupleHash to the set of full AggregationHashes (one per
	// reporter) that belong to that group. Only populated when deduplicationEnabled is true.
	fiveTupleGroups map[uint64][]uint64

	logger      log.Component
	rdnsQuerier rdnsquerier.Component
}

func newFlowAccumulator(flushConfig common.FlushConfig, flowScheduler FlowScheduler, aggregatorFlowContextTTL time.Duration, portRollupThreshold int, portRollupDisabled bool, deduplicationEnabled bool, logger log.Component, rdnsQuerier rdnsquerier.Component) *flowAccumulator {
	acc := &flowAccumulator{
		flows:                  make(map[uint64]flowContext),
		FlushConfig:            flushConfig,
		flowContextTTL:         aggregatorFlowContextTTL,
		portRollup:             portrollup.NewEndpointPairPortRollupStore(portRollupThreshold),
		portRollupThreshold:    portRollupThreshold,
		portRollupDisabled:     portRollupDisabled,
		deduplicationEnabled:   deduplicationEnabled,
		hashCollisionFlowCount: atomic.NewUint64(0),
		scheduler:              flowScheduler,
		logger:                 logger,
		rdnsQuerier:            rdnsQuerier,
	}
	if deduplicationEnabled {
		acc.fiveTupleGroups = make(map[uint64][]uint64)
	}
	return acc
}

// flush will flush flow contexts that are due, returning the flows to send.
// Each context flushes independently based on its scheduled nextFlush time.
// Use flushGroups instead when deduplication is enabled.
func (f *flowAccumulator) flush(flushContext common.FlushContext) []*common.Flow {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	return f.flushStandard(flushContext.FlushTime)
}

// flushStandard is the original per-flow flush logic.
//
// flowContextTTL:
// flowContextTTL defines the duration we should keep a specific flowContext in `flowAccumulator.flows`
// after `lastSuccessfulFlush`. Flow context in `flowAccumulator.flows` map will be deleted if `flowContextTTL`
// is reached to avoid keeping flow context that are not seen anymore.
// We need to keep flowContext (contains `nextFlush` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flowFlushInterval`).
// Example, after a flush, flowContext will have a new nextFlush, that will be the next flush time for new flows being added.
func (f *flowAccumulator) flushStandard(now time.Time) []*common.Flow {
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

// flushGroups flushes reporters in deduplication mode, returning one slice of flows per
// 5-tuple group. A group flushes when any active (non-ghost) reporter's nextFlush is due;
// all reporters in the group are included in that flush together.
//
// Active reporters that flush are kept alive as ghosts (Bytes == 0, Packets == 0) for one
// additional cycle so the platform can use them as metadata when assigning flow_role.
// Ghost reporters are included in the next flush, then transitioned to dead (flow = nil).
func (f *flowAccumulator) flushGroups(flushContext common.FlushContext) [][]*common.Flow {
	f.flowsMutex.Lock()
	defer f.flowsMutex.Unlock()

	now := flushContext.FlushTime

	// groupFlows collects the reporters to emit per 5-tuple; groupOrder preserves
	// first-seen ordering for deterministic output.
	groupFlows := make(map[uint64][]*common.Flow)
	var groupOrder []uint64
	var emptyGroups []uint64

	for fiveTupleHash, reporterHashes := range f.fiveTupleGroups {
		// Check if any active (non-ghost) reporter in this group is due to flush.
		groupReady := false
		for _, hash := range reporterHashes {
			ctx, ok := f.flows[hash]
			if !ok || ctx.flow == nil || ctx.isGhost {
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

			if _, exists := groupFlows[fiveTupleHash]; !exists {
				groupOrder = append(groupOrder, fiveTupleHash)
			}

			if ctx.isGhost {
				// Include ghost (Bytes == 0) as metadata for the platform, then mark dead.
				groupFlows[fiveTupleHash] = append(groupFlows[fiveTupleHash], ctx.flow)
				ctx.lastSuccessfulFlush = now
				ctx.flow = nil
				ctx.isGhost = false
				ctx.nextFlush = f.scheduler.RefreshFlushTime(ctx)
				f.flows[hash] = ctx
			} else if !ctx.nextFlush.After(now) {
				// Active reporter: copy the flow for the payload, then zero bytes/packets
				// and keep alive as a ghost for one additional cycle.
				flowCopy := *ctx.flow
				groupFlows[fiveTupleHash] = append(groupFlows[fiveTupleHash], &flowCopy)
				ctx.flow.Bytes = 0
				ctx.flow.Packets = 0
				ctx.isGhost = true
				ctx.lastSuccessfulFlush = now
				ctx.nextFlush = f.scheduler.RefreshFlushTime(ctx)
				f.flows[hash] = ctx
			}
			// Reporters whose nextFlush is still in the future are left unchanged.
		}

		if len(liveHashes) == 0 {
			emptyGroups = append(emptyGroups, fiveTupleHash)
		} else {
			f.fiveTupleGroups[fiveTupleHash] = liveHashes
		}
	}

	for _, key := range emptyGroups {
		delete(f.fiveTupleGroups, key)
	}

	result := make([][]*common.Flow, 0, len(groupOrder))
	for _, h := range groupOrder {
		result = append(result, groupFlows[h])
	}
	return result
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

		if f.deduplicationEnabled {
			fiveTupleHash := flowToAdd.FiveTupleHash()
			// New reporter joining an existing group: inherit the group's flush time so
			// all reporters flush together rather than at independently jittered times.
			for _, h := range f.fiveTupleGroups[fiveTupleHash] {
				if existingCtx, exists := f.flows[h]; exists && existingCtx.flow != nil && !existingCtx.isGhost {
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
		// flowToAdd is for the same hash as an aggregated flow that has been flushed (dead state).
		aggFlow.flow = flowToAdd
		aggFlow.isGhost = false
		f.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
	} else {
		// use go routine for hash collision detection to avoid blocking critical path
		go f.detectHashCollision(aggHash, *aggFlow.flow, *flowToAdd)

		if aggFlow.isGhost {
			// Reactivate: a late-arriving flow woke up the ghost. Clear ghost state so
			// the accumulation below adds on top of the zeroed bytes/packets correctly.
			aggFlow.isGhost = false
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
