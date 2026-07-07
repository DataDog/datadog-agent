// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"sync"
	"sync/atomic"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

const (
	// DefaultEgressInterval is how often continuous egress wakes up to check for
	// retained ranges that have become eligible.
	DefaultEgressInterval = 10 * time.Second
)

var (
	tlmEgressMode         = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "egress_mode", []string{"mode"}, "Current metric lookback egress mode")
	tlmEgressTransitions  = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "egress_transitions", []string{"from", "to", "reason"}, "Count of metric lookback egress mode transitions")
	tlmEgressRanges       = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "egress_ranges", []string{"state"}, "Count of metric lookback egress ranges")
	tlmEgressRuns         = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "egress_runs", []string{"state"}, "Count of metric lookback egress forwarding attempts")
	tlmEgressSeries       = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "egress_series", nil, "Number of series sent by the last metric lookback egress range")
	tlmEgressRangeSeconds = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "egress_range_seconds", nil, "Width of the last metric lookback egress range in seconds")
)

// EgressControllerOptions controls how retained ranges are forwarded. These
// options are intentionally code-level defaults for now; public config only
// enables the monitor and selects the monitor metric/range epsilon.
type EgressControllerOptions struct {
	// PreWindow extends forwarding before a monitor breach/unknown window.
	PreWindow time.Duration
	// PostWindow extends forwarding after the healthy window that suppresses
	// egress.
	PostWindow time.Duration
	// SendDelay is the minimum age of a metric timestamp before it is forwarded.
	SendDelay time.Duration
	// EgressInterval is how often the background loop checks for eligible ranges.
	EgressInterval time.Duration
	// HealthyWindowsToSuppress is the number of consecutive healthy monitor windows
	// required before egress transitions to suppressed.
	HealthyWindowsToSuppress int
	// MonitorStaleTimeout controls when suppressed egress returns to forwarding if
	// no fresh monitor decision is observed. Zero disables stale reopening.
	MonitorStaleTimeout time.Duration

	// Now and Sleep are injectable for deterministic tests. They default to
	// time.Now and time.Sleep.
	Now   func() time.Time
	Sleep func(time.Duration)
}

// EgressController applies monitor decisions to an EgressPolicy and forwards the
// policy-planned retained ranges through the serializer. It does not own monitor
// scoring or retention admission.
type EgressController struct {
	retention        *Retention
	metricSerializer serializer.MetricSerializer
	policy           *EgressPolicy
	egressInterval   time.Duration
	now              func() time.Time
	sleep            func(time.Duration)

	runMu    sync.Mutex
	policyMu sync.Mutex

	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewEgressController creates a monitor decision sink backed by the retention
// ring and serializer. It returns nil when forwarding is not possible.
func NewEgressController(retention *Retention, metricSerializer serializer.MetricSerializer, opts EgressControllerOptions) *EgressController {
	if retention == nil || metricSerializer == nil {
		return nil
	}
	if opts.EgressInterval <= 0 {
		opts.EgressInterval = DefaultEgressInterval
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Sleep == nil {
		opts.Sleep = time.Sleep
	}

	policy := NewEgressPolicy(EgressPolicyOptions{
		PreWindow:                opts.PreWindow,
		PostWindow:               opts.PostWindow,
		SendDelay:                opts.SendDelay,
		HealthyWindowsToSuppress: opts.HealthyWindowsToSuppress,
		MonitorStaleTimeout:      opts.MonitorStaleTimeout,
	})
	return &EgressController{
		retention:        retention,
		metricSerializer: metricSerializer,
		policy:           policy,
		egressInterval:   opts.EgressInterval,
		now:              opts.Now,
		sleep:            opts.Sleep,
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
	}
}

// Start begins the background egress loop. It is safe to call more than once.
func (c *EgressController) Start() {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		c.started.Store(true)
		go c.loop()
	})
}

// Stop asks the background egress loop to stop. The loop may finish its current
// sleep before observing the stop request because Sleep is injectable and may not
// be interruptible.
func (c *EgressController) Stop() {
	if c == nil {
		return
	}
	if !c.started.Load() {
		return
	}
	c.stopOnce.Do(func() {
		close(c.stopCh)
		<-c.doneCh
	})
}

func (c *EgressController) loop() {
	defer close(c.doneCh)
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}
		c.RunOnce()
		c.sleep(c.egressInterval)
	}
}

// OnDecision applies a monitor decision and asynchronously wakes egress so the
// DogStatsD append path does not block on serializer latency.
func (c *EgressController) OnDecision(decision monitor.Decision) {
	if c == nil || c.policy == nil {
		return
	}
	c.policyMu.Lock()
	before := c.policy.Mode()
	c.policy.OnDecisionAt(decision, c.now())
	c.recordModeTransition(before, c.policy.Mode(), decision.State.String())
	c.policyMu.Unlock()
	go c.RunOnce()
}

// Mode returns the current egress mode.
func (c *EgressController) Mode() EgressMode {
	if c == nil || c.policy == nil {
		return EgressSuppressed
	}
	c.policyMu.Lock()
	defer c.policyMu.Unlock()
	return c.policy.Mode()
}

// RunOnce forwards every policy-planned range currently eligible. It is exposed
// for deterministic tests; the background loop calls it periodically.
func (c *EgressController) RunOnce() {
	if c == nil || c.policy == nil {
		return
	}
	c.runMu.Lock()
	defer c.runMu.Unlock()

	now := c.now()
	available, ok := c.availableRange()
	if !ok {
		tlmEgressRuns.Inc("empty")
		return
	}

	c.policyMu.Lock()
	before := c.policy.Mode()
	if c.policy.MarkStaleIfNeeded(now) {
		c.recordModeTransition(before, c.policy.Mode(), "stale")
	}
	mode := c.policy.Mode()
	ranges := c.policy.RangesToForward(now)
	c.policyMu.Unlock()
	c.recordMode(mode)

	if len(ranges) == 0 {
		tlmEgressRuns.Inc("empty")
		return
	}

	forwardedAny := false
	for _, planned := range ranges {
		r, ok := intersectRanges(planned, available)
		if !ok {
			continue
		}
		tlmEgressRanges.Inc("planned")
		tlmEgressRangeSeconds.Set(r.To.Sub(r.From).Seconds())
		count, err := c.retention.ForwardRange(c.metricSerializer, r.From, r.To)
		tlmEgressSeries.Set(float64(count))
		if err != nil {
			tlmEgressRanges.Inc("retry")
			tlmEgressRuns.Inc("error")
			return
		}
		if count == 0 {
			// Do not mark empty ranges as forwarded. A later-arriving retained point with
			// an older timestamp should still be eligible if it falls in a forwarding
			// interval.
			tlmEgressRuns.Inc("empty")
			continue
		}
		c.policyMu.Lock()
		c.policy.MarkForwarded(r)
		c.policyMu.Unlock()
		tlmEgressRanges.Inc("forwarded")
		tlmEgressRuns.Inc("success")
		c.retention.Stats()
		forwardedAny = true
	}
	if !forwardedAny {
		tlmEgressRuns.Inc("empty")
	}
}

func (c *EgressController) availableRange() (TimeRange, bool) {
	stats := c.retention.Stats()
	if stats.Records == 0 && c.retention.SketchStats().Records == 0 {
		return TimeRange{}, false
	}

	oldestUnixMicro := stats.OldestUnixMicro
	newestUnixMicro := stats.NewestUnixMicro
	if sketchStats := c.retention.SketchStats(); sketchStats.Records > 0 {
		if oldestUnixMicro == 0 || sketchStats.OldestUnixMicro < oldestUnixMicro {
			oldestUnixMicro = sketchStats.OldestUnixMicro
		}
		if sketchStats.NewestUnixMicro > newestUnixMicro {
			newestUnixMicro = sketchStats.NewestUnixMicro
		}
	}
	if oldestUnixMicro == 0 || newestUnixMicro == 0 {
		return TimeRange{}, false
	}
	return TimeRange{
		From: time.UnixMicro(oldestUnixMicro),
		To:   time.UnixMicro(newestUnixMicro).Add(time.Microsecond),
	}, true
}

func (c *EgressController) recordMode(mode EgressMode) {
	tlmEgressMode.Set(0, "forwarding")
	tlmEgressMode.Set(0, "suppressed")
	tlmEgressMode.Set(1, mode.String())
}

func (c *EgressController) recordModeTransition(from, to EgressMode, reason string) {
	if from == to {
		return
	}
	tlmEgressTransitions.Inc(from.String(), to.String(), reason)
}

func intersectRanges(a, b TimeRange) (TimeRange, bool) {
	from := maxTime(a.From, b.From)
	to := minTime(a.To, b.To)
	r := TimeRange{From: from, To: to}
	return r, validHalfOpenRange(r)
}
