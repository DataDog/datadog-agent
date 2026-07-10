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
	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

const (
	// DefaultEgressInterval is how often continuous egress wakes up to check for
	// retained ranges that have become eligible.
	DefaultEgressInterval = 10 * time.Second
)

var (
	tlmEgressMode          = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "egress_mode", []string{"mode"}, "Current metric lookback egress mode")
	tlmEgressTransitions   = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "egress_transitions", []string{"from", "to", "reason"}, "Count of metric lookback egress mode transitions")
	tlmEgressRanges        = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "egress_ranges", []string{"state"}, "Count of metric lookback egress ranges")
	tlmEgressRuns          = telemetryimpl.GetCompatComponent().NewCounter("metric_lookback", "egress_runs", []string{"state"}, "Count of metric lookback egress forwarding attempts")
	tlmEgressPayloadSeries = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "egress_payload_series", []string{"payload"}, "Number of series or sketch series sent by the last metric lookback egress range")
	tlmEgressRangeSeconds  = telemetryimpl.GetCompatComponent().NewGauge("metric_lookback", "egress_range_seconds", nil, "Width of the last metric lookback egress range in seconds")
)

// MonitorStateTransition describes an observed change in the monitor's own
// health state. It is emitted independently of whether the monitor is allowed to
// affect egress mode.
type MonitorStateTransition struct {
	MetricName string
	From       monitor.State
	To         monitor.State
	Initial    bool
	Decision   monitor.Decision
	DryRun     bool
	EgressMode EgressMode
}

// MonitorStateTransitionLogger receives monitor state transition diagnostics.
type MonitorStateTransitionLogger func(MonitorStateTransition)

// EgressControllerOptions controls how retained ranges are forwarded. Most
// options are code-level defaults; public config currently selects monitor
// enablement, the watched metric/range epsilon, dry-run mode, and trigger/recovery
// egress context windows.
type EgressControllerOptions struct {
	// PreTriggerWindow extends forwarding before a monitor breach/unknown window.
	PreTriggerWindow time.Duration
	// PostRecoveryWindow extends forwarding after the first healthy window that
	// suppresses egress.
	PostRecoveryWindow time.Duration
	// SendDelay is the minimum age of a metric timestamp before it is forwarded.
	SendDelay time.Duration
	// EgressInterval is how often the background loop checks for eligible ranges.
	EgressInterval time.Duration
	// MonitorStaleTimeout controls when suppressed egress returns to forwarding if
	// no fresh monitor decision is observed. Zero disables stale reopening.
	MonitorStaleTimeout time.Duration
	// DryRun keeps egress forwarding from startup and prevents monitor decisions
	// from changing egress mode. Monitor state transitions are still logged.
	DryRun bool
	// MonitorStateTransitionLogger is called whenever the monitor's own state
	// changes, including the first observed state.
	MonitorStateTransitionLogger MonitorStateTransitionLogger

	// Now is injectable for deterministic tests. It defaults to time.Now.
	Now func() time.Time
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
	dryRun           bool
	logTransition    MonitorStateTransitionLogger

	runMu    sync.Mutex
	policyMu sync.Mutex

	hasMonitorState  bool
	lastMonitorState monitor.State

	asyncRunMu sync.Mutex
	asyncRunWG sync.WaitGroup

	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
	stopped   atomic.Bool
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
	policy := NewEgressPolicy(EgressPolicyOptions{
		PreTriggerWindow:    opts.PreTriggerWindow,
		PostRecoveryWindow:  opts.PostRecoveryWindow,
		SendDelay:           opts.SendDelay,
		MonitorStaleTimeout: opts.MonitorStaleTimeout,
		StartForwarding:     opts.DryRun,
	})
	return &EgressController{
		retention:        retention,
		metricSerializer: metricSerializer,
		policy:           policy,
		egressInterval:   opts.EgressInterval,
		now:              opts.Now,
		dryRun:           opts.DryRun,
		logTransition:    opts.MonitorStateTransitionLogger,
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
		if c.stopped.Load() {
			return
		}
		c.started.Store(true)
		go c.loop()
	})
}

// Stop asks the background egress loop to stop and waits for it to exit.
func (c *EgressController) Stop() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		c.asyncRunMu.Lock()
		c.stopped.Store(true)
		c.asyncRunMu.Unlock()
		if c.started.Load() {
			close(c.stopCh)
			<-c.doneCh
		}
		c.asyncRunWG.Wait()
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

		timer := time.NewTimer(c.egressInterval)
		select {
		case <-c.stopCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}

func (c *EgressController) runOnceAsync() {
	c.asyncRunMu.Lock()
	defer c.asyncRunMu.Unlock()
	if c.stopped.Load() {
		return
	}
	c.asyncRunWG.Add(1)
	go func() {
		defer c.asyncRunWG.Done()
		if c.stopped.Load() {
			return
		}
		c.RunOnce()
	}()
}

// OnDecision applies a monitor decision and asynchronously wakes egress so the
// DogStatsD append path does not block on serializer latency. In dry-run mode,
// the monitor decision is recorded for transition logging but does not affect
// egress policy, which remains forwarding.
func (c *EgressController) OnDecision(decision monitor.Decision) {
	if c == nil || c.policy == nil {
		return
	}
	var transition MonitorStateTransition
	var logTransition bool

	c.policyMu.Lock()
	before := c.policy.Mode()
	if !c.dryRun {
		c.policy.OnDecisionAt(decision, c.now())
	}
	after := c.policy.Mode()
	if !c.dryRun {
		c.recordModeTransition(before, after, decision.State.String())
	}
	transition, logTransition = c.recordMonitorStateLocked(decision, after)
	c.policyMu.Unlock()

	if logTransition && c.logTransition != nil {
		c.logTransition(transition)
	}
	c.runOnceAsync()
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
	seriesAvailable, hasSeries := c.availableSeriesRange()
	sketchAvailable, hasSketch := c.availableSketchRange()
	if !hasSeries && !hasSketch {
		tlmEgressRuns.Inc("empty")
		return
	}

	c.policyMu.Lock()
	before := c.policy.Mode()
	if c.policy.MarkStaleIfNeeded(now) {
		c.recordModeTransition(before, c.policy.Mode(), "stale")
	}
	mode := c.policy.Mode()
	seriesRanges := c.policy.SeriesRangesToForward(now)
	sketchRanges := c.policy.SketchRangesToForward(now)
	c.policyMu.Unlock()
	c.recordMode(mode)

	if len(seriesRanges) == 0 && len(sketchRanges) == 0 {
		tlmEgressRuns.Inc("empty")
		return
	}

	forwardedAny := false
	hadError := false
	if hasSeries {
		forwarded, failed := c.forwardRanges("series", seriesRanges, seriesAvailable, c.retention.ForwardSeriesRange, func(policy *EgressPolicy, r TimeRange) {
			policy.MarkSeriesForwarded(r)
		})
		forwardedAny = forwardedAny || forwarded
		hadError = hadError || failed
	}
	if hasSketch {
		forwarded, failed := c.forwardRanges("sketch", sketchRanges, sketchAvailable, c.retention.ForwardSketchRange, func(policy *EgressPolicy, r TimeRange) {
			policy.MarkSketchForwarded(r)
		})
		forwardedAny = forwardedAny || forwarded
		hadError = hadError || failed
	}
	if hadError {
		tlmEgressRuns.Inc("error")
		return
	}
	if !forwardedAny {
		tlmEgressRuns.Inc("empty")
	}
}

func (c *EgressController) forwardRanges(payload string, ranges []TimeRange, available TimeRange, forward func(serializer.MetricSerializer, time.Time, time.Time) (int, error), markForwarded func(*EgressPolicy, TimeRange)) (bool, bool) {
	forwardedAny := false
	for _, planned := range ranges {
		r, ok := intersectRanges(planned, available)
		if !ok {
			continue
		}
		tlmEgressRanges.Inc("planned")
		tlmEgressRangeSeconds.Set(r.To.Sub(r.From).Seconds())
		count, err := forward(c.metricSerializer, r.From, r.To)
		tlmEgressPayloadSeries.Set(float64(count), payload)
		if err != nil {
			tlmEgressRanges.Inc("retry")
			return forwardedAny, true
		}
		if count == 0 {
			// Do not mark empty ranges as forwarded. A later-arriving retained point with
			// an older timestamp should still be eligible if it falls in a forwarding
			// interval.
			tlmEgressRuns.Inc("empty")
			continue
		}
		c.policyMu.Lock()
		markForwarded(c.policy, r)
		c.policyMu.Unlock()
		tlmEgressRanges.Inc("forwarded")
		tlmEgressRuns.Inc("success")
		forwardedAny = true
	}
	return forwardedAny, false
}

func (c *EgressController) availableSeriesRange() (TimeRange, bool) {
	return availableRangeFromStats(c.retention.Stats())
}

func (c *EgressController) availableSketchRange() (TimeRange, bool) {
	return availableRangeFromStats(c.retention.SketchStats())
}

func availableRangeFromStats(stats ringbuffer.Stats) (TimeRange, bool) {
	if stats.Records == 0 || stats.OldestUnixMicro == 0 || stats.NewestUnixMicro == 0 {
		return TimeRange{}, false
	}
	return TimeRange{
		From: time.UnixMicro(stats.OldestUnixMicro),
		To:   time.UnixMicro(stats.NewestUnixMicro).Add(time.Microsecond),
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

func (c *EgressController) recordMonitorStateLocked(decision monitor.Decision, egressMode EgressMode) (MonitorStateTransition, bool) {
	transition := MonitorStateTransition{
		MetricName: decision.MetricName,
		From:       c.lastMonitorState,
		To:         decision.State,
		Initial:    !c.hasMonitorState,
		Decision:   decision,
		DryRun:     c.dryRun,
		EgressMode: egressMode,
	}
	if c.hasMonitorState && c.lastMonitorState == decision.State {
		return transition, false
	}
	c.lastMonitorState = decision.State
	c.hasMonitorState = true
	return transition, true
}

func intersectRanges(a, b TimeRange) (TimeRange, bool) {
	from := maxTime(a.From, b.From)
	to := minTime(a.To, b.To)
	r := TimeRange{From: from, To: to}
	return r, validHalfOpenRange(r)
}
