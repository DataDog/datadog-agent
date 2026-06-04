// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"context"
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
	activeInstancesMetricName = "aws.lambda.enhanced.microvm.active_instances"

	// unknownTagValue is the placeholder used when the MicroVM ID has not
	// yet been observed (when the platform omits the header).
	unknownTagValue = "unknown"
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
	// resourceName is the MicroVM image ARN. When non-empty it is emitted as
	// microvm_image_arn on the heartbeat metric. Empty disables the tag.
	resourceName string

	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	microVMID string // mutable via SetMicroVMID; "unknown" until set
}

// NewHeartbeat constructs a Heartbeat. Non-positive interval falls back to
// DefaultHeartbeatInterval. resourceName is the MicroVM image ARN; it is used
// as the microvm_image_arn tag on heartbeat emissions. Pass "" to omit the tag.
func NewHeartbeat(interval time.Duration, emitter MetricEmitter, source metrics.MetricSource, resourceName string) *Heartbeat {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	return &Heartbeat{
		interval:      interval,
		metricEmitter: emitter,
		metricSource:  source,
		resourceName:  resourceName,
		microVMID:     unknownTagValue,
	}
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
	// Emit once immediately so metrics are recorded even if the MicroVM
	// instance is terminated before the first ticker interval elapses.
	h.emitAll()
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.emitAll()
		}
	}
}

func (h *Heartbeat) emitAll() {
	h.mu.Lock()
	id := h.microVMID
	h.mu.Unlock()

	emitMetric(h.metricEmitter, h.metricSource, activeInstancesMetricName, h.buildHeartbeatTags(id)...)
}

func (h *Heartbeat) buildHeartbeatTags(id string) []string {
	tags := make([]string, 0, 2)
	if h.resourceName != "" {
		tags = append(tags, "microvm_image_arn:"+h.resourceName)
	}
	return append(tags, "microvm_id:"+id)
}

// tagsForEmit returns the current heartbeat tag slice. Used by tests to
// inspect tag state without triggering a metric emission.
func (h *Heartbeat) tagsForEmit() []string {
	h.mu.Lock()
	id := h.microVMID
	h.mu.Unlock()
	return h.buildHeartbeatTags(id)
}
