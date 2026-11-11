// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

func TestDarwinRuntimeFallbackTracerSwitchesOnce(t *testing.T) {
	control := newFakeNStatControl()
	primary := newNStatTracerWithControl(testNStatConfig(), control)
	fallback := &recordingTracer{
		connection: network.ConnectionStats{
			ConnectionTuple: network.ConnectionTuple{Pid: 9999},
		},
	}
	var factoryCalls int
	tracer := newDarwinRuntimeFallbackTracer(primary, func() (Tracer, error) {
		factoryCalls++
		return fallback, nil
	}).(*darwinRuntimeFallbackTracer)

	var closed []*network.ConnectionStats
	require.Equal(t, TracerTypeNStat, tracer.Type())
	require.NoError(t, tracer.Start(func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}))
	primary.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 13,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(3333, tcpStateEstablished),
	})

	runtimeErr := errors.New("control disconnected")
	primary.handleRuntimeFailure(runtimeErr)
	require.NoError(t, tracer.switchToFallback(runtimeErr))

	require.Equal(t, 1, factoryCalls)
	require.True(t, control.isClosed())
	require.Len(t, closed, 1)
	require.Equal(t, uint32(3333), closed[0].Pid)
	require.True(t, closed[0].IsClosed)
	require.True(t, fallback.isStarted())
	require.Equal(t, TracerTypeEbpfless, tracer.Type())
	status := GetDarwinTracerStatus(tracer)
	require.Equal(t, "ebpfless", status.ActiveBackend)
	require.Equal(t, nstat.ABIRevision, status.ABIRevision)
	require.True(t, status.RuntimeFallback)
	require.Equal(t, runtimeErr.Error(), status.LastError)

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(9999), buffer.Connections()[0].Pid)

	require.NoError(t, tracer.switchToFallback(errors.New("ignored")))
	require.Equal(t, 1, factoryCalls)
	tracer.Stop()
	require.True(t, fallback.isStopped())
}

func TestDarwinRuntimeFallbackTracerServesEmptySnapshotDuringHandoff(t *testing.T) {
	control := newFakeNStatControl()
	primary := newNStatTracerWithControl(testNStatConfig(), control)
	fallback := &recordingTracer{}
	factoryStarted := make(chan struct{})
	releaseFactory := make(chan struct{})
	tracer := newDarwinRuntimeFallbackTracer(primary, func() (Tracer, error) {
		close(factoryStarted)
		<-releaseFactory
		return fallback, nil
	}).(*darwinRuntimeFallbackTracer)
	require.NoError(t, tracer.Start(func(*network.ConnectionStats) {}))

	runtimeErr := errors.New("control disconnected")
	primary.handleRuntimeFailure(runtimeErr)
	<-factoryStarted

	var buffer network.ConnectionBuffer
	buffer.Append([]network.ConnectionStats{{
		ConnectionTuple: network.ConnectionTuple{Pid: 1234},
	}})
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Empty(t, buffer.Connections())

	close(releaseFactory)
	require.NoError(t, tracer.switchToFallback(runtimeErr))
	tracer.Stop()
}

func TestDarwinRuntimeFallbackTracerReportsFallbackFailure(t *testing.T) {
	control := newFakeNStatControl()
	primary := newNStatTracerWithControl(testNStatConfig(), control)
	fallbackErr := errors.New("cannot open BPF")
	tracer := newDarwinRuntimeFallbackTracer(primary, func() (Tracer, error) {
		return nil, fallbackErr
	}).(*darwinRuntimeFallbackTracer)
	require.NoError(t, tracer.Start(func(*network.ConnectionStats) {}))

	err := tracer.switchToFallback(errors.New("nstat failed"))

	require.ErrorIs(t, err, fallbackErr)
	var buffer network.ConnectionBuffer
	require.ErrorIs(t, tracer.GetConnections(&buffer, nil), fallbackErr)
	status := GetDarwinTracerStatus(tracer)
	require.Equal(t, "unavailable", status.ActiveBackend)
	require.False(t, status.SourceHealthy)
	require.Contains(t, status.LastError, fallbackErr.Error())
	tracer.Stop()
}

func TestDarwinRuntimeFallbackTracerHandlesStartupFailure(t *testing.T) {
	primaryErr := errors.New("subscription rejected")
	primary := &failureAwareRecordingTracer{
		recordingTracer: &recordingTracer{startErr: primaryErr},
	}
	fallback := &recordingTracer{}
	tracer := newDarwinRuntimeFallbackTracer(primary, func() (Tracer, error) {
		return fallback, nil
	}).(*darwinRuntimeFallbackTracer)

	require.NoError(t, tracer.Start(func(*network.ConnectionStats) {}))
	require.True(t, primary.isStopped())
	require.True(t, fallback.isStarted())
	tracer.Stop()
}

func TestDarwinStatusErrorIsBoundedAndSerializable(t *testing.T) {
	message := strings.Repeat("界", darwinStatusErrorLimit) + "\nsecret"
	got := boundedDarwinStatusError(errors.New(message))

	require.LessOrEqual(t, len(got), darwinStatusErrorLimit)
	require.True(t, utf8.ValidString(got))
	require.NotContains(t, got, "\n")
}

type recordingTracer struct {
	mu sync.Mutex

	started    bool
	stopped    bool
	startErr   error
	connection network.ConnectionStats
}

func (t *recordingTracer) Start(func(*network.ConnectionStats)) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.startErr != nil {
		return t.startErr
	}
	t.started = true
	return nil
}

func (t *recordingTracer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
}

func (t *recordingTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if filter == nil || filter(&t.connection) {
		buffer.Append([]network.ConnectionStats{t.connection})
	}
	return nil
}

func (t *recordingTracer) FlushPending() {}

func (t *recordingTracer) Remove(*network.ConnectionStats) error { return nil }

func (t *recordingTracer) GetMap(string) (*ebpf.Map, error) { return nil, nil }

func (t *recordingTracer) DumpMaps(io.Writer, ...string) error { return nil }

func (t *recordingTracer) Type() TracerType { return TracerTypeEbpfless }

func (t *recordingTracer) Pause() error  { return nil }
func (t *recordingTracer) Resume() error { return nil }

func (t *recordingTracer) Describe(chan<- *prometheus.Desc) {}
func (t *recordingTracer) Collect(chan<- prometheus.Metric) {}

func (t *recordingTracer) isStarted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.started
}

func (t *recordingTracer) isStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

var _ Tracer = (*recordingTracer)(nil)

type failureAwareRecordingTracer struct {
	*recordingTracer
	callback func(error)
}

func (t *failureAwareRecordingTracer) setRuntimeFailureCallback(callback func(error)) {
	t.callback = callback
}

var _ runtimeFailureAware = (*failureAwareRecordingTracer)(nil)
