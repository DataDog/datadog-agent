// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"io"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/libproc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// darwinCompositeTracer coordinates an authoritative NStat source with
// optional packet and libproc sidecars. NStat alone owns connection creation,
// counters, and lifecycle; the sidecars may only enrich an existing NStat
// connection with packet evidence or otherwise missing process identity.
type darwinCompositeTracer struct {
	primary    *nstatTracer
	packet     *darwinPacketSidecar
	reconciler *darwinLibprocReconciler

	mu                     sync.Mutex
	started                bool
	stopped                bool
	runtimeFailureCallback func(error)
	stopSidecarsOnce       sync.Once
}

// newDarwinCompositeTracer constructs the complete backend without selecting
// it from the production Darwin tracer path.
//
//nolint:unused // The production backend selection is introduced in the integration commit.
func newDarwinCompositeTracer(cfg *config.Config) (*darwinCompositeTracer, error) {
	primary, err := newNStatTracer(cfg)
	if err != nil {
		return nil, err
	}
	composite := newDarwinCompositeTracerWithComponents(primary, nil, nil)

	packetSource, packetErr := filter.NewLibpcapSource(
		filter.OptSnapLen(darwinPrefixLimit),
		filter.OptBPFFilter("tcp"),
	)
	if packetErr != nil {
		log.Warnf("Darwin NStat packet enrichment unavailable: %v", packetErr)
	} else {
		packetFanout := filter.NewPacketSourceFanout(packetSource)
		composite.packet = newDarwinPacketSidecar(packetFanout, primary, int(cfg.MaxTrackedConnections))
	}

	scanner, scannerErr := libproc.NewNativeScanner(libproc.DefaultLimits)
	if scannerErr != nil {
		log.Warnf("Darwin NStat libproc reconciliation unavailable: %v", scannerErr)
	} else {
		composite.reconciler = newDarwinLibprocReconciler(scanner, primary, darwinLibprocInterval)
	}
	return composite, nil
}

func newDarwinCompositeTracerWithComponents(
	primary *nstatTracer,
	packet *darwinPacketSidecar,
	reconciler *darwinLibprocReconciler,
) *darwinCompositeTracer {
	composite := &darwinCompositeTracer{
		primary:    primary,
		packet:     packet,
		reconciler: reconciler,
	}
	if primary != nil {
		primary.setRuntimeFailureCallback(composite.handlePrimaryFailure)
	}
	return composite
}

func (t *darwinCompositeTracer) Start(closeCallback func(*network.ConnectionStats)) error {
	t.mu.Lock()
	if t.primary == nil {
		t.mu.Unlock()
		return errors.New("Darwin composite tracer has no authoritative NStat source")
	}
	if t.stopped {
		t.mu.Unlock()
		return io.ErrClosedPipe
	}
	if t.started {
		t.mu.Unlock()
		return errors.New("Darwin composite tracer already started")
	}
	t.started = true
	t.mu.Unlock()

	wrappedClose := func(conn *network.ConnectionStats) {
		if t.packet != nil {
			t.packet.remove(conn.Cookie)
		}
		if closeCallback != nil {
			closeCallback(conn)
		}
	}
	if err := t.primary.Start(wrappedClose); err != nil {
		t.stopSidecars()
		t.primary.Stop()
		t.mu.Lock()
		t.stopped = true
		t.mu.Unlock()
		return err
	}
	if t.packet != nil {
		t.packet.start()
	}
	if t.reconciler != nil {
		t.reconciler.start()
	}
	return nil
}

func (t *darwinCompositeTracer) Stop() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	t.mu.Unlock()
	t.stopSidecars()
	if t.primary != nil {
		t.primary.Stop()
	}
}

func (t *darwinCompositeTracer) stopSidecars() {
	t.stopSidecarsOnce.Do(func() {
		if t.packet != nil {
			t.packet.stop()
		}
		if t.reconciler != nil {
			t.reconciler.stop()
		}
	})
}

func (t *darwinCompositeTracer) handlePrimaryFailure(err error) {
	t.stopSidecars()
	t.mu.Lock()
	callback := t.runtimeFailureCallback
	t.mu.Unlock()
	if callback != nil {
		callback(err)
	}
}

func (t *darwinCompositeTracer) setRuntimeFailureCallback(callback func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.runtimeFailureCallback = callback
}

func (t *darwinCompositeTracer) GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error {
	return t.primary.GetConnections(buffer, filter)
}

func (t *darwinCompositeTracer) FlushPending() {
	t.primary.FlushPending()
}

func (t *darwinCompositeTracer) Remove(conn *network.ConnectionStats) error {
	err := t.primary.Remove(conn)
	if t.packet != nil {
		t.packet.remove(conn.Cookie)
	}
	return err
}

func (t *darwinCompositeTracer) GetMap(name string) (*ebpf.Map, error) {
	return t.primary.GetMap(name)
}

func (t *darwinCompositeTracer) DumpMaps(writer io.Writer, maps ...string) error {
	return t.primary.DumpMaps(writer, maps...)
}

func (t *darwinCompositeTracer) Type() TracerType {
	return TracerTypeNStat
}

func (t *darwinCompositeTracer) Pause() error {
	return t.primary.Pause()
}

func (t *darwinCompositeTracer) Resume() error {
	return t.primary.Resume()
}

func (t *darwinCompositeTracer) Describe(channel chan<- *prometheus.Desc) {
	t.primary.Describe(channel)
}

func (t *darwinCompositeTracer) Collect(channel chan<- prometheus.Metric) {
	t.primary.Collect(channel)
}

var _ Tracer = (*darwinCompositeTracer)(nil)
