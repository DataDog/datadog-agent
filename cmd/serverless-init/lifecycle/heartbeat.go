// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DefaultHeartbeatInterval is the period between heartbeat metrics. The
// interval is hardcoded for now; if the platform team needs to tune it,
// promote to a config value (see DD_SERVERLESS_MICROVM_* convention used by
// the forwarder port env var).
const DefaultHeartbeatInterval = 5 * time.Minute

const (
	activeInstancesMetricName   = "aws.lambda.enhanced.microvm.active_instances"
	tracedInvocationsMetricName = activeInstancesMetricName

	// UnknownTagValue is the placeholder used when a tag value has not yet
	// been observed (e.g. MicroVM ID before /launch, or ARN fields that
	// cannot be parsed). Exported so cloudservice can reference the sentinel
	// without duplicating the string literal.
	UnknownTagValue = "unknown"
)

// Heartbeat periodically emits a heartbeat metric while the MicroVM is in
// the "running" phase (between /launch and /suspend or /terminate).
//
// This type is MicroVM-specific: it is constructed only by main.go's
// newLifecycleServerIfMicroVM, which itself short-circuits to nil for
// non-MicroVM cloud services. Do not wire Heartbeat into other code paths.
//
// Start and Stop are idempotent and safe to call from concurrent goroutines.
// Stop waits up to heartbeatStopTimeout for the goroutine to exit. If it
// times out (goroutine blocked in AddEnhancedMetric), h.cancel is left set
// so a subsequent Start remains a no-op until the goroutine self-cleans on
// drain — preventing overlapping emit goroutines across suspend/resume cycles.
type Heartbeat struct {
	interval      time.Duration
	metricEmitter MetricEmitter
	metricSource  metrics.MetricSource
	baseTags      []string // immutable after construction (e.g., microvm_image_arn)

	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	microVMID string // mutable via SetMicroVMID; "unknown" until set
}

// NewHeartbeat constructs a Heartbeat. Non-positive interval falls back to
// DefaultHeartbeatInterval. baseTags are tags known at construction time
// (typically derived from env vars, e.g., microvm_image_arn); the
// microvm_id tag is appended at emit time and is set at runtime via
// SetMicroVMID from the /launch request.
func NewHeartbeat(interval time.Duration, emitter MetricEmitter, source metrics.MetricSource, baseTags []string) *Heartbeat {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	return &Heartbeat{
		interval:      interval,
		metricEmitter: emitter,
		metricSource:  source,
		baseTags:      slices.Clone(baseTags),
		microVMID:     UnknownTagValue,
	}
}

// BaseTags returns the immutable tags set at construction (e.g. microvm_image_arn).
// Safe to call on a nil receiver. Exposed for white-box tests in external packages.
func (h *Heartbeat) BaseTags() []string {
	if h == nil {
		return nil
	}
	return slices.Clone(h.baseTags)
}

// SetMicroVMID records the MicroVM instance ID extracted from the /launch
// request header. Empty input is ignored so the existing value (default
// "unknown" or a previously-set ID) is preserved. Safe to call concurrently
// with the emitting goroutine; the change is visible on the next tick.
func (h *Heartbeat) SetMicroVMID(id string) {
	if h == nil || id == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.microVMID = id
}

// Start launches the heartbeat goroutine. No-op if already running or if the
// receiver is nil, so callers can invoke unconditionally.
func (h *Heartbeat) Start() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.done = make(chan struct{})
	go h.run(ctx, h.done)
	log.Debugf("MicroVM lifecycle: heartbeat started (interval=%s)", h.interval)
}

// heartbeatStopTimeout bounds how long Stop waits for the emit goroutine to
// exit. AddEnhancedMetric can block when the metric-agent samples channel is
// full; without a deadline, /suspend and /terminate would hang past
// flushTimeout waiting for the heartbeat to drain.
const heartbeatStopTimeout = 1 * time.Second

// Stop cancels the heartbeat goroutine and waits up to heartbeatStopTimeout
// for it to exit. No-op if not running or if the receiver is nil.
// If the goroutine is still blocked after the timeout, Stop returns without
// clearing h.cancel so that a subsequent Start remains a no-op until the
// goroutine self-cleans (preventing overlapping goroutines).
func (h *Heartbeat) Stop() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.cancel == nil {
		h.mu.Unlock()
		return
	}
	cancel := h.cancel
	done := h.done
	h.mu.Unlock()

	cancel()
	select {
	case <-done:
		log.Debug("MicroVM lifecycle: heartbeat stopped")
	case <-time.After(heartbeatStopTimeout):
		log.Warn("MicroVM lifecycle: heartbeat goroutine did not stop within deadline; continuing shutdown")
	}
}

func (h *Heartbeat) run(ctx context.Context, done chan struct{}) {
	defer func() {
		// Clear h.cancel and h.done under the lock before signalling done, so
		// that once Stop() returns (or once done is closed), Start() sees nil
		// and can safely spawn a new goroutine. The identity check on h.done
		// guards against a hypothetical second goroutine clearing a newer done.
		h.mu.Lock()
		if h.done == done {
			h.cancel = nil
			h.done = nil
		}
		h.mu.Unlock()
		close(done)
	}()
	// Emit once immediately so a metric is recorded even if the MicroVM
	// instance is terminated before the first ticker interval elapses.
	h.emit()
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.emit()
		}
	}
}

func (h *Heartbeat) emit() {
	timestamp := float64(time.Now().UnixNano()) / float64(time.Second)
	h.metricEmitter.AddEnhancedMetric(activeInstancesMetricName, 1.0, h.metricSource, timestamp, h.tagsForEmit()...)
}

// tagsForEmit returns a fresh tag slice combining the immutable base tags
// (set at construction) with the current microvm_id (set at /launch).
func (h *Heartbeat) tagsForEmit() []string {
	h.mu.Lock()
	id := h.microVMID
	h.mu.Unlock()
	tags := make([]string, 0, len(h.baseTags)+1)
	tags = append(tags, h.baseTags...)
	tags = append(tags, "microvm_id:"+id)
	return tags
}
