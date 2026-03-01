// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// ============================================================================
// Mock connection.Tracer
// ============================================================================

type mockConnTracer struct {
	startErr      error
	getConnsErr   error
	stopCalled    bool
	startCallback func(*network.ConnectionStats)
	connsToReturn []*network.ConnectionStats
}

func (m *mockConnTracer) Start(cb func(*network.ConnectionStats)) error {
	m.startCallback = cb
	return m.startErr
}

func (m *mockConnTracer) Stop() {
	m.stopCalled = true
}

func (m *mockConnTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	if m.getConnsErr != nil {
		return m.getConnsErr
	}
	for _, c := range m.connsToReturn {
		if filter == nil || filter(c) {
			next := buffer.Next()
			*next = *c
		}
	}
	return nil
}

func (m *mockConnTracer) FlushPending() {}

func (m *mockConnTracer) Remove(_ *network.ConnectionStats) error { return nil }

func (m *mockConnTracer) GetMap(_ string) (*ebpf.Map, error) { return nil, nil }

func (m *mockConnTracer) DumpMaps(_ io.Writer, _ ...string) error { return nil }

func (m *mockConnTracer) Type() connection.TracerType { return connection.TracerTypeDarwin }

func (m *mockConnTracer) Pause() error { return nil }

func (m *mockConnTracer) Resume() error { return nil }

func (m *mockConnTracer) Describe(_ chan<- *prometheus.Desc) {}

func (m *mockConnTracer) Collect(_ chan<- prometheus.Metric) {}

// ============================================================================
// Mock network.State
// ============================================================================

type mockNetworkState struct {
	registerClientCalls []string
	closedConns         []*network.ConnectionStats
}

func (m *mockNetworkState) GetDelta(_ string, _ uint64, _ []network.ConnectionStats, _ dns.StatsByKeyByNameByType, _ map[protocols.ProtocolType]interface{}) network.Delta {
	return network.Delta{}
}

func (m *mockNetworkState) GetTelemetryDelta(_ string, _ map[network.ConnTelemetryType]int64) map[network.ConnTelemetryType]int64 {
	return nil
}

func (m *mockNetworkState) RegisterClient(clientID string) {
	m.registerClientCalls = append(m.registerClientCalls, clientID)
}

func (m *mockNetworkState) RemoveClient(_ string) {}

func (m *mockNetworkState) RemoveExpiredClients(_ time.Time) {}

func (m *mockNetworkState) RemoveConnections(_ []*network.ConnectionStats) {}

func (m *mockNetworkState) StoreClosedConnection(conn *network.ConnectionStats) {
	m.closedConns = append(m.closedConns, conn)
}

func (m *mockNetworkState) GetStats() map[string]interface{} { return nil }

func (m *mockNetworkState) DumpState(_ string) map[string]interface{} { return nil }

// newTestTracer builds a Tracer directly without opening pcap handles.
func newTestTracer(connTracer connection.Tracer, state network.State) *Tracer {
	cfg := config.New()
	return &Tracer{
		config:     cfg,
		connTracer: connTracer,
		state:      state,
		reverseDNS: dns.NewNullReverseDNS(),
	}
}

func TestDarwinTracer_Stop_CallsConnTracerStop(t *testing.T) {
	mock := &mockConnTracer{}
	tr := newTestTracer(mock, &mockNetworkState{})

	tr.Stop()

	assert.True(t, mock.stopCalled, "Stop() must call connTracer.Stop()")
}

func TestDarwinTracer_RegisterClient(t *testing.T) {
	state := &mockNetworkState{}
	tr := newTestTracer(&mockConnTracer{}, state)

	err := tr.RegisterClient("client-1")
	require.NoError(t, err)

	assert.Equal(t, []string{"client-1"}, state.registerClientCalls)
}

func TestDarwinTracer_GetActiveConnections_PropagatesTracerError(t *testing.T) {
	wantErr := errors.New("pcap read error")
	mock := &mockConnTracer{getConnsErr: wantErr}
	tr := newTestTracer(mock, &mockNetworkState{})

	_ = tr.RegisterClient("c1")

	_, _, err := tr.GetActiveConnections("c1")
	require.Error(t, err)
	assert.ErrorContains(t, err, "pcap read error")
}

func TestDarwinTracer_GetActiveConnections_ReturnsConnections(t *testing.T) {
	conn := &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("10.0.0.2"),
			SPort:  12345,
			DPort:  80,
		},
	}
	mock := &mockConnTracer{connsToReturn: []*network.ConnectionStats{conn}}
	state := &mockNetworkState{}
	tr := newTestTracer(mock, state)

	_ = tr.RegisterClient("c1")

	conns, cleanup, err := tr.GetActiveConnections("c1")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	cleanup()

	// The mock state's GetDelta returns an empty Delta, so no connections come
	// through the delta path — but the call should succeed without error.
	assert.NotNil(t, conns)
}

func TestDarwinTracer_StoreClosedConnection_SkipsExcluded(t *testing.T) {
	state := &mockNetworkState{}
	tr := newTestTracer(&mockConnTracer{}, state)

	// A connection on the configured DNS port (53) to loopback is excluded when
	// CollectLocalDNS is false (the default).
	dnsConn := &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("127.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  53,
		},
	}
	tr.storeClosedConnection(dnsConn)

	assert.Empty(t, state.closedConns, "local DNS connection should be skipped")
}

func TestDarwinTracer_StoreClosedConnection_StoresAllowed(t *testing.T) {
	state := &mockNetworkState{}
	tr := newTestTracer(&mockConnTracer{}, state)

	conn := &network.ConnectionStats{
		ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("8.8.8.8"),
			DPort:  443,
		},
	}
	tr.storeClosedConnection(conn)

	require.Len(t, state.closedConns, 1)
	assert.Equal(t, conn, state.closedConns[0])
}

func TestDarwinTracer_GetStats_ReturnsExpectedKeys(t *testing.T) {
	tr := newTestTracer(&mockConnTracer{}, &mockNetworkState{})

	stats, err := tr.GetStats()
	require.NoError(t, err)

	assert.Contains(t, stats, "state")
	assert.Contains(t, stats, "tracer")

	tracerStats, ok := stats["tracer"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "darwin_pcap", tracerStats["type"])
}

func TestDarwinTracer_DebugNetworkState(t *testing.T) {
	tr := newTestTracer(&mockConnTracer{}, &mockNetworkState{})

	info, err := tr.DebugNetworkState("client-x")
	require.NoError(t, err)
	assert.Equal(t, "darwin_pcap", info["tracer_type"])
}

func TestDarwinTracer_DebugEBPFMaps_WritesMessage(t *testing.T) {
	tr := newTestTracer(&mockConnTracer{}, &mockNetworkState{})

	var buf mockWriter
	err := tr.DebugEBPFMaps(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.data, "eBPF maps not available on Darwin")
}

func TestDarwinTracer_DebugCachedConntrack_ReturnsError(t *testing.T) {
	tr := newTestTracer(&mockConnTracer{}, &mockNetworkState{})
	_, err := tr.DebugCachedConntrack(nil)
	require.Error(t, err)
}

func TestDarwinTracer_StaticTable_ReturnsNil(t *testing.T) {
	tr := newTestTracer(&mockConnTracer{}, &mockNetworkState{})
	assert.Nil(t, tr.StaticTable(0))
}

// mockWriter captures bytes written to it.
type mockWriter struct {
	data string
}

func (w *mockWriter) Write(p []byte) (int, error) {
	w.data += string(p)
	return len(p), nil
}
