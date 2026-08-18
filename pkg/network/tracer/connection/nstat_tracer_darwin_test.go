// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"io"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/libproc"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

func TestNStatTracerTCPActiveAndFinalLifecycle(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(100, 0)
	tracer.now = func() time.Time { return now }
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventAdded,
		SourceRef: 7,
		Provider:  nstat.ProviderTCPKernel,
	})
	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{7}, control.descriptions())

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventUpdate,
		SourceRef: 7,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1234, 4),
		Counts: &nstat.Counts{
			RXPackets:            11,
			RXBytes:              1200,
			TXPackets:            12,
			TXBytes:              1417,
			TXRetransmittedBytes: 17,
			AverageRTT:           905,
			RTTVariance:          32,
			ConnectAttempts:      1,
			ConnectSuccesses:     1,
		},
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	active := buffer.Connections()
	require.Len(t, active, 1)
	require.Equal(t, uint32(1234), active[0].Pid)
	require.Equal(t, uint64(1400), active[0].Monotonic.SentBytes)
	require.Equal(t, uint64(1200), active[0].Monotonic.RecvBytes)
	require.Equal(t, uint16(1), active[0].Monotonic.TCPEstablished)
	require.Equal(t, uint64(12), active[0].Monotonic.SentPackets)
	require.Equal(t, uint64(11), active[0].Monotonic.RecvPackets)
	require.Equal(t, uint32(28281), active[0].RTT)
	require.Equal(t, uint32(2000), active[0].RTTVar)
	require.Equal(t, uint32(14), active[0].InterfaceIndex)
	require.False(t, active[0].IsClosed)

	now = now.Add(3 * time.Second)
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 7})
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 7})

	require.Len(t, closed, 1)
	require.True(t, closed[0].IsClosed)
	require.Equal(t, uint16(1), closed[0].Monotonic.TCPClosed)
	require.Equal(t, 3*time.Second, closed[0].Duration)
	buffer.Reset()
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Empty(t, buffer.Connections())
}

func TestNStatTracerLatchesTimeoutAndDoesNotReplayClose(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(200, 0)
	tracer.now = func() time.Time { return now }
	var closed *network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) { closed = conn }

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventUpdate,
		SourceRef: 8,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(2345, 2),
		Counts: &nstat.Counts{
			ConnectAttempts: 1,
		},
	})
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventUpdate,
		SourceRef: 8,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(2345, tcpStateClosed),
		Counts:    &nstat.Counts{},
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	failed := buffer.Connections()[0]
	require.Equal(t, map[uint16]uint32{network.TCPFailureErrnoTimedOut: 1}, failed.TCPFailures)
	require.Equal(t, uint16(1), failed.Monotonic.TCPClosed)
	require.True(t, failed.IsClosed)

	now = now.Add(time.Second)
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 8})

	require.NotNil(t, closed)
	require.Equal(t, uint16(1), closed.Monotonic.TCPClosed)
	require.Equal(t, map[uint16]uint32{network.TCPFailureErrnoTimedOut: 1}, closed.TCPFailures)
}

func TestNStatTracerDeliversLateDescriptionAfterRemoval(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	tracer.now = func() time.Time { return time.Unix(300, 0) }
	var closed *network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) { closed = conn }

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventAdded,
		SourceRef: 9,
		Provider:  nstat.ProviderUDPKernel,
	})
	tracer.pumpDescriptionRequests(tracer.now())
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 9})
	tracer.pumpDescriptionRequests(tracer.now())
	require.Equal(t, []uint64{9, 9}, control.descriptions())

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 9,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      testNStatUDPFlow(3456),
	})

	require.NotNil(t, closed)
	require.Equal(t, network.UDP, closed.Type)
	require.Equal(t, uint32(3456), closed.Pid)
	require.True(t, closed.IsClosed)
	require.Zero(t, closed.Monotonic.TCPClosed)
}

func TestNStatTracerSeparatesReusedSourceReference(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	tracer.now = func() time.Time { return time.Unix(350, 0) }
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 10,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1111, tcpStateEstablished),
	})
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	firstCookie := buffer.Connections()[0].Cookie

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 10,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(2222, tcpStateEstablished),
	})

	require.Len(t, closed, 1)
	require.Equal(t, uint32(1111), closed[0].Pid)
	buffer.Reset()
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(2222), buffer.Connections()[0].Pid)
	require.NotEqual(t, firstCookie, buffer.Connections()[0].Cookie)
}

func TestNStatTracerWaitsForCompleteTCPTuple(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(360, 0)
	tracer.now = func() time.Time { return now }
	partial := testNStatTCPFlow(3333, tcpStateEstablished)
	partial.Local.Address = netip.IPv4Unspecified()
	partial.Remote.Address = netip.IPv4Unspecified()
	partial.Remote.Port = 0
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 14,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      partial,
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Empty(t, buffer.Connections())
	tracer.queueUnresolvedDescriptions(now)
	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{14}, control.descriptions())

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 14,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(3333, tcpStateEstablished),
	})
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, netip.MustParseAddr("192.0.2.10"), buffer.Connections()[0].Source.Addr)
	require.Equal(t, netip.MustParseAddr("198.51.100.20"), buffer.Connections()[0].Dest.Addr)
}

func TestNStatTracerAppliesLateAuthoritativePID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 15,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 15,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(4321, tcpStateEstablished),
	})

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(9876, 1)},
	})

	require.Zero(t, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(4321), buffer.Connections()[0].Pid)
}

func TestNStatTracerAppliesLateAuthoritativeUDPRemote(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	initial := testNStatUDPFlow(1234)
	initial.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 16,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      initial,
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	oldTuple := buffer.Connections()[0].ConnectionTuple
	require.False(t, oldTuple.Dest.Addr.IsValid())

	update := testNStatUDPFlow(1234)
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 16,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      update,
	})

	buffer.Reset()
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	conn := buffer.Connections()[0]
	require.Equal(t, update.Remote.Address, conn.Dest.Addr)
	require.Equal(t, update.Remote.Port, conn.DPort)
	require.False(t, tracer.tuples.match(oldTuple).matched)
	match := tracer.tuples.match(conn.ConnectionTuple)
	require.True(t, match.matched)
	require.Equal(t, conn.Cookie, match.cookie)
}

func TestNStatTracerRuntimeFailureClosesActiveGenerationOnce(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(375, 0)
	tracer.now = func() time.Time { return now }
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}
	var failures []error
	tracer.setRuntimeFailureCallback(func(err error) {
		failures = append(failures, err)
	})

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 11,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1111, tcpStateEstablished),
	})
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 12,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      testNStatUDPFlow(2222),
	})

	now = now.Add(2 * time.Second)
	runtimeErr := errors.New("ABI changed")
	tracer.handleRuntimeFailure(runtimeErr)
	tracer.handleRuntimeFailure(errors.New("second failure"))

	require.Equal(t, []error{runtimeErr}, failures)
	require.Len(t, closed, 2)
	for _, conn := range closed {
		require.True(t, conn.IsClosed)
		require.Equal(t, 2*time.Second, conn.Duration)
	}
	require.Empty(t, tracer.sources)
	var buffer network.ConnectionBuffer
	require.ErrorIs(t, tracer.GetConnections(&buffer, nil), runtimeErr)
}

func TestNStatTracerQueryAllTreatsBackpressureAsTransient(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)

	control.queryErr = unix.EAGAIN
	require.NoError(t, tracer.queryAll())

	control.queryErr = io.ErrUnexpectedEOF
	require.ErrorIs(t, tracer.queryAll(), io.ErrUnexpectedEOF)
}

func TestNStatTracerBoundsAndRetriesDescriptionRequests(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(390, 0)
	tracer.now = func() time.Time { return now }
	for sourceRef := uint64(1); sourceRef <= nstatDescriptionBatch+2; sourceRef++ {
		tracer.processEvent(nstat.Event{
			Kind:      nstat.EventAdded,
			SourceRef: sourceRef,
			Provider:  nstat.ProviderTCPKernel,
		})
	}

	tracer.pumpDescriptionRequests(now)
	require.Len(t, control.descriptions(), nstatDescriptionBatch)
	tracer.queueUnresolvedDescriptions(now)
	tracer.pumpDescriptionRequests(now)
	require.Len(t, control.descriptions(), nstatDescriptionBatch+2)

	now = now.Add(nstatDescriptionRetry)
	tracer.queueUnresolvedDescriptions(now)
	tracer.pumpDescriptionRequests(now)
	require.Len(t, control.descriptions(), 2*nstatDescriptionBatch+2)
}

func TestNStatTracerPrioritizesNewDescriptionsWithoutStarvingRetries(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(395, 0)
	tracer.now = func() time.Time { return now }

	for sourceRef := uint64(1); sourceRef <= nstatDescriptionBatch+2; sourceRef++ {
		tracer.processEvent(nstat.Event{
			Kind:      nstat.EventDescription,
			SourceRef: sourceRef,
			Provider:  nstat.ProviderTCPKernel,
			Flow: &nstat.Flow{
				PID: uint32(sourceRef),
				Local: nstat.Endpoint{
					Address: netip.MustParseAddr("192.0.2.10"),
					Port:    uint16(40000 + sourceRef),
					Present: true,
				},
			},
		})
		tracer.mu.Lock()
		tracer.queueDescriptionLocked(sourceRef, tracer.sources[sourceRef])
		tracer.mu.Unlock()
	}

	for _, sourceRef := range []uint64{100, 101} {
		tracer.processEvent(nstat.Event{
			Kind:      nstat.EventAdded,
			SourceRef: sourceRef,
			Provider:  nstat.ProviderUDPKernel,
		})
	}

	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{100, 101, 1, 2, 3, 4, 5, 6}, control.descriptions())

	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{100, 101, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, control.descriptions())
}

func TestNStatTracerStartAndStop(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)

	require.NoError(t, tracer.Start(func(*network.ConnectionStats) {}))
	require.ErrorContains(t, tracer.Start(func(*network.ConnectionStats) {}), "already started")
	<-control.pollStarted
	tracer.Stop()
	tracer.Stop()
	require.ErrorIs(t, tracer.Start(func(*network.ConnectionStats) {}), io.ErrClosedPipe)

	require.Equal(t, nstatProviders[:], control.subscriptions())
	require.True(t, control.isClosed())
}

func TestNStatConversions(t *testing.T) {
	require.Equal(t, uint64(0), saturatingSubtract(10, 11))
	require.Equal(t, uint64(0), saturatingSubtract(10, 10))
	require.Equal(t, uint64(9), saturatingSubtract(10, 1))
	require.Equal(t, uint32(28281), scaledMicroseconds(905, nstatTCPRTTScale))

	current := testNStatTCPFlow(1234, tcpStateEstablished)
	update := &nstat.Flow{
		TCPState: tcpStateClosed,
		Local: nstat.Endpoint{
			Address: netip.IPv4Unspecified(),
			Port:    current.Local.Port,
			Present: true,
		},
		Remote: nstat.Endpoint{
			Address: netip.IPv4Unspecified(),
			Present: true,
		},
	}
	merged := mergeNStatFlow(current, update)
	require.Equal(t, current.PID, merged.PID)
	require.Equal(t, current.UniquePID, merged.UniquePID)
	require.Equal(t, current.Local, merged.Local)
	require.Equal(t, current.Remote, merged.Remote)
	require.Equal(t, uint32(tcpStateClosed), merged.TCPState)
}

func testNStatConfig() *config.Config {
	return &config.Config{MaxTrackedConnections: 100}
}

func testNStatTCPFlow(pid, state uint32) *nstat.Flow {
	return &nstat.Flow{
		UniquePID:      uint64(pid) << 32,
		PID:            pid,
		InterfaceIndex: 14,
		TCPState:       state,
		Local: nstat.Endpoint{
			Address: netip.MustParseAddr("192.0.2.10"),
			Port:    50000,
			Present: true,
		},
		Remote: nstat.Endpoint{
			Address: netip.MustParseAddr("198.51.100.20"),
			Port:    443,
			Present: true,
		},
	}
}

func testNStatUDPFlow(pid uint32) *nstat.Flow {
	flow := testNStatTCPFlow(pid, 0)
	return flow
}

type fakeNStatControl struct {
	mu sync.Mutex

	subscribed          []uint32
	descriptionRequests []uint64
	closed              chan struct{}
	closeOnce           sync.Once
	pollStarted         chan struct{}
	pollOnce            sync.Once
	queryErr            error
}

func newFakeNStatControl() *fakeNStatControl {
	return &fakeNStatControl{
		closed:      make(chan struct{}),
		pollStarted: make(chan struct{}),
	}
}

func (f *fakeNStatControl) Subscribe(provider uint32) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscribed = append(f.subscribed, provider)
	return 0, nil
}

func (f *fakeNStatControl) QueryAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queryErr
}

func (f *fakeNStatControl) RequestDescription(sourceRef uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.descriptionRequests = append(f.descriptionRequests, sourceRef)
	return nil
}

func (f *fakeNStatControl) Poll(time.Duration) (bool, error) {
	f.pollOnce.Do(func() { close(f.pollStarted) })
	<-f.closed
	return false, io.ErrClosedPipe
}

func (f *fakeNStatControl) Receive([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func (f *fakeNStatControl) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeNStatControl) subscriptions() []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint32(nil), f.subscribed...)
}

func (f *fakeNStatControl) descriptions() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint64(nil), f.descriptionRequests...)
}

func (f *fakeNStatControl) isClosed() bool {
	select {
	case <-f.closed:
		return true
	default:
		return false
	}
}
