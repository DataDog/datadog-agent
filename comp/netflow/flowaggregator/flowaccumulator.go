// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"slices"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/portrollup"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	"go.uber.org/atomic"
)

var timeNow = time.Now

// FlowBatch is the flush result type for the standard accumulator.
type FlowBatch = []*common.Flow

// FlowGroupBatch is the flush result type for the dedup accumulator.
type FlowGroupBatch = []FlowGroup

// FlowAccumulator accumulates incoming flows and flushes them on a schedule.
// The type parameter T determines the shape of the flush result:
//   - FlowBatch for standard (per-flow) mode
//   - FlowGroupBatch for deduplication (per-5-tuple) mode
type FlowAccumulator[T any] interface {
	Add(*common.Flow)
	Flush(common.FlushContext) T
	GetFlowContextCount() int
	PortRollup() *portrollup.EndpointPairPortRollupStore
	HashCollisionCount() *atomic.Uint64
}

// FlowGroup represents all reporters for a single 5-tuple, as returned by the
// dedup accumulator's Flush. Reporters observed data this cycle; GhostReporters
// are 0-byte snapshots from the previous cycle included so the platform can use
// them as metadata when assigning flow_role.
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

// ---------------------------------------------------------------------------
// flowAccumulatorBase: shared state and helpers for both accumulator variants
// ---------------------------------------------------------------------------

// flowAccumulatorBase holds the fields and helper methods shared by both the
// standard and dedup accumulator implementations.
type flowAccumulatorBase struct {
	flows map[uint64]flowContext
	// mutex is needed to protect `flows` since Add() and Flush()
	// are called by different routines.
	flowsMutex sync.Mutex

	flushConfig    common.FlushConfig
	flowContextTTL time.Duration
	scheduler      FlowScheduler

	portRollup          *portrollup.EndpointPairPortRollupStore
	portRollupThreshold int
	portRollupDisabled  bool

	hashCollisionFlowCount *atomic.Uint64

	logger      log.Component
	rdnsQuerier rdnsquerier.Component
}

func newFlowAccumulatorBase(flushConfig common.FlushConfig, flowScheduler FlowScheduler, flowContextTTL time.Duration, portRollupThreshold int, portRollupDisabled bool, logger log.Component, rdnsQuerier rdnsquerier.Component) flowAccumulatorBase {
	return flowAccumulatorBase{
		flows:                  make(map[uint64]flowContext),
		flushConfig:            flushConfig,
		flowContextTTL:         flowContextTTL,
		portRollup:             portrollup.NewEndpointPairPortRollupStore(portRollupThreshold),
		portRollupThreshold:    portRollupThreshold,
		portRollupDisabled:     portRollupDisabled,
		hashCollisionFlowCount: atomic.NewUint64(0),
		scheduler:              flowScheduler,
		logger:                 logger,
		rdnsQuerier:            rdnsQuerier,
	}
}

// GetFlowContextCount returns the number of flow contexts currently tracked.
func (b *flowAccumulatorBase) GetFlowContextCount() int {
	b.flowsMutex.Lock()
	defer b.flowsMutex.Unlock()
	return len(b.flows)
}

// PortRollup returns the port rollup store.
func (b *flowAccumulatorBase) PortRollup() *portrollup.EndpointPairPortRollupStore {
	return b.portRollup
}

// HashCollisionCount returns the hash collision counter.
func (b *flowAccumulatorBase) HashCollisionCount() *atomic.Uint64 {
	return b.hashCollisionFlowCount
}

// applyPortRollup rewrites ephemeral ports on the flow based on the port rollup store.
// Must be called before computing any hash that depends on ports.
func (b *flowAccumulatorBase) applyPortRollup(flowToAdd *common.Flow) {
	if b.portRollupDisabled {
		return
	}
	b.portRollup.Add(flowToAdd.SrcAddr, flowToAdd.DstAddr, uint16(flowToAdd.SrcPort), uint16(flowToAdd.DstPort))
	ephemeralStatus := b.portRollup.IsEphemeral(flowToAdd.SrcAddr, flowToAdd.DstAddr, uint16(flowToAdd.SrcPort), uint16(flowToAdd.DstPort))
	switch ephemeralStatus {
	case portrollup.IsEphemeralSourcePort:
		flowToAdd.SrcPort = portrollup.EphemeralPort
	case portrollup.IsEphemeralDestPort:
		flowToAdd.DstPort = portrollup.EphemeralPort
	}
}

// accumulateInto merges flowToAdd's counters and metadata into an existing flow context.
// Caller must hold flowsMutex.
func (b *flowAccumulatorBase) accumulateInto(aggHash uint64, existing *common.Flow, flowToAdd *common.Flow) {
	go b.detectHashCollision(aggHash, *existing, *flowToAdd)

	existing.Bytes += flowToAdd.Bytes
	existing.Packets += flowToAdd.Packets
	existing.StartTimestamp = common.Min(existing.StartTimestamp, flowToAdd.StartTimestamp)
	existing.EndTimestamp = common.Max(existing.EndTimestamp, flowToAdd.EndTimestamp)
	existing.SequenceNum = common.Max(existing.SequenceNum, flowToAdd.SequenceNum)
	existing.TCPFlags |= flowToAdd.TCPFlags

	if flowToAdd.AdditionalFields != nil {
		if existing.AdditionalFields == nil {
			existing.AdditionalFields = make(common.AdditionalFields)
		}
		for field, value := range flowToAdd.AdditionalFields {
			if _, ok := existing.AdditionalFields[field]; !ok {
				existing.AdditionalFields[field] = value
			}
		}
	}
}

func (b *flowAccumulatorBase) setSrcReverseDNSHostname(aggHash uint64, hostname string, acquireLock bool) {
	if hostname == "" {
		return
	}

	if acquireLock {
		b.flowsMutex.Lock()
		defer b.flowsMutex.Unlock()
	}

	aggFlow, ok := b.flows[aggHash]
	if ok && aggFlow.flow != nil {
		aggFlow.flow.SrcReverseDNSHostname = hostname
	}
}

func (b *flowAccumulatorBase) setDstReverseDNSHostname(aggHash uint64, hostname string, acquireLock bool) {
	if hostname == "" {
		return
	}

	if acquireLock {
		b.flowsMutex.Lock()
		defer b.flowsMutex.Unlock()
	}

	aggFlow, ok := b.flows[aggHash]
	if ok && aggFlow.flow != nil {
		aggFlow.flow.DstReverseDNSHostname = hostname
	}
}

func (b *flowAccumulatorBase) addRDNSEnrichment(aggHash uint64, srcAddr []byte, dstAddr []byte) {
	err := b.rdnsQuerier.GetHostnameAsync(
		srcAddr,
		// Sync callback, lock is already held
		func(hostname string) {
			b.setSrcReverseDNSHostname(aggHash, hostname, false)
		},
		// Async callback will reacquire the lock
		func(hostname string, err error) {
			if err != nil {
				b.logger.Debugf("Error resolving reverse DNS enrichment for source IP address: %v error: %v", srcAddr, err)
				return
			}
			b.setSrcReverseDNSHostname(aggHash, hostname, true)
		},
	)
	if err != nil {
		b.logger.Debugf("Error requesting reverse DNS enrichment for source IP address: %v error: %v", srcAddr, err)
	}

	err = b.rdnsQuerier.GetHostnameAsync(
		dstAddr,
		// Sync callback, lock is held
		func(hostname string) {
			b.setDstReverseDNSHostname(aggHash, hostname, false)
		},
		// Async callback will reacquire the lock
		func(hostname string, err error) {
			if err != nil {
				b.logger.Debugf("Error resolving reverse DNS enrichment for destination IP address: %v error: %v", dstAddr, err)
				return
			}
			b.setDstReverseDNSHostname(aggHash, hostname, true)
		},
	)
	if err != nil {
		b.logger.Debugf("Error requesting reverse DNS enrichment for destination IP address: %v error: %v", dstAddr, err)
	}
}

func (b *flowAccumulatorBase) detectHashCollision(hash uint64, existingFlow common.Flow, flowToAdd common.Flow) {
	if !common.IsEqualPerReporterContext(existingFlow, flowToAdd) {
		b.logger.Warnf("Hash collision for flows with hash `%d`: existingFlow=`%+v` flowToAdd=`%+v`", hash, existingFlow, flowToAdd)
		b.hashCollisionFlowCount.Inc()
	}
}

func isFlowCtxExpired(flowCtx flowContext, flowTTL time.Duration, now time.Time) bool {
	flowExpiresAt := flowCtx.lastSuccessfulFlush.Add(flowTTL)
	return now.After(flowExpiresAt)
}

// ---------------------------------------------------------------------------
// standardFlowAccumulator — flush produces a flat list of flows
// ---------------------------------------------------------------------------

// standardFlowAccumulator implements FlowAccumulator[[]*common.Flow].
// Each flow context flushes independently based on its scheduled nextFlush time.
type standardFlowAccumulator struct {
	flowAccumulatorBase
}

var _ FlowAccumulator[[]*common.Flow] = (*standardFlowAccumulator)(nil)

func newStandardFlowAccumulator(flushConfig common.FlushConfig, flowScheduler FlowScheduler, flowContextTTL time.Duration, portRollupThreshold int, portRollupDisabled bool, logger log.Component, rdnsQuerier rdnsquerier.Component) *standardFlowAccumulator {
	return &standardFlowAccumulator{
		flowAccumulatorBase: newFlowAccumulatorBase(flushConfig, flowScheduler, flowContextTTL, portRollupThreshold, portRollupDisabled, logger, rdnsQuerier),
	}
}

// Add accumulates a flow into the standard (per-reporter) accumulator.
func (s *standardFlowAccumulator) Add(flowToAdd *common.Flow) {
	s.logger.Tracef("Add new flow: %+v", flowToAdd)
	s.applyPortRollup(flowToAdd)

	s.flowsMutex.Lock()
	defer s.flowsMutex.Unlock()

	aggHash := flowToAdd.PerReporterHash()
	aggFlow, ok := s.flows[aggHash]
	if !ok {
		s.flows[aggHash] = flowContext{
			flow:      flowToAdd,
			nextFlush: s.scheduler.ScheduleNewFlowFlush(timeNow()),
		}
		s.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
		return
	}
	if aggFlow.flow == nil {
		aggFlow.flow = flowToAdd
		s.addRDNSEnrichment(aggHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
	} else {
		s.accumulateInto(aggHash, aggFlow.flow, flowToAdd)
	}
	s.flows[aggHash] = aggFlow
}

// Flush flushes flow contexts that are due, returning the flows to send.
// Each context flushes independently based on its scheduled nextFlush time.
//
// flowContextTTL defines the duration we should keep a specific flowContext in `flows`
// after `lastSuccessfulFlush`. Flow context will be deleted if `flowContextTTL`
// is reached to avoid keeping flow context that are not seen anymore.
// We need to keep flowContext (contains `nextFlush` and `lastSuccessfulFlush`) after flush
// to be able to flush at regular interval (`flowFlushInterval`).
func (s *standardFlowAccumulator) Flush(flushContext common.FlushContext) []*common.Flow {
	s.flowsMutex.Lock()
	defer s.flowsMutex.Unlock()

	now := flushContext.FlushTime
	var flowsToFlush []*common.Flow
	var expiredFlowKeys []uint64

	for key, flowCtx := range s.flows {
		if flowCtx.flow == nil {
			if isFlowCtxExpired(flowCtx, s.flowContextTTL, now) {
				expiredFlowKeys = append(expiredFlowKeys, key)
			}

			// Added to support legacy behavior. Keep the same cadence for flushes.
			if !flowCtx.nextFlush.After(now) {
				flowCtx.nextFlush = s.scheduler.RefreshFlushTime(flowCtx)
				s.flows[key] = flowCtx
			}

			continue
		}

		if flowCtx.nextFlush.After(now) {
			continue
		}

		flowsToFlush = append(flowsToFlush, flowCtx.flow)

		flowCtx.lastSuccessfulFlush = now
		flowCtx.flow = nil
		flowCtx.nextFlush = s.scheduler.RefreshFlushTime(flowCtx)
		s.flows[key] = flowCtx
	}

	for _, key := range expiredFlowKeys {
		delete(s.flows, key)
	}

	return flowsToFlush
}

// ---------------------------------------------------------------------------
// dedupFlowAccumulator — flush produces one FlowGroup per 5-tuple
// ---------------------------------------------------------------------------

// dedupFlowAccumulator implements FlowAccumulator[[]FlowGroup].
// Flows are grouped by 5-tuple; when any reporter in a group is due, the entire
// group flushes together as a single FlowGroup.
type dedupFlowAccumulator struct {
	flowAccumulatorBase

	// fiveTupleGroups maps a DeduplicationHash to the set of full PerReporterHashes (one per
	// reporter) that belong to that group.
	fiveTupleGroups map[uint64][]uint64

	// prevCycleReporters holds 0-byte snapshots of reporters from the most recent flush of
	// each group. These are returned as GhostReporters in the next flush so the platform can
	// use them as metadata for flow_role assignment. Keyed by DeduplicationHash.
	prevCycleReporters map[uint64][]*common.Flow
}

// Compile-time assertion that dedupFlowAccumulator implements FlowAccumulator[[]FlowGroup].
var _ FlowAccumulator[[]FlowGroup] = (*dedupFlowAccumulator)(nil)

func newDedupFlowAccumulator(flushConfig common.FlushConfig, flowScheduler FlowScheduler, flowContextTTL time.Duration, portRollupThreshold int, portRollupDisabled bool, logger log.Component, rdnsQuerier rdnsquerier.Component) *dedupFlowAccumulator {
	return &dedupFlowAccumulator{
		flowAccumulatorBase: newFlowAccumulatorBase(flushConfig, flowScheduler, flowContextTTL, portRollupThreshold, portRollupDisabled, logger, rdnsQuerier),
		fiveTupleGroups:     make(map[uint64][]uint64),
		prevCycleReporters:  make(map[uint64][]*common.Flow),
	}
}

// Add accumulates a flow into the dedup accumulator. New reporters joining an
// existing 5-tuple group inherit the group's flush time so all reporters in the
// group flush together.
func (d *dedupFlowAccumulator) Add(flowToAdd *common.Flow) {
	d.logger.Tracef("Add new flow: %+v", flowToAdd)
	d.applyPortRollup(flowToAdd)

	// Compute the dedup hash after port rollup so ephemeral port rewrites are reflected.
	dedupHash := flowToAdd.DeduplicationHash()

	d.flowsMutex.Lock()
	defer d.flowsMutex.Unlock()

	reporterHash := flowToAdd.PerReporterHash()
	aggFlow, ok := d.flows[reporterHash]
	if !ok {
		// First time seeing this reporter. Schedule its flush, inheriting the
		// group's existing schedule if one exists so all reporters flush together.
		nextFlush := d.scheduler.ScheduleNewFlowFlush(timeNow())
		for _, h := range d.fiveTupleGroups[dedupHash] {
			if existingCtx, exists := d.flows[h]; exists {
				nextFlush = existingCtx.nextFlush
				break
			}
		}
		d.fiveTupleGroups[dedupHash] = append(d.fiveTupleGroups[dedupHash], reporterHash)
		d.flows[reporterHash] = flowContext{
			flow:      flowToAdd,
			nextFlush: nextFlush,
		}
		d.addRDNSEnrichment(reporterHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
		return
	}
	if aggFlow.flow == nil {
		aggFlow.flow = flowToAdd
		d.addRDNSEnrichment(reporterHash, flowToAdd.SrcAddr, flowToAdd.DstAddr)
	} else {
		d.accumulateInto(reporterHash, aggFlow.flow, flowToAdd)
	}
	d.flows[reporterHash] = aggFlow
}

// Flush flushes reporters in deduplication mode, returning one FlowGroup per 5-tuple.
// A group flushes when any active reporter's nextFlush is due; all reporters in the group
// flush together. GhostReporters are 0-byte snapshots from the previous flush cycle and
// are included so the platform can use them as metadata for flow_role assignment.
func (d *dedupFlowAccumulator) Flush(flushContext common.FlushContext) []FlowGroup {
	d.flowsMutex.Lock()
	defer d.flowsMutex.Unlock()

	now := flushContext.FlushTime
	var result []FlowGroup
	var emptyGroups []uint64

	for fiveTupleHash, reporterHashes := range d.fiveTupleGroups {
		// Check if any active reporter in this group is due to flush.
		groupReady := slices.ContainsFunc(reporterHashes, func(hash uint64) bool {
			ctx, ok := d.flows[hash]
			if !ok {
				return false
			} else if ctx.flow == nil {
				return false
			}

			return !ctx.nextFlush.After(now)
		})
		if !groupReady {
			continue
		}

		var activeFlows []*common.Flow
		var liveHashes []uint64

		for _, hash := range reporterHashes {
			ctx, ok := d.flows[hash]
			if !ok {
				continue
			}

			if ctx.flow == nil {
				// Dead context: clean up if TTL expired, otherwise keep for scheduling.
				if isFlowCtxExpired(ctx, d.flowContextTTL, now) {
					delete(d.flows, hash)
				} else {
					if !ctx.nextFlush.After(now) {
						ctx.nextFlush = d.scheduler.RefreshFlushTime(ctx)
						d.flows[hash] = ctx
					}
					liveHashes = append(liveHashes, hash)
				}
				continue
			}

			liveHashes = append(liveHashes, hash)

			// Once the group is ready, flush ALL reporters that have data — not just
			// those individually due. This ensures reporters with slightly offset
			// flush times (e.g. a late joiner) are still sent together for dedup.
			flowCopy := *ctx.flow
			activeFlows = append(activeFlows, &flowCopy)
			ctx.lastSuccessfulFlush = now
			ctx.flow = nil
			ctx.nextFlush = d.scheduler.RefreshFlushTime(ctx)
			d.flows[hash] = ctx
		}

		if len(activeFlows) > 0 {
			result = append(result, FlowGroup{
				Reporters:      activeFlows,
				GhostReporters: d.prevCycleReporters[fiveTupleHash],
			})
			// Store 0-byte snapshots so they become GhostReporters on the next flush.
			d.prevCycleReporters[fiveTupleHash] = zeroedSnapshots(activeFlows)
		} else {
			// No active reporters this cycle — discard stale ghost data so it
			// doesn't accumulate indefinitely for groups with only dead contexts.
			delete(d.prevCycleReporters, fiveTupleHash)
		}

		if len(liveHashes) == 0 {
			emptyGroups = append(emptyGroups, fiveTupleHash)
		} else {
			d.fiveTupleGroups[fiveTupleHash] = liveHashes
		}
	}

	for _, key := range emptyGroups {
		delete(d.fiveTupleGroups, key)
		delete(d.prevCycleReporters, key)
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
