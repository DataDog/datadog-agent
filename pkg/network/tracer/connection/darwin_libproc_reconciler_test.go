// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/libproc"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
	processutil "github.com/DataDog/datadog-agent/pkg/process/util"
)

func testHostScope(tracer *nstatTracer) libprocScanScope {
	return libprocScanScope{scanStart: tracer.now().Add(time.Second), hostWide: true}
}

func (tracer *nstatTracer) reconcileHostSnapshot(snapshot libproc.Snapshot) (resolved, ambiguous, reuseRejected int) {
	return tracer.reconcileLibprocSnapshot(snapshot, testHostScope(tracer))
}

func TestDarwinLibprocReconcilerScansImmediately(t *testing.T) {
	scanner := &recordingLibprocScanner{}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	reconciler := newDarwinLibprocReconciler(scanner, tracer, time.Hour)

	require.NoError(t, reconciler.runOnce())
	require.Zero(t, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})
	require.NoError(t, reconciler.runOnce())
	require.Equal(t, 1, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocReconcilerFillsOnlyUnresolvedPID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(0, tcpStateEstablished)
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(1234, 1)},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(1234), buffer.Connections()[0].Pid)
	require.Equal(t, flow.Local.Address, buffer.Connections()[0].Source.Addr)
	require.Equal(t, flow.Remote.Address, buffer.Connections()[0].Dest.Addr)
}

func TestDarwinLibprocReconcilerRejectsAmbiguousOwnership(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})
	first := testDarwinLibprocObservation(1234, 1)
	second := testDarwinLibprocObservation(5678, 2)

	resolved, ambiguous, _ := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{first, second},
	})

	require.Zero(t, resolved)
	require.Equal(t, 1, ambiguous)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Zero(t, buffer.Connections()[0].Pid)
}

func TestDarwinLibprocReconcilerRejectsCandidateWithWrongPID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(1234, tcpStateEstablished)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(5678, 1)},
	})

	require.Zero(t, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	require.Equal(t, uint32(1234), tracer.sources[1].flow.PID)
	require.False(t, nstatEndpointComplete(tracer.sources[1].flow.Remote))
}

func TestDarwinLibprocReconcilerSelectsCandidateWithAuthoritativePID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(1234, tcpStateEstablished)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})
	correct := testDarwinLibprocObservation(1234, 1)
	wrong := testDarwinLibprocObservation(5678, 1)
	wrong.Tuple.Dest.Addr = netip.MustParseAddr("198.51.100.21")
	wrong.Tuple.DPort = 8443

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{wrong, correct},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(1234), buffer.Connections()[0].Pid)
	require.Equal(t, correct.Tuple.Dest.Addr, buffer.Connections()[0].Dest.Addr)
	require.Equal(t, correct.Tuple.DPort, buffer.Connections()[0].DPort)
}

func TestDarwinLibprocReconcilerRejectsAmbiguousSocketsFromSameProcess(t *testing.T) {
	first := testDarwinLibprocObservation(1234, 1)
	second := first
	second.Tuple.Dest.Addr = netip.MustParseAddr("198.51.100.21")
	second.Tuple.DPort = 8443

	for name, observations := range map[string][]libproc.Observation{
		"forward scan order": {first, second},
		"reverse scan order": {second, first},
	} {
		t.Run(name, func(t *testing.T) {
			tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
			flow := testNStatTCPFlow(0, tcpStateEstablished)
			flow.Remote = nstat.Endpoint{}
			tracer.processEvent(nstat.Event{
				Kind:      nstat.EventDescription,
				SourceRef: 1,
				Provider:  nstat.ProviderTCPKernel,
				Flow:      flow,
			})

			resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
				Observations: observations,
			})

			require.Zero(t, resolved)
			require.Equal(t, 1, ambiguous)
			require.Zero(t, reuseRejected)
			require.False(t, nstatEndpointComplete(tracer.sources[1].flow.Remote))
		})
	}
}

func TestDarwinLibprocReconcilerDeduplicatesIdenticalSocketObservations(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(0, tcpStateEstablished)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})
	observation := testDarwinLibprocObservation(1234, 1)

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation, observation},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, observation.Tuple.Dest.Addr, buffer.Connections()[0].Dest.Addr)
	require.Equal(t, observation.Tuple.DPort, buffer.Connections()[0].DPort)
}

func TestDarwinLibprocEvidenceCannotOverwriteNStatTuple(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(0, tcpStateEstablished)
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})
	conflicting := testDarwinLibprocObservation(1234, 1)
	conflicting.Tuple.Source.Addr = netip.MustParseAddr("203.0.113.10")
	conflicting.Tuple.Dest.Addr = netip.MustParseAddr("203.0.113.20")
	conflicting.Tuple.SPort = 1234
	conflicting.Tuple.DPort = 4321

	tracer.mu.Lock()
	tracer.applyLibprocEvidence(1, tracer.sources[1], conflicting)
	tracer.mu.Unlock()

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	conn := buffer.Connections()[0]
	require.Equal(t, uint32(1234), conn.Pid)
	require.Equal(t, flow.Local.Address, conn.Source.Addr)
	require.Equal(t, flow.Local.Port, conn.SPort)
	require.Equal(t, flow.Remote.Address, conn.Dest.Addr)
	require.Equal(t, flow.Remote.Port, conn.DPort)
	require.Equal(t, uint32(tcpStateEstablished), tracer.sources[1].flow.TCPState)
}

func TestDarwinLibprocReconcilerCompletesUDPConnectionTuple(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatUDPFlow(0)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      flow,
	})
	observation := testDarwinLibprocObservation(1234, 1)
	observation.Tuple.Type = network.UDP

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	conn := buffer.Connections()[0]
	require.Equal(t, observation.Tuple.Dest.Addr, conn.Dest.Addr)
	require.Equal(t, observation.Tuple.DPort, conn.DPort)
	require.Equal(t, uint32(1234), conn.Pid)
	match := tracer.tuples.match(conn.ConnectionTuple)
	require.True(t, match.matched)
	require.Equal(t, conn.Cookie, match.cookie)
}

func TestDarwinLibprocReconcilerIgnoresUnconnectedUDPWithNStatPID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatUDPFlow(1234)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      flow,
	})
	observation := testDarwinLibprocObservation(9876, 1)
	observation.Tuple.Type = network.UDP

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation},
	})

	require.Zero(t, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	conn := buffer.Connections()[0]
	require.Equal(t, uint32(1234), conn.Pid)
	require.False(t, conn.Dest.Addr.IsValid())
	require.Zero(t, conn.DPort)
}

func TestDarwinLibprocReconcilerRejectsPIDReuse(t *testing.T) {
	now := time.Unix(100, 0)
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.now = func() time.Time { return now }
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})
	observation := testDarwinLibprocObservation(1234, uint64(now.Add(2*time.Second).UnixNano()))

	resolved, _, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation},
	})

	require.Zero(t, resolved)
	require.Equal(t, 1, reuseRejected)
}

func TestDarwinLibprocReconcilerCompletesRemovedPartialSourceOnce(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	partial := testNStatTCPFlow(0, tcpStateEstablished)
	partial.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      partial,
	})
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 1})
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(1234, 1)},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	require.Len(t, closed, 1)
	require.True(t, closed[0].IsClosed)
	require.Equal(t, uint32(1234), closed[0].Pid)

	tracer.reconcileHostSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(1234, 1)},
	})
	require.Len(t, closed, 1)
}

// TestDarwinLibprocIndexSelectsLocalPortBucket matches one local-port bucket
// among many unrelated observations instead of scanning the full snapshot.
func TestDarwinLibprocIndexSelectsLocalPortBucket(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})

	observations := make([]libproc.Observation, 0, 201)
	for port := uint16(1); port <= 200; port++ {
		observation := testDarwinLibprocObservation(1000+uint32(port), 1)
		observation.Tuple.SPort = port
		observations = append(observations, observation)
	}
	observations = append(observations, testDarwinLibprocObservation(1234, 1))

	index := indexDarwinLibprocObservations(observations)
	require.Len(t, index.candidates(tupleFromNStatFlow(tracer.sources[1])), 1)

	resolved, ambiguous, reuseRejected := tracer.reconcileHostSnapshot(libproc.Snapshot{Observations: observations})
	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Equal(t, uint32(1234), buffer.Connections()[0].Pid)
}

func testDarwinLibprocObservation(pid uint32, start uint64) libproc.Observation {
	return libproc.Observation{
		Tuple: network.ConnectionTuple{
			Source: processutil.Address{Addr: netip.MustParseAddr("192.0.2.10")},
			Dest:   processutil.Address{Addr: netip.MustParseAddr("198.51.100.20")},
			SPort:  50000,
			DPort:  443,
			Type:   network.TCP,
			Family: network.AFINET,
		},
		PID:              pid,
		ProcessStartTime: start,
	}
}

type recordingLibprocScanner struct {
	hostWalks     int
	scanPIDs      []uint32
	hostSnapshot  libproc.Snapshot
	pidSnapshots  map[uint32]libproc.Snapshot
	err           error
	pidErrs       map[uint32]error
}

func (s *recordingLibprocScanner) Scan() (libproc.Snapshot, error) {
	s.hostWalks++
	if s.err != nil {
		return libproc.Snapshot{}, s.err
	}
	return s.hostSnapshot, nil
}

func (s *recordingLibprocScanner) ScanPID(pid uint32) (libproc.Snapshot, error) {
	s.scanPIDs = append(s.scanPIDs, pid)
	if err := s.pidErrs[pid]; err != nil {
		return libproc.Snapshot{}, err
	}
	if s.pidSnapshots != nil {
		return s.pidSnapshots[pid], nil
	}
	return libproc.Snapshot{}, nil
}

func addNStatSource(tracer *nstatTracer, ref uint64, flow *nstat.Flow) {
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: ref,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})
}

func advanceTracerNow(tracer *nstatTracer) {
	base := tracer.now()
	tracer.now = func() time.Time { return base.Add(time.Second) }
}

func resetRecording(scanner *recordingLibprocScanner) {
	scanner.hostWalks = 0
	scanner.scanPIDs = nil
}

func newGatedReconciler(scanner *recordingLibprocScanner, tracer *nstatTracer) *darwinLibprocReconciler {
	return newDarwinLibprocReconciler(scanner, tracer, time.Hour)
}

func TestDarwinLibprocSkipWhenNothingNeedsIt(t *testing.T) {
	scanner := &recordingLibprocScanner{}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(1234, tcpStateEstablished))
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocStoppedPID0SkipsWhileNeedStaysTrue(t *testing.T) {
	scanner := &recordingLibprocScanner{}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, 1, scanner.hostWalks)
	require.True(t, tracer.sources[1].hostWalkStop)
	require.True(t, sourceNeedsLibprocReconciliation(tracer.sources[1]))

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocFirstWorkTickIsHostWalkThenKnownPIDIsScanPID(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, 1, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
	require.Zero(t, tracer.sources[1].targetedPID)

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
	require.Equal(t, []uint32{1234}, scanner.scanPIDs)
}

func TestDarwinLibprocMixedPID0AndKnownPIDOneHostWalk(t *testing.T) {
	observation := testDarwinLibprocObservation(1234, 1)
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{Observations: []libproc.Observation{observation}}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	pid0 := testNStatTCPFlow(0, tcpStateEstablished)
	pid0.Local.Port = 51000
	addNStatSource(tracer, 1, pid0)
	addNStatSource(tracer, 2, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, 1, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
	require.True(t, tracer.sources[1].hostWalkStop)
	require.Equal(t, uint32(1234), tracer.sources[2].targetedPID)
}

func TestDarwinLibprocHostWideTruncatedWritesNoTargetedFingerprint(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	addNStatSource(tracer, 2, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, tracer.sources[2].targetedPID)
	require.False(t, tracer.sources[1].hostWalkStop)

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, []uint32{1234}, scanner.scanPIDs)
}

func TestDarwinLibprocTwoNewPID0SecondTickDoesNotHostWalk(t *testing.T) {
	scanner := &recordingLibprocScanner{}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, 1, scanner.hostWalks)

	addNStatSource(tracer, 2, testNStatTCPFlow(0, tcpStateEstablished))
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
}

func TestDarwinLibprocNoSecondCompleteHostWalkIfFingerprintUnchanged(t *testing.T) {
	scanner := &recordingLibprocScanner{}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	bits := tracer.sources[1].hostWalkBits
	require.NotZero(t, bits)

	for range 5 {
		resetRecording(scanner)
		require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
		require.Zero(t, scanner.hostWalks)
	}
	require.Equal(t, bits, tracer.sources[1].hostWalkBits)
}

func TestDarwinLibprocRetryWhenTupleGainsABit(t *testing.T) {
	scanner := &recordingLibprocScanner{}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(0, tcpStateEstablished)
	flow.Remote.Port = 0
	addNStatSource(tracer, 1, flow)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.True(t, tracer.sources[1].hostWalkStop)

	updated := *flow
	updated.Remote.Port = 443
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      &updated,
	})
	// lastHostWalkTick is 1; ticks 2 and 3 are inside the interval.
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, 1, scanner.hostWalks)
}

func TestDarwinLibprocKnownPIDTargetedThenSkip(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())

	scanner.pidSnapshots = map[uint32]libproc.Snapshot{
		1234: {Observations: []libproc.Observation{testDarwinLibprocObservation(1234, 1)}},
	}
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, []uint32{1234}, scanner.scanPIDs)
	require.Equal(t, uint32(1234), tracer.sources[1].targetedPID)

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocHostWalkedPID0ThenGivenPIDGetsScanPID(t *testing.T) {
	scanner := &recordingLibprocScanner{}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	pid0 := testNStatTCPFlow(0, tcpStateEstablished)
	pid0.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, pid0)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())

	updated := testNStatTCPFlow(99, tcpStateEstablished)
	updated.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      updated,
	})
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, []uint32{99}, scanner.scanPIDs)
}

func TestDarwinLibprocPIDChangeInvalidatesTargetedFingerprint(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(10, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	scanner.pidSnapshots = map[uint32]libproc.Snapshot{10: {}}
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, uint32(10), tracer.sources[1].targetedPID)

	changed := testNStatTCPFlow(20, tcpStateEstablished)
	changed.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      changed,
	})
	require.Zero(t, tracer.sources[1].targetedPID)
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, []uint32{20}, scanner.scanPIDs)
}

func TestDarwinLibprocScanPIDAmbiguousAndFDTruncatedMissWaitForCap(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())

	first := testDarwinLibprocObservation(1234, 1)
	second := first
	second.Tuple.Dest.Addr = netip.MustParseAddr("198.51.100.21")
	scanner.pidSnapshots = map[uint32]libproc.Snapshot{
		1234: {Observations: []libproc.Observation{first, second}},
	}
	for i := 1; i <= 2; i++ {
		resetRecording(scanner)
		require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
		require.Equal(t, []uint32{1234}, scanner.scanPIDs)
		require.Zero(t, tracer.sources[1].targetedPID)
		require.Equal(t, uint8(i), tracer.sources[1].targetedTransient)
	}
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, uint32(1234), tracer.sources[1].targetedPID)

	incomplete2 := testNStatTCPFlow(5678, tcpStateEstablished)
	incomplete2.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 2, incomplete2)
	advanceTracerNow(tracer)
	scanner.pidSnapshots[5678] = libproc.Snapshot{FDTruncatedPIDs: []uint32{5678}}
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Contains(t, scanner.scanPIDs, uint32(5678))
	require.Zero(t, tracer.sources[2].targetedPID)
	require.Equal(t, uint8(1), tracer.sources[2].targetedTransient)
}

func TestDarwinLibprocTargetedReuseRejectIsStable(t *testing.T) {
	now := time.Unix(100, 0)
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.now = func() time.Time { return now }
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	now = now.Add(time.Second)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())

	obs := testDarwinLibprocObservation(1234, uint64(now.Add(2*time.Second).UnixNano()))
	scanner.pidSnapshots = map[uint32]libproc.Snapshot{1234: {Observations: []libproc.Observation{obs}}}
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, uint32(1234), tracer.sources[1].targetedPID)

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocHostWalkReuseRejectDoesNotRecordTargeted(t *testing.T) {
	now := time.Unix(100, 0)
	obs := testDarwinLibprocObservation(1234, uint64(now.Add(3*time.Second).UnixNano()))
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{Observations: []libproc.Observation{obs}}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.now = func() time.Time { return now }
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	now = now.Add(time.Second)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, tracer.sources[1].targetedPID)
	require.Equal(t, uint8(1), tracer.sources[1].targetedTransient)
}

func TestDarwinLibprocPID0CompleteWalkStopsOnAmbiguous(t *testing.T) {
	first := testDarwinLibprocObservation(1234, 1)
	second := testDarwinLibprocObservation(5678, 2)
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{Observations: []libproc.Observation{first, second}}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, tracer.sources[1].flow.PID)
	require.True(t, tracer.sources[1].hostWalkStop)
	require.NotZero(t, tracer.sources[1].hostWalkBits)

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocPID0CompleteWalkStopsOnReuseReject(t *testing.T) {
	now := time.Unix(100, 0)
	obs := testDarwinLibprocObservation(1234, uint64(now.Add(3*time.Second).UnixNano()))
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{Observations: []libproc.Observation{obs}}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.now = func() time.Time { return now }
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	now = now.Add(time.Second)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, tracer.sources[1].flow.PID)
	require.True(t, tracer.sources[1].hostWalkStop)
	require.NotZero(t, tracer.sources[1].hostWalkBits)

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocTCPListenerOneScanThenSkip(t *testing.T) {
	obs := testDarwinLibprocObservation(1234, 1)
	obs.Tuple.Dest.Addr = netip.IPv4Unspecified()
	obs.Tuple.DPort = 0
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{Observations: []libproc.Observation{obs}}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPListenerFlow(0, "192.0.2.10", 50000))
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.True(t, sourceNeedsLibprocReconciliation(tracer.sources[1]))

	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, scanner.hostWalks)
	require.Empty(t, scanner.scanPIDs)
}

func TestDarwinLibprocDedupScanPIDByPID(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	first := testNStatTCPFlow(42, tcpStateEstablished)
	first.Remote = nstat.Endpoint{}
	second := testNStatTCPFlow(42, tcpStateEstablished)
	second.Local.Port = 50001
	second.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, first)
	addNStatSource(tracer, 2, second)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, []uint32{42}, scanner.scanPIDs)
}

func TestDarwinLibprocTruncatedHostWalkStopsAfterThreeAndStoresBits(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	advanceTracerNow(tracer)
	reconciler := newGatedReconciler(scanner, tracer)

	require.NoError(t, reconciler.runOnce())
	require.Equal(t, 1, scanner.hostWalks)
	require.False(t, tracer.sources[1].hostWalkStop)

	resetRecording(scanner)
	require.NoError(t, reconciler.runOnce())
	require.NoError(t, reconciler.runOnce())
	require.Zero(t, scanner.hostWalks)

	require.NoError(t, reconciler.runOnce())
	require.Equal(t, 1, scanner.hostWalks)
	require.False(t, tracer.sources[1].hostWalkStop)

	resetRecording(scanner)
	require.NoError(t, reconciler.runOnce())
	require.NoError(t, reconciler.runOnce())
	require.Zero(t, scanner.hostWalks)

	require.NoError(t, reconciler.runOnce())
	require.Equal(t, 1, scanner.hostWalks)
	require.True(t, tracer.sources[1].hostWalkStop)
	require.NotZero(t, tracer.sources[1].hostWalkBits)

	resetRecording(scanner)
	require.NoError(t, reconciler.runOnce())
	require.Zero(t, scanner.hostWalks)
}

func TestDarwinLibprocScanPIDDoesNotSetOtherSourceHostWalkStop(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(7, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	addNStatSource(tracer, 2, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, []uint32{7}, scanner.scanPIDs)
	require.False(t, tracer.sources[1].hostWalkStop)
}

func TestDarwinLibprocScanPIDDoesNotApplyToOtherKnownPID(t *testing.T) {
	obs := testDarwinLibprocObservation(7, 1)
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	a := testNStatTCPFlow(5, tcpStateEstablished)
	a.Remote = nstat.Endpoint{}
	b := testNStatTCPFlow(7, tcpStateEstablished)
	b.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, a)
	addNStatSource(tracer, 2, b)
	pid0 := testNStatTCPFlow(0, tcpStateEstablished)
	pid0.Local.Port = 50000
	addNStatSource(tracer, 3, pid0)
	advanceTracerNow(tracer)
	resolved, _, _ := tracer.reconcileLibprocSnapshot(libproc.Snapshot{Observations: []libproc.Observation{obs}}, libprocScanScope{
		scanStart: tracer.now().Add(time.Second),
		pid:       7,
	})
	require.NotZero(t, resolved)
	require.Equal(t, uint32(5), tracer.sources[1].flow.PID)
	require.False(t, nstatEndpointComplete(tracer.sources[1].flow.Remote))
	require.Equal(t, uint32(7), tracer.sources[2].flow.PID)
	require.True(t, nstatEndpointComplete(tracer.sources[2].flow.Remote))
	require.Equal(t, uint32(7), tracer.sources[3].flow.PID)
}

func TestDarwinLibprocPID0StopsDespiteForeignFDTruncation(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{FDTruncatedPIDs: []uint32{99}}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.True(t, tracer.sources[1].hostWalkStop)
}

func TestDarwinLibprocKnownPIDFDTruncatedMissWaitsForCap(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	scanner.pidSnapshots = map[uint32]libproc.Snapshot{1234: {FDTruncatedPIDs: []uint32{1234}}}
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, tracer.sources[1].targetedPID)
}

func TestDarwinLibprocDeadPIDEmptySnapshotIsSuccess(t *testing.T) {
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{HostWideTruncated: true}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	scanner.pidSnapshots = map[uint32]libproc.Snapshot{1234: {}}
	resetRecording(scanner)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, uint32(1234), tracer.sources[1].targetedPID)
}

func TestDarwinLibprocScanPIDFailureContinues(t *testing.T) {
	scanner := &recordingLibprocScanner{
		hostSnapshot: libproc.Snapshot{HostWideTruncated: true},
		pidErrs:      map[uint32]error{1: errors.New("alloc")},
	}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	first := testNStatTCPFlow(1, tcpStateEstablished)
	first.Remote = nstat.Endpoint{}
	second := testNStatTCPFlow(2, tcpStateEstablished)
	second.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, first)
	addNStatSource(tracer, 2, second)
	advanceTracerNow(tracer)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	resetRecording(scanner)
	err := newGatedReconciler(scanner, tracer).runOnce()
	require.Error(t, err)
	require.ElementsMatch(t, []uint32{1, 2}, scanner.scanPIDs)
	require.Zero(t, tracer.sources[1].targetedPID)
}

func TestDarwinLibprocFailedHostWalkDoesNotAdvanceLastHostWalkTick(t *testing.T) {
	scanner := &recordingLibprocScanner{err: errors.New("alloc")}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	advanceTracerNow(tracer)
	require.Error(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Zero(t, tracer.lastHostWalkTick)
	require.Equal(t, 1, scanner.hostWalks)
	resetRecording(scanner)
	require.Error(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, 1, scanner.hostWalks)
}

func TestDarwinLibprocMidWalkSourceIsNotMarked(t *testing.T) {
	now := time.Unix(100, 0)
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.now = func() time.Time { return now }
	addNStatSource(tracer, 1, testNStatTCPFlow(0, tcpStateEstablished))
	scanStart := now.Add(time.Millisecond)
	now = now.Add(time.Second)
	late := testNStatTCPFlow(0, tcpStateEstablished)
	late.Local.Port = 50001
	addNStatSource(tracer, 2, late)
	tracer.reconcileLibprocSnapshot(libproc.Snapshot{}, libprocScanScope{scanStart: scanStart, hostWide: true})
	require.True(t, tracer.sources[1].hostWalkStop)
	require.False(t, tracer.sources[2].hostWalkStop)
}

func TestDarwinLibprocTransientResetsWhenTupleGainsABit(t *testing.T) {
	now := time.Unix(100, 0)
	obs := testDarwinLibprocObservation(1234, uint64(now.Add(3*time.Second).UnixNano()))
	scanner := &recordingLibprocScanner{hostSnapshot: libproc.Snapshot{Observations: []libproc.Observation{obs}}}
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.now = func() time.Time { return now }
	incomplete := testNStatTCPFlow(1234, tcpStateEstablished)
	incomplete.Remote = nstat.Endpoint{}
	addNStatSource(tracer, 1, incomplete)
	now = now.Add(time.Second)
	require.NoError(t, newGatedReconciler(scanner, tracer).runOnce())
	require.Equal(t, uint8(1), tracer.sources[1].targetedTransient)

	filled := testNStatTCPFlow(1234, tcpStateEstablished)
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      filled,
	})
	require.Zero(t, tracer.sources[1].targetedTransient)
}
