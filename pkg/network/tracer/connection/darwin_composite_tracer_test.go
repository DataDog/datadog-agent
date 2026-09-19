// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

func TestDarwinCompositePrimaryFailureClosesGenerationAndSidecarsOnce(t *testing.T) {
	control := newFakeNStatControl()
	primary := newNStatTracerWithControl(testNStatConfig(), control)
	primary.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1234, tcpStateEstablished),
	})
	packetSource := newBlockingDarwinPacketSource()
	packet := newDarwinPacketSidecar(packetSource, primary, 10)
	composite := newDarwinCompositeTracerWithComponents(primary, packet, nil)

	var mu sync.Mutex
	var closed int
	var failures int
	composite.setRuntimeFailureCallback(func(error) {
		mu.Lock()
		failures++
		mu.Unlock()
	})
	require.NoError(t, composite.Start(func(*network.ConnectionStats) {
		mu.Lock()
		closed++
		mu.Unlock()
	}))
	<-control.pollStarted
	<-packetSource.started

	primary.handleRuntimeFailure(errors.New("fatal nstat failure"))
	primary.handleRuntimeFailure(errors.New("duplicate failure"))
	composite.Stop()

	mu.Lock()
	require.Equal(t, 1, closed)
	require.Equal(t, 1, failures)
	mu.Unlock()
	require.Equal(t, 1, packetSource.closeCount())
}

func TestDarwinCompositeRejectsMissingPrimary(t *testing.T) {
	composite := newDarwinCompositeTracerWithComponents(nil, nil, nil)
	require.ErrorContains(t, composite.Start(nil), "no authoritative NStat source")
	composite.Stop()
}

func TestDarwinCompositeRemoveCleansPacketStateAfterPrimary(t *testing.T) {
	primary := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	primary.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1234, tcpStateEstablished),
	})
	var buffer network.ConnectionBuffer
	require.NoError(t, primary.GetConnections(&buffer, nil))
	conn := buffer.Connections()[0]
	packet := newDarwinPacketSidecar(&fakeDarwinPacketSource{}, primary, 10)
	packet.analyzer.process(conn.Cookie, true, false, &layers.TCP{Seq: 1, SYN: true})
	composite := newDarwinCompositeTracerWithComponents(primary, packet, nil)

	require.NoError(t, composite.Remove(&conn))

	buffer.Reset()
	require.NoError(t, primary.GetConnections(&buffer, nil))
	require.Empty(t, buffer.Connections())
	packet.analyzer.mu.Lock()
	_, present := packet.analyzer.flows[conn.Cookie]
	packet.analyzer.mu.Unlock()
	require.False(t, present)
}

type blockingDarwinPacketSource struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once

	mu     sync.Mutex
	closes int
}

func newBlockingDarwinPacketSource() *blockingDarwinPacketSource {
	return &blockingDarwinPacketSource{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (s *blockingDarwinPacketSource) VisitPackets(func([]byte, filter.PacketInfo, time.Time) error) error {
	s.once.Do(func() { close(s.started) })
	<-s.closed
	return nil
}

func (s *blockingDarwinPacketSource) LayerType() gopacket.LayerType { return gopacket.LayerTypeZero }

func (s *blockingDarwinPacketSource) Close() {
	s.mu.Lock()
	s.closes++
	s.mu.Unlock()
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
}

func (s *blockingDarwinPacketSource) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closes
}
