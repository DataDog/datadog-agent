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
)

var darwinLibprocTelemetry = struct {
	scans       telemetry.Counter
	errors      telemetry.Counter
	truncated   telemetry.Counter
	resolved    telemetry.Counter
	ambiguous   telemetry.Counter
	reuseReject telemetry.Counter
}{
	scans:       telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "scans", nil, "Bounded libproc reconciliation scans"),
	errors:      telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "errors", nil, "Failed libproc reconciliation scans"),
	truncated:   telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "truncated", nil, "Libproc reconciliation scans stopped at a configured bound"),
	resolved:    telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "resolved", nil, "NStat sources resolved through direct libproc evidence"),
	ambiguous:   telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "ambiguous", nil, "Libproc ownership candidates rejected as ambiguous"),
	reuseReject: telemetryimpl.GetCompatComponent().NewCounter("network_tracer__darwin_libproc", "pid_reuse_rejected", nil, "Libproc candidates rejected by process start time"),
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

func (r *darwinLibprocReconciler) runOnce() error {
	darwinLibprocTelemetry.scans.Inc()
	snapshot, err := r.scanner.Scan()
	if err != nil {
		darwinLibprocTelemetry.errors.Inc()
		return err
	}
	if snapshot.Truncated {
		darwinLibprocTelemetry.truncated.Inc()
	}
	resolved, ambiguous, reuseRejected := r.primary.reconcileLibprocSnapshot(snapshot)
	darwinLibprocTelemetry.resolved.Add(float64(resolved))
	darwinLibprocTelemetry.ambiguous.Add(float64(ambiguous))
	darwinLibprocTelemetry.reuseReject.Add(float64(reuseRejected))
	return nil
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

func (t *nstatTracer) reconcileLibprocSnapshot(snapshot libproc.Snapshot) (resolved, ambiguous, reuseRejected int) {
	index := indexDarwinLibprocObservations(snapshot.Observations)
	var closed []*network.ConnectionStats
	t.mu.Lock()
	for sourceRef, source := range t.sources {
		if !sourceNeedsLibprocReconciliation(source) {
			continue
		}
		candidate, status := matchDarwinLibprocSource(source, index.candidates(tupleFromNStatFlow(source)))
		switch status {
		case darwinLibprocAmbiguous:
			ambiguous++
			continue
		case darwinLibprocNoMatch:
			continue
		}
		if candidate.ProcessStartTime != 0 && !source.createdAt.IsZero() &&
			candidate.ProcessStartTime > uint64(source.createdAt.Add(darwinLibprocStartTimeLeeway).UnixNano()) {
			reuseRejected++
			continue
		}

		t.applyLibprocEvidence(sourceRef, source, candidate)
		resolved++
		if source.removed && source.conn != nil {
			closed = append(closed, t.closeAndRemoveSource(sourceRef, source))
		}
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
	if flow.PID == 0 {
		flow.PID = observation.PID
	}
	flow.Local = fillNStatEndpoint(flow.Local, observation.Tuple.Source.Addr, observation.Tuple.SPort)
	flow.Remote = fillNStatEndpoint(flow.Remote, observation.Tuple.Dest.Addr, observation.Tuple.DPort)
	source.flow = &flow
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
