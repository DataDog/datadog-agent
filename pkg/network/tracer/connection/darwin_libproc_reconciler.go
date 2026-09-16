// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"net/netip"
	"sync"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/libproc"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	darwinLibprocInterval        = 10 * time.Second
	darwinLibprocStartTimeLeeway = time.Second
	libprocHostWalkMinTicks      = 3
	libprocTransientCap          = 3

	libprocBitLocalPresent  uint8 = 1 << 0
	libprocBitLocalAddr     uint8 = 1 << 1
	libprocBitLocalPort     uint8 = 1 << 2
	libprocBitRemotePresent uint8 = 1 << 3
	libprocBitRemoteAddr    uint8 = 1 << 4
	libprocBitRemotePort    uint8 = 1 << 5
)

var darwinLibprocTelemetry = struct {
	scans         telemetry.Counter
	hostWalks     telemetry.Counter
	targetedScans telemetry.Counter
	skips         telemetry.Counter
	stillNeeded   telemetry.Gauge
	errors        telemetry.Counter
	truncated     telemetry.Counter
	resolved      telemetry.Counter
	ambiguous     telemetry.Counter
	reuseReject   telemetry.Counter
}{
	scans:         telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "scans", nil, "Bounded libproc reconciliation scans"),
	hostWalks:     telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "host_walks", nil, "Host-wide libproc scans"),
	targetedScans: telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "targeted_scans", nil, "Per-PID libproc scans"),
	skips:         telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "skips", nil, "Libproc ticks that issued no scan"),
	stillNeeded:   telemetryimpl.GetCompatComponent().NewGauge("network_tracer__darwin_libproc", "still_needed", nil, "NStat sources that still need libproc at classify"),
	errors:        telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "errors", nil, "Failed libproc reconciliation scans"),
	truncated:     telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "truncated", nil, "Libproc host walks stopped at a host-wide bound"),
	resolved:      telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "resolved", nil, "NStat sources resolved through direct libproc evidence"),
	ambiguous:     telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "ambiguous", nil, "Libproc ownership candidates rejected as ambiguous"),
	reuseReject:   telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "pid_reuse_rejected", nil, "Libproc candidates rejected by process start time"),
}

type darwinLibprocReconciler struct {
	scanner  libproc.Scanner
	primary  *nstatTracer
	interval time.Duration

	exit     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func newDarwinLibprocReconciler(scanner libproc.Scanner, primary *nstatTracer, interval time.Duration) *darwinLibprocReconciler {
	return &darwinLibprocReconciler{
		scanner:  scanner,
		primary:  primary,
		interval: interval,
		exit:     make(chan struct{}),
	}
}

func (r *darwinLibprocReconciler) start() {
	if r == nil || r.scanner == nil || r.primary == nil || r.interval <= 0 {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := r.runOnce(); err != nil {
			log.Debugf("initial Darwin libproc reconciliation failed: %v", err)
		}
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := r.runOnce(); err != nil {
					log.Debugf("Darwin libproc reconciliation failed: %v", err)
				}
			case <-r.exit:
				return
			}
		}
	}()
}

type libprocScanScope struct {
	scanStart time.Time
	hostWide  bool
	pid       uint32
}

type libprocWorkPlan struct {
	skip        bool
	hostWalk    bool
	pids        []uint32
	stillNeeded int
	scanStart   time.Time
}

func (r *darwinLibprocReconciler) runOnce() error {
	plan := r.primary.classifyLibprocWork()
	darwinLibprocTelemetry.stillNeeded.Set(float64(plan.stillNeeded))
	if plan.skip {
		darwinLibprocTelemetry.skips.Inc()
		return nil
	}
	if plan.hostWalk {
		return r.runHostWalk(plan.scanStart)
	}
	return r.runTargetedScans(plan.scanStart, plan.pids)
}

func (r *darwinLibprocReconciler) runHostWalk(scanStart time.Time) error {
	darwinLibprocTelemetry.scans.Inc()
	darwinLibprocTelemetry.hostWalks.Inc()
	snapshot, err := r.scanner.Scan()
	if err != nil {
		darwinLibprocTelemetry.errors.Inc()
		return err
	}
	if snapshot.HostWideTruncated {
		darwinLibprocTelemetry.truncated.Inc()
	}
	r.reportReconcile(r.primary.reconcileLibprocSnapshot(snapshot, libprocScanScope{
		scanStart: scanStart,
		hostWide:  true,
	}))
	return nil
}

func (r *darwinLibprocReconciler) runTargetedScans(scanStart time.Time, pids []uint32) error {
	var lastErr error
	for _, pid := range pids {
		darwinLibprocTelemetry.scans.Inc()
		darwinLibprocTelemetry.targetedScans.Inc()
		snapshot, err := r.scanner.ScanPID(pid)
		if err != nil {
			darwinLibprocTelemetry.errors.Inc()
			lastErr = err
			continue
		}
		r.reportReconcile(r.primary.reconcileLibprocSnapshot(snapshot, libprocScanScope{
			scanStart: scanStart,
			pid:       pid,
		}))
	}
	return lastErr
}

func (r *darwinLibprocReconciler) reportReconcile(resolved, ambiguous, reuseRejected int) {
	darwinLibprocTelemetry.resolved.Add(float64(resolved))
	darwinLibprocTelemetry.ambiguous.Add(float64(ambiguous))
	darwinLibprocTelemetry.reuseReject.Add(float64(reuseRejected))
}

func (r *darwinLibprocReconciler) stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.exit) })
	r.wg.Wait()
}

type darwinProcessIdentity struct {
	pid       uint32
	startTime uint64
}

type darwinLibprocCandidateIdentity struct {
	process darwinProcessIdentity
	tuple   network.ConnectionTuple
}

func (t *nstatTracer) classifyLibprocWork() libprocWorkPlan {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.libprocTick++
	plan := libprocWorkPlan{scanStart: t.now()}
	pids := make(map[uint32]struct{})
	var anyNeed bool
	var pid0Eligible bool
	for _, source := range t.sources {
		if !sourceNeedsLibprocReconciliation(source) {
			continue
		}
		anyNeed = true
		plan.stillNeeded++
		flow := source.flow
		current := nstatTupleFingerprint(flow)
		if flow.PID == 0 {
			if source.hostWalkStop && tupleMoreComplete(current, source.hostWalkBits) {
				source.hostWalkTruncated = 0
			}
			if !source.hostWalkStop || tupleMoreComplete(current, source.hostWalkBits) {
				pid0Eligible = true
			}
			continue
		}
		if tupleMoreComplete(current, source.targetedBits) {
			source.targetedTransient = 0
		}
		if source.targetedPID == 0 || flow.PID != source.targetedPID || tupleMoreComplete(current, source.targetedBits) {
			pids[flow.PID] = struct{}{}
		}
	}
	if !anyNeed {
		plan.skip = true
		return plan
	}
	if t.lastHostWalkTick == 0 ||
		(pid0Eligible && t.libprocTick-t.lastHostWalkTick >= libprocHostWalkMinTicks) {
		plan.hostWalk = true
		return plan
	}
	if len(pids) == 0 {
		plan.skip = true
		return plan
	}
	plan.pids = make([]uint32, 0, len(pids))
	for pid := range pids {
		plan.pids = append(plan.pids, pid)
	}
	return plan
}

func (t *nstatTracer) reconcileLibprocSnapshot(snapshot libproc.Snapshot, scope libprocScanScope) (resolved, ambiguous, reuseRejected int) {
	useIndex := scope.hostWide
	var index darwinLibprocIndex
	if useIndex {
		index = indexDarwinLibprocObservations(snapshot.Observations)
	}
	var closed []*network.ConnectionStats
	t.mu.Lock()
	for sourceRef, source := range t.sources {
		if source == nil || source.flow == nil {
			continue
		}
		if !scope.hostWide && source.flow.PID != 0 && source.flow.PID != scope.pid {
			continue
		}
		if !sourceNeedsLibprocReconciliation(source) {
			continue
		}
		observations := snapshot.Observations
		if useIndex {
			observations = index.candidates(tupleFromNStatFlow(source))
		}
		candidate, status := matchDarwinLibprocSource(source, observations)
		var outcome libprocOutcome
		switch status {
		case darwinLibprocAmbiguous:
			ambiguous++
			outcome = libprocOutcomeAmbiguous
		case darwinLibprocNoMatch:
			outcome = libprocOutcomeMiss
		default:
			if candidate.ProcessStartTime != 0 && !source.createdAt.IsZero() &&
				candidate.ProcessStartTime > uint64(source.createdAt.Add(darwinLibprocStartTimeLeeway).UnixNano()) {
				reuseRejected++
				outcome = libprocOutcomeReuseReject
			} else {
				t.applyLibprocEvidence(sourceRef, source, candidate)
				resolved++
				outcome = libprocOutcomeApplied
				t.markLibprocAttempt(source, scope, snapshot, outcome)
				if source.removed && source.conn != nil {
					closed = append(closed, t.closeAndRemoveSource(sourceRef, source))
				}
				continue
			}
		}
		t.markLibprocAttempt(source, scope, snapshot, outcome)
	}
	if scope.hostWide {
		t.lastHostWalkTick = t.libprocTick
	}
	callback := t.closeCallback
	t.mu.Unlock()

	if callback != nil {
		for _, conn := range closed {
			callback(conn)
		}
	}
	return resolved, ambiguous, reuseRejected
}

type libprocOutcome uint8

const (
	libprocOutcomeMiss libprocOutcome = iota
	libprocOutcomeApplied
	libprocOutcomeAmbiguous
	libprocOutcomeReuseReject
)

func nstatTupleFingerprint(flow *nstat.Flow) uint8 {
	if flow == nil {
		return 0
	}
	var bits uint8
	if flow.Local.Present {
		bits |= libprocBitLocalPresent
		if flow.Local.Address.IsValid() && !flow.Local.Address.IsUnspecified() {
			bits |= libprocBitLocalAddr
		}
		if flow.Local.Port != 0 {
			bits |= libprocBitLocalPort
		}
	}
	if flow.Remote.Present {
		bits |= libprocBitRemotePresent
		if flow.Remote.Address.IsValid() && !flow.Remote.Address.IsUnspecified() {
			bits |= libprocBitRemoteAddr
		}
		if flow.Remote.Port != 0 {
			bits |= libprocBitRemotePort
		}
	}
	return bits
}

func tupleMoreComplete(current, stored uint8) bool {
	return current&^stored != 0
}

func snapshotFDTruncated(snapshot libproc.Snapshot, pid uint32) bool {
	for _, truncatedPID := range snapshot.FDTruncatedPIDs {
		if truncatedPID == pid {
			return true
		}
	}
	return false
}

func sourcePredatesScan(source *nstatSource, scope libprocScanScope) bool {
	return !source.createdAt.IsZero() && source.createdAt.Before(scope.scanStart)
}

func clearLibprocTargetedState(source *nstatSource) {
	source.targetedBits = 0
	source.targetedPID = 0
	source.targetedTransient = 0
}

func clearLibprocTargetedOnPIDChange(source *nstatSource, oldPID, newPID uint32) {
	if oldPID != newPID {
		clearLibprocTargetedState(source)
	}
}

func resetLibprocCountersOnMoreComplete(source *nstatSource, previous uint8) {
	if source == nil || source.flow == nil {
		return
	}
	if !tupleMoreComplete(nstatTupleFingerprint(source.flow), previous) {
		return
	}
	source.targetedTransient = 0
	source.hostWalkTruncated = 0
}

func (t *nstatTracer) markLibprocAttempt(source *nstatSource, scope libprocScanScope, snapshot libproc.Snapshot, outcome libprocOutcome) {
	if !sourcePredatesScan(source, scope) {
		return
	}
	current := nstatTupleFingerprint(source.flow)
	if scope.hostWide && snapshot.HostWideTruncated {
		if source.flow != nil && source.flow.PID == 0 {
			source.hostWalkTruncated++
			if source.hostWalkTruncated >= libprocTransientCap {
				source.hostWalkStop = true
				source.hostWalkBits = current
			}
		}
		return
	}
	if scope.hostWide {
		if source.flow != nil && source.flow.PID == 0 {
			source.hostWalkStop = true
			source.hostWalkBits = current
			return
		}
		markLibprocTargeted(source, current, snapshot, outcome, false)
		return
	}
	markLibprocTargeted(source, current, snapshot, outcome, true)
}

func markLibprocTargeted(source *nstatSource, current uint8, snapshot libproc.Snapshot, outcome libprocOutcome, targetedScan bool) {
	if source.flow == nil {
		return
	}
	fdTruncated := snapshotFDTruncated(snapshot, source.flow.PID)
	stable := false
	transient := false
	switch outcome {
	case libprocOutcomeApplied:
		stable = true
	case libprocOutcomeMiss:
		if fdTruncated {
			transient = true
		} else {
			stable = true
		}
	case libprocOutcomeAmbiguous:
		transient = true
	case libprocOutcomeReuseReject:
		if targetedScan {
			stable = true
		} else {
			transient = true
		}
	}
	if stable {
		source.targetedBits = current
		source.targetedPID = source.flow.PID
		source.targetedTransient = 0
		return
	}
	if transient {
		// Bits are last-seen tuple state so a later more-complete update
		// can reset targetedTransient. Eligibility still keys off
		// targetedPID == 0 until the cap writes a fingerprint.
		source.targetedBits = current
		source.targetedTransient++
		if source.targetedTransient >= libprocTransientCap {
			source.targetedPID = source.flow.PID
		}
	}
}

func sourceNeedsLibprocReconciliation(source *nstatSource) bool {
	if source == nil || source.flow == nil {
		return false
	}
	flow := source.flow
	if flow.PID == 0 || !nstatEndpointComplete(flow.Local) {
		return true
	}
	if nstat.IsTCPProvider(source.provider) {
		return !nstatEndpointComplete(flow.Remote)
	}
	if nstat.IsUDPProvider(source.provider) {
		return flow.Remote.Present && !nstatEndpointComplete(flow.Remote)
	}
	return false
}

func nstatEndpointComplete(endpoint nstat.Endpoint) bool {
	return endpoint.Present &&
		endpoint.Address.IsValid() &&
		!endpoint.Address.IsUnspecified() &&
		endpoint.Port != 0
}

func (t *nstatTracer) applyLibprocEvidence(sourceRef uint64, source *nstatSource, observation libproc.Observation) {
	flow := *source.flow
	oldPID := flow.PID
	if flow.PID == 0 {
		flow.PID = observation.PID
	}
	flow.Local = fillNStatEndpoint(flow.Local, observation.Tuple.Source.Addr, observation.Tuple.SPort)
	flow.Remote = fillNStatEndpoint(flow.Remote, observation.Tuple.Dest.Addr, observation.Tuple.DPort)
	source.flow = &flow
	clearLibprocTargetedOnPIDChange(source, oldPID, source.flow.PID)
	if source.conn == nil && nstatSourceResolved(source) {
		source.conn = t.newConnection(sourceRef, source)
	}
	if source.conn != nil {
		t.fillConnectionTupleFromLibproc(source.conn, observation.Tuple)
		if source.conn.Pid == 0 {
			source.conn.Pid = observation.PID
		}
		t.applySource(source)
	}
}

func (t *nstatTracer) fillConnectionTupleFromLibproc(conn *network.ConnectionStats, observed network.ConnectionTuple) {
	changed := false
	if !conn.Source.Addr.IsValid() || conn.Source.Addr.IsUnspecified() {
		conn.Source = observed.Source
		changed = true
	}
	if conn.SPort == 0 {
		conn.SPort = observed.SPort
		changed = true
	}
	if !conn.Dest.Addr.IsValid() || conn.Dest.Addr.IsUnspecified() {
		conn.Dest = observed.Dest
		changed = true
	}
	if conn.DPort == 0 {
		conn.DPort = observed.DPort
		changed = true
	}
	if changed {
		if conn.Source.Addr.Is6() || conn.Dest.Addr.Is6() {
			conn.Family = network.AFINET6
		}
		t.tuples.add(conn)
	}
}

func fillNStatEndpoint(endpoint nstat.Endpoint, address netip.Addr, port uint16) nstat.Endpoint {
	if !endpoint.Present {
		endpoint.Present = true
	}
	if endpoint.Port == 0 {
		endpoint.Port = port
	}
	if !endpoint.Address.IsValid() || endpoint.Address.IsUnspecified() {
		endpoint.Address = address
	}
	return endpoint
}

type darwinLibprocMatchStatus uint8

const (
	darwinLibprocNoMatch darwinLibprocMatchStatus = iota
	darwinLibprocMatched
	darwinLibprocAmbiguous
)

type darwinLibprocPortKey struct {
	family network.ConnectionFamily
	typ    network.ConnectionType
	port   uint16
}

type darwinLibprocFamilyKey struct {
	family network.ConnectionFamily
	typ    network.ConnectionType
}

type darwinLibprocIndex struct {
	byLocalPort    map[darwinLibprocPortKey][]libproc.Observation
	byRemotePort   map[darwinLibprocPortKey][]libproc.Observation
	remotePortZero map[darwinLibprocFamilyKey][]libproc.Observation
	byFamilyType   map[darwinLibprocFamilyKey][]libproc.Observation
}

// indexDarwinLibprocObservations buckets a snapshot by family, type, and port
// so each source can be matched without scanning every observation.
func indexDarwinLibprocObservations(observations []libproc.Observation) darwinLibprocIndex {
	index := darwinLibprocIndex{
		byLocalPort:    make(map[darwinLibprocPortKey][]libproc.Observation, len(observations)),
		byRemotePort:   make(map[darwinLibprocPortKey][]libproc.Observation, len(observations)),
		remotePortZero: make(map[darwinLibprocFamilyKey][]libproc.Observation),
		byFamilyType:   make(map[darwinLibprocFamilyKey][]libproc.Observation),
	}
	for _, observation := range observations {
		familyKey := darwinLibprocFamilyKey{family: observation.Tuple.Family, typ: observation.Tuple.Type}
		index.byFamilyType[familyKey] = append(index.byFamilyType[familyKey], observation)
		if observation.Tuple.SPort != 0 {
			localKey := darwinLibprocPortKey{family: observation.Tuple.Family, typ: observation.Tuple.Type, port: observation.Tuple.SPort}
			index.byLocalPort[localKey] = append(index.byLocalPort[localKey], observation)
		}
		if observation.Tuple.DPort != 0 {
			remoteKey := darwinLibprocPortKey{family: observation.Tuple.Family, typ: observation.Tuple.Type, port: observation.Tuple.DPort}
			index.byRemotePort[remoteKey] = append(index.byRemotePort[remoteKey], observation)
			continue
		}
		index.remotePortZero[familyKey] = append(index.remotePortZero[familyKey], observation)
	}
	return index
}

// candidates returns the observations that can forward-match target, using the
// tightest port bucket that still covers the scorer's wildcard-remote cases.
func (index darwinLibprocIndex) candidates(target network.ConnectionTuple) []libproc.Observation {
	if target.SPort != 0 {
		return index.byLocalPort[darwinLibprocPortKey{family: target.Family, typ: target.Type, port: target.SPort}]
	}
	familyKey := darwinLibprocFamilyKey{family: target.Family, typ: target.Type}
	if target.DPort != 0 {
		matched := index.byRemotePort[darwinLibprocPortKey{family: target.Family, typ: target.Type, port: target.DPort}]
		if zero := index.remotePortZero[familyKey]; len(zero) > 0 {
			combined := make([]libproc.Observation, 0, len(matched)+len(zero))
			return append(append(combined, matched...), zero...)
		}
		return matched
	}
	return index.byFamilyType[familyKey]
}

func matchDarwinLibprocSource(source *nstatSource, observations []libproc.Observation) (libproc.Observation, darwinLibprocMatchStatus) {
	target := tupleFromNStatFlow(source)
	bestScore := -1
	candidates := make(map[darwinLibprocCandidateIdentity]libproc.Observation)
	for _, observation := range observations {
		if source.flow.PID != 0 && observation.PID != source.flow.PID {
			continue
		}
		forward := scoreDarwinLibprocTuple(observation.Tuple, target, false)
		reverse := scoreDarwinLibprocTuple(observation.Tuple, target, true)
		if forward < 0 || forward <= reverse {
			continue
		}
		if forward > bestScore {
			bestScore = forward
			clear(candidates)
		}
		if forward != bestScore {
			continue
		}
		identity := darwinLibprocCandidateIdentity{
			process: darwinProcessIdentity{
				pid:       observation.PID,
				startTime: observation.ProcessStartTime,
			},
			tuple: canonicalDarwinLibprocTuple(observation.Tuple),
		}
		candidates[identity] = observation
	}
	if bestScore < 0 || len(candidates) == 0 {
		return libproc.Observation{}, darwinLibprocNoMatch
	}
	if len(candidates) != 1 {
		return libproc.Observation{}, darwinLibprocAmbiguous
	}
	for _, candidate := range candidates {
		return candidate, darwinLibprocMatched
	}
	return libproc.Observation{}, darwinLibprocNoMatch
}

func canonicalDarwinLibprocTuple(tuple network.ConnectionTuple) network.ConnectionTuple {
	tuple.Source.Addr = normalizeDarwinAddress(tuple.Source.Addr)
	tuple.Dest.Addr = normalizeDarwinAddress(tuple.Dest.Addr)
	tuple.Pid = 0
	tuple.NetNS = 0
	tuple.Direction = network.UNKNOWN
	return tuple
}

func tupleFromNStatFlow(source *nstatSource) network.ConnectionTuple {
	var tuple network.ConnectionTuple
	if nstat.IsTCPProvider(source.provider) {
		tuple.Type = network.TCP
	} else if nstat.IsUDPProvider(source.provider) {
		tuple.Type = network.UDP
	}
	if source.flow.Local.Present {
		tuple.Source.Addr = source.flow.Local.Address
		tuple.SPort = source.flow.Local.Port
	}
	if source.flow.Remote.Present {
		tuple.Dest.Addr = source.flow.Remote.Address
		tuple.DPort = source.flow.Remote.Port
	}
	address := tuple.Source.Addr
	if !address.IsValid() {
		address = tuple.Dest.Addr
	}
	if address.Is6() {
		tuple.Family = network.AFINET6
	} else {
		tuple.Family = network.AFINET
	}
	return tuple
}

func scoreDarwinLibprocTuple(socket, target network.ConnectionTuple, reverse bool) int {
	if socket.Family != target.Family || socket.Type != target.Type {
		return -1
	}
	if reverse {
		target = reverseDarwinTuple(target)
	}
	localScore := scoreDarwinLibprocEndpoint(
		socket.Source.Addr,
		socket.SPort,
		target.Source.Addr,
		target.SPort,
		false,
	)
	remoteScore := scoreDarwinLibprocEndpoint(
		socket.Dest.Addr,
		socket.DPort,
		target.Dest.Addr,
		target.DPort,
		true,
	)
	if localScore < 0 || remoteScore < 0 {
		return -1
	}
	return localScore + remoteScore
}

func scoreDarwinLibprocEndpoint(socketAddress netip.Addr, socketPort uint16, targetAddress netip.Addr, targetPort uint16, allowAnyPort bool) int {
	if targetPort != 0 && socketPort != targetPort && (!allowAnyPort || socketPort != 0) {
		return -1
	}
	if targetAddress.IsValid() && !targetAddress.IsUnspecified() &&
		socketAddress.IsValid() && !socketAddress.IsUnspecified() &&
		normalizeDarwinAddress(socketAddress) != normalizeDarwinAddress(targetAddress) {
		return -1
	}
	score := 0
	if socketPort != 0 && targetPort != 0 {
		score += 2
	}
	if socketAddress.IsValid() && !socketAddress.IsUnspecified() &&
		targetAddress.IsValid() && !targetAddress.IsUnspecified() {
		score++
	}
	return score
}
