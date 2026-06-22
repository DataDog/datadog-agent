// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package crashreceiver

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/xreceiver"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/reporter/samples"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// collectWindow is how long we wait after the first crash event for a given
// process before flushing. All threads of a dying process fire do_exit within
// milliseconds of each other; 500ms gives ample headroom for ring-buffer
// delivery without adding meaningful latency to the crash report.
const collectWindow = 500 * time.Millisecond

type threadEvent struct {
	trace *libpf.Trace
	meta  *samples.TraceEventMeta
}

type pendingCrash struct {
	events []*threadEvent
	timer  *time.Timer
}

// CrashReporter implements reporter.Reporter for crash-origin traces.
// It groups events by PID within a short collection window so that all
// threads of a crashing process are bundled into a single payload.
//
// Thread model: mu serialises all field access. consumer and ctx are written
// once each (by setConsumer / Start) before the pipeline becomes active, then
// read under mu during flush so they can be copied out before the lock is
// released. ConsumeProfiles is always called outside the lock.
type CrashReporter struct {
	mu       sync.Mutex
	consumer xconsumer.Profiles
	ctx      context.Context
	pending  map[libpf.PID]*pendingCrash
}

func NewCrashReporter() *CrashReporter {
	return &CrashReporter{
		pending: make(map[libpf.PID]*pendingCrash),
	}
}

func (r *CrashReporter) setConsumer(c xconsumer.Profiles) {
	r.mu.Lock()
	r.consumer = c
	r.mu.Unlock()
	log.Info("crash receiver: consumer registered, crash events will be forwarded to the crash pipeline")
}

func (r *CrashReporter) Start(ctx context.Context) error {
	r.mu.Lock()
	r.ctx = ctx
	r.mu.Unlock()
	return nil
}

// Stop flushes any pending crash bundles before shutdown.
func (r *CrashReporter) Stop() {
	r.mu.Lock()
	pids := make([]libpf.PID, 0, len(r.pending))
	for pid := range r.pending {
		pids = append(pids, pid)
	}
	r.mu.Unlock()

	for _, pid := range pids {
		r.flush(pid)
	}
}

func (r *CrashReporter) ReportTraceEvent(trace *libpf.Trace, meta *samples.TraceEventMeta) error {
	pid := meta.PID
	ev := &threadEvent{trace: trace, meta: meta}

	r.mu.Lock()
	p, exists := r.pending[pid]
	if !exists {
		p = &pendingCrash{}
		p.timer = time.AfterFunc(collectWindow, func() { r.flush(pid) })
		r.pending[pid] = p
		log.Infof("crash receiver: first thread captured — pid=%d comm=%s signal=%d frames=%d tid=%d",
			meta.PID, meta.Comm, meta.Value, len(trace.Frames), meta.TID)
	} else {
		log.Infof("crash receiver: additional thread captured — pid=%d tid=%d frames=%d",
			meta.PID, meta.TID, len(trace.Frames))
	}
	p.events = append(p.events, ev)
	r.mu.Unlock()

	return nil
}

func (r *CrashReporter) flush(pid libpf.PID) {
	r.mu.Lock()
	p, ok := r.pending[pid]
	if !ok {
		r.mu.Unlock()
		return
	}
	p.timer.Stop()
	delete(r.pending, pid)
	events := p.events
	c := r.consumer
	ctx := r.ctx
	r.mu.Unlock()

	if c == nil {
		log.Warnf("crash receiver: dropping %d event(s) for pid=%d (no consumer registered)", len(events), pid)
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	log.Infof("crash receiver: flushing crash bundle — pid=%d threads=%d", pid, len(events))
	if err := c.ConsumeProfiles(ctx, buildBundledProfile(events)); err != nil {
		log.Errorf("crash receiver: failed to deliver crash bundle for pid=%d: %v", pid, err)
	}
}

type noopReceiver struct{}

func (noopReceiver) Start(_ context.Context, _ component.Host) error { return nil }
func (noopReceiver) Shutdown(_ context.Context) error                 { return nil }

type crashConfig struct{}

func (c *crashConfig) Validate() error { return nil }

func NewFactory(cr *CrashReporter) receiver.Factory {
	return xreceiver.NewFactory(
		component.MustNewType("crash"),
		func() component.Config { return &crashConfig{} },
		xreceiver.WithProfiles(
			func(_ context.Context, _ receiver.Settings, _ component.Config, nextConsumer xconsumer.Profiles) (xreceiver.Profiles, error) {
				cr.setConsumer(nextConsumer)
				return noopReceiver{}, nil
			},
			component.StabilityLevelAlpha,
		),
	)
}
