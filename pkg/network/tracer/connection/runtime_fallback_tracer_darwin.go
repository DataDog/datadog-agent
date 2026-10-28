// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var darwinRuntimeFallbackTelemetry = struct {
	switches telemetry.Counter
}{
	switches: telemetryimpl.GetCompatComponent().NewCounter(
		"network_tracer__darwin",
		"runtime_fallbacks",
		[]string{"result"},
		"Darwin connection tracer runtime fallback attempts",
	),
}

type runtimeFailureAware interface {
	setRuntimeFailureCallback(func(error))
}

// darwinRuntimeFallbackTracer performs a one-way handoff from NStat to the
// existing eBPF-less tracer. It never runs two primary emitters concurrently.
type darwinRuntimeFallbackTracer struct {
	mu sync.RWMutex

	active          Tracer
	fallbackFactory func() (Tracer, error)
	closeCallback   func(*network.ConnectionStats)
	started         bool
	stopped         bool
	runtimeErr      error
	fallbackActive  bool
	fallbackCause   error

	switchOnce sync.Once
	switchErr  error
	switching  atomic.Bool
}

func newDarwinRuntimeFallbackTracer(primary Tracer, fallbackFactory func() (Tracer, error)) Tracer {
	failureAware, ok := primary.(runtimeFailureAware)
	if !ok {
		return primary
	}
	tracer := &darwinRuntimeFallbackTracer{
		active:          primary,
		fallbackFactory: fallbackFactory,
	}
	failureAware.setRuntimeFailureCallback(tracer.onRuntimeFailure)
	return tracer
}

func (t *darwinRuntimeFallbackTracer) onRuntimeFailure(err error) {
	if !t.switching.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer t.switching.Store(false)
		if switchErr := t.switchToFallback(err); switchErr != nil &&
			!errors.Is(switchErr, io.ErrClosedPipe) {
			log.Errorf("could not fall back from NStat to eBPF-less tracing: %v", switchErr)
		}
	}()
}

func (t *darwinRuntimeFallbackTracer) switchToFallback(cause error) error {
	t.switchOnce.Do(func() {
		t.switchErr = t.performSwitch(cause)
	})
	return t.switchErr
}

func (t *darwinRuntimeFallbackTracer) performSwitch(cause error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer t.switching.Store(false)
	if t.stopped {
		return io.ErrClosedPipe
	}

	t.active.Stop()
	t.fallbackCause = cause
	fallback, err := t.fallbackFactory()
	if err != nil {
		darwinRuntimeFallbackTelemetry.switches.Inc("create_error")
		t.runtimeErr = fmt.Errorf("create eBPF-less fallback after %v: %w", cause, err)
		return t.runtimeErr
	}
	if t.started {
		if err := fallback.Start(t.closeCallback); err != nil {
			fallback.Stop()
			darwinRuntimeFallbackTelemetry.switches.Inc("start_error")
			t.runtimeErr = fmt.Errorf("start eBPF-less fallback after %v: %w", cause, err)
			return t.runtimeErr
		}
	}
	t.active = fallback
	t.runtimeErr = nil
	t.fallbackActive = true
	darwinRuntimeFallbackTelemetry.switches.Inc("success")
	log.Warnf("switched Darwin connection tracing from NStat to eBPF-less after runtime failure: %v", cause)
	return nil
}

func (t *darwinRuntimeFallbackTracer) Start(closeCallback func(*network.ConnectionStats)) error {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return io.ErrClosedPipe
	}
	if t.started {
		t.mu.Unlock()
		return errors.New("Darwin runtime fallback tracer already started")
	}
	t.closeCallback = closeCallback
	t.started = true
	if err := t.active.Start(closeCallback); err != nil {
		t.mu.Unlock()
		return t.switchToFallback(err)
	}
	t.mu.Unlock()
	return nil
}

func (t *darwinRuntimeFallbackTracer) Stop() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	t.active.Stop()
}

func (t *darwinRuntimeFallbackTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	if t.switching.Load() {
		buffer.Reset()
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.runtimeErr != nil {
		return t.runtimeErr
	}
	err := t.active.GetConnections(buffer, filter)
	if t.switching.Load() {
		buffer.Reset()
		return nil
	}
	return err
}

func (t *darwinRuntimeFallbackTracer) FlushPending() {
	t.mu.RLock()
	defer t.mu.RUnlock()
	t.active.FlushPending()
}

func (t *darwinRuntimeFallbackTracer) Remove(conn *network.ConnectionStats) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.runtimeErr != nil {
		return t.runtimeErr
	}
	return t.active.Remove(conn)
}

func (t *darwinRuntimeFallbackTracer) GetMap(name string) (*ebpf.Map, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.runtimeErr != nil {
		return nil, t.runtimeErr
	}
	return t.active.GetMap(name)
}

func (t *darwinRuntimeFallbackTracer) DumpMaps(writer io.Writer, maps ...string) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.runtimeErr != nil {
		return t.runtimeErr
	}
	return t.active.DumpMaps(writer, maps...)
}

func (t *darwinRuntimeFallbackTracer) Type() TracerType {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.active.Type()
}

func (t *darwinRuntimeFallbackTracer) darwinStatus() DarwinTracerStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.runtimeErr != nil {
		status := nstatStatus()
		status.ActiveBackend = "unavailable"
		status.SourceHealthy = false
		status.LastError = boundedDarwinStatusError(t.runtimeErr)
		return status
	}
	status := GetDarwinTracerStatus(t.active)
	if t.fallbackActive {
		status.ABIRevision = nstatStatus().ABIRevision
		status.RuntimeFallback = true
		status.LastError = boundedDarwinStatusError(t.fallbackCause)
	}
	return status
}

func (t *darwinRuntimeFallbackTracer) Pause() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.runtimeErr != nil {
		return t.runtimeErr
	}
	return t.active.Pause()
}

func (t *darwinRuntimeFallbackTracer) Resume() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.runtimeErr != nil {
		return t.runtimeErr
	}
	return t.active.Resume()
}

func (t *darwinRuntimeFallbackTracer) Describe(descs chan<- *prometheus.Desc) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	t.active.Describe(descs)
}

func (t *darwinRuntimeFallbackTracer) Collect(metrics chan<- prometheus.Metric) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	t.active.Collect(metrics)
}

var _ Tracer = (*darwinRuntimeFallbackTracer)(nil)
