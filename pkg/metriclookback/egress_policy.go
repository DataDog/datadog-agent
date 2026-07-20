// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
)

const (
	// DefaultEgressSendDelay waits until retained metric timestamps are old enough
	// that the regular coarser metric pipeline should have had time to send first.
	DefaultEgressSendDelay = 30 * time.Second
	// DefaultMonitorStaleTimeout keeps stale-monitor reopening disabled by default.
	// Empty periods without the trigger metric should not enable egress.
	DefaultMonitorStaleTimeout = 0
)

// TimeRange is a half-open timestamp range: [From, To). A zero From is an
// unbounded lower edge. Policy methods only return ranges with a non-zero To.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// EgressMode is whether retained lookback ranges should currently be forwarded.
type EgressMode int

const (
	// EgressForwarding means selected retained ranges should be forwarded once they
	// are eligible under the send-delay policy.
	EgressForwarding EgressMode = iota
	// EgressSuppressed means retention continues but ranges are not forwarded until
	// the monitor breaches, returns unknown, or becomes stale when stale reopening
	// is explicitly configured.
	EgressSuppressed
)

// String returns a stable label for telemetry and diagnostics.
func (m EgressMode) String() string {
	switch m {
	case EgressSuppressed:
		return "suppressed"
	default:
		return "forwarding"
	}
}

// EgressPolicyOptions controls the pure range-planning policy.
type EgressPolicyOptions struct {
	// PreTriggerWindow extends a forwarding range before a monitor breach or
	// unknown window.
	PreTriggerWindow time.Duration
	// PostRecoveryWindow extends a forwarding range after the first healthy window
	// that suppresses egress.
	PostRecoveryWindow time.Duration
	// SendDelay is the minimum age of a metric timestamp before it is forwarded.
	SendDelay time.Duration
	// MonitorStaleTimeout controls when suppressed egress returns to forwarding if
	// no fresh monitor decision is observed. Zero disables stale reopening.
	MonitorStaleTimeout time.Duration
	// StartForwarding makes the policy start in forwarding mode instead of the
	// default suppressed mode. This is used for monitor dry-runs where monitor
	// decisions are observed and logged but must not gate intake egress.
	StartForwarding bool
}

// EgressPolicy converts monitor decisions and wall-clock eligibility into
// half-open retained timestamp ranges that should be forwarded. It owns no
// retention or serializer resources so it can be tested exhaustively.
type EgressPolicy struct {
	preTriggerWindow    time.Duration
	postRecoveryWindow  time.Duration
	sendDelay           time.Duration
	monitorStaleTimeout time.Duration

	mode EgressMode

	forwardingRanges      []TimeRange
	forwardedSeriesRanges []TimeRange
	forwardedSketchRanges []TimeRange

	lastMonitorAt  time.Time
	lastDecisionAt time.Time
}

// NewEgressPolicy creates an egress policy. By default it starts suppressed:
// retention is active immediately, but forwarding only opens after the watched
// metric breaches or produces an unknown monitor decision. StartForwarding
// overrides this initial mode for monitor dry-runs.
func NewEgressPolicy(opts EgressPolicyOptions) *EgressPolicy {
	if opts.SendDelay < 0 {
		opts.SendDelay = 0
	}
	if opts.SendDelay == 0 {
		opts.SendDelay = DefaultEgressSendDelay
	}
	if opts.MonitorStaleTimeout < 0 {
		opts.MonitorStaleTimeout = 0
	}
	policy := &EgressPolicy{
		preTriggerWindow:    nonNegativeDuration(opts.PreTriggerWindow),
		postRecoveryWindow:  nonNegativeDuration(opts.PostRecoveryWindow),
		sendDelay:           opts.SendDelay,
		monitorStaleTimeout: opts.MonitorStaleTimeout,
		mode:                EgressSuppressed,
	}
	if opts.StartForwarding {
		policy.openForwardingAt(time.Time{})
	}
	return policy
}

func nonNegativeDuration(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}

// Mode returns the current egress mode.
func (p *EgressPolicy) Mode() EgressMode {
	if p == nil {
		return EgressSuppressed
	}
	return p.mode
}

// OnDecision applies a monitor decision to the egress mode and forwarding
// ranges. The decision window timestamp is also used as the decision freshness
// time; callers with a wall-clock receipt time should use OnDecisionAt.
func (p *EgressPolicy) OnDecision(decision monitor.Decision) {
	p.OnDecisionAt(decision, decision.WindowTo)
}

// OnDecisionAt applies a monitor decision to the egress mode and forwarding
// ranges, recording observedAt as the wall-clock freshness time for stale-monitor
// fallback.
func (p *EgressPolicy) OnDecisionAt(decision monitor.Decision, observedAt time.Time) {
	if p == nil {
		return
	}
	if !decision.WindowTo.IsZero() {
		p.lastMonitorAt = decision.WindowTo
	}
	if !observedAt.IsZero() {
		p.lastDecisionAt = observedAt
	}

	switch decision.State {
	case monitor.Healthy:
		p.onHealthy(decision)
	case monitor.Breach:
		p.onBreach(decision)
	default:
		p.onUnknown(decision)
	}
}

func (p *EgressPolicy) onHealthy(decision monitor.Decision) {
	if p.mode == EgressSuppressed {
		return
	}
	closeAt := decision.WindowTo.Add(p.postRecoveryWindow)
	p.closeForwardingAt(closeAt)
	p.mode = EgressSuppressed
}

func (p *EgressPolicy) onBreach(decision monitor.Decision) {
	if p.mode == EgressForwarding {
		return
	}
	p.openForwardingAt(decision.WindowFrom.Add(-p.preTriggerWindow))
}

func (p *EgressPolicy) onUnknown(decision monitor.Decision) {
	if p.mode == EgressForwarding {
		return
	}
	p.openForwardingAt(decision.WindowFrom.Add(-p.preTriggerWindow))
}

// MarkStaleIfNeeded returns suppressed egress to forwarding when no fresh
// monitor decision has arrived within the stale timeout.
func (p *EgressPolicy) MarkStaleIfNeeded(now time.Time) bool {
	if p == nil || p.mode == EgressForwarding || p.monitorStaleTimeout <= 0 || now.IsZero() || p.lastDecisionAt.IsZero() || p.lastMonitorAt.IsZero() {
		return false
	}
	if now.Sub(p.lastDecisionAt) <= p.monitorStaleTimeout {
		return false
	}
	p.openForwardingAt(p.lastMonitorAt)
	return true
}

// RangesToForward returns half-open series ranges that are in a forwarding
// interval, old enough under send delay, and not already marked as forwarded.
func (p *EgressPolicy) RangesToForward(now time.Time) []TimeRange {
	return p.SeriesRangesToForward(now)
}

// SeriesRangesToForward returns half-open series ranges that are in a forwarding
// interval, old enough under send delay, and not already marked as forwarded.
func (p *EgressPolicy) SeriesRangesToForward(now time.Time) []TimeRange {
	if p == nil {
		return nil
	}
	return p.rangesToForward(now, p.forwardedSeriesRanges)
}

// SketchRangesToForward returns half-open sketch ranges that are in a forwarding
// interval, old enough under send delay, and not already marked as forwarded.
func (p *EgressPolicy) SketchRangesToForward(now time.Time) []TimeRange {
	if p == nil {
		return nil
	}
	return p.rangesToForward(now, p.forwardedSketchRanges)
}

func (p *EgressPolicy) rangesToForward(now time.Time, forwardedRanges []TimeRange) []TimeRange {
	if now.IsZero() {
		return nil
	}
	eligibleThrough := now.Add(-p.sendDelay)
	if eligibleThrough.IsZero() || eligibleThrough.After(now) {
		return nil
	}

	var candidates []TimeRange
	for _, r := range p.forwardingRanges {
		candidate := r
		if candidate.To.IsZero() || eligibleThrough.Before(candidate.To) {
			candidate.To = eligibleThrough
		}
		if validHalfOpenRange(candidate) {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	return subtractRanges(candidates, forwardedRanges)
}

// MarkForwarded records that a half-open range was successfully forwarded for
// both series and sketch payloads.
func (p *EgressPolicy) MarkForwarded(r TimeRange) {
	p.MarkSeriesForwarded(r)
	p.MarkSketchForwarded(r)
}

// MarkSeriesForwarded records that a half-open range was successfully forwarded
// for series payloads.
func (p *EgressPolicy) MarkSeriesForwarded(r TimeRange) {
	if p == nil || !validHalfOpenRange(r) {
		return
	}
	p.forwardedSeriesRanges = appendAndMergeRanges(p.forwardedSeriesRanges, r)
}

// MarkSketchForwarded records that a half-open range was successfully forwarded
// for sketch payloads.
func (p *EgressPolicy) MarkSketchForwarded(r TimeRange) {
	if p == nil || !validHalfOpenRange(r) {
		return
	}
	p.forwardedSketchRanges = appendAndMergeRanges(p.forwardedSketchRanges, r)
}

// ForwardingRanges returns a copy of the policy's forwarding ranges for tests
// and diagnostics.
func (p *EgressPolicy) ForwardingRanges() []TimeRange {
	if p == nil {
		return nil
	}
	return append([]TimeRange(nil), p.forwardingRanges...)
}

// ForwardedRanges returns a copy of successfully forwarded series ranges for
// tests and diagnostics.
func (p *EgressPolicy) ForwardedRanges() []TimeRange {
	return p.ForwardedSeriesRanges()
}

// ForwardedSeriesRanges returns a copy of successfully forwarded series ranges
// for tests and diagnostics.
func (p *EgressPolicy) ForwardedSeriesRanges() []TimeRange {
	if p == nil {
		return nil
	}
	return append([]TimeRange(nil), p.forwardedSeriesRanges...)
}

// ForwardedSketchRanges returns a copy of successfully forwarded sketch ranges
// for tests and diagnostics.
func (p *EgressPolicy) ForwardedSketchRanges() []TimeRange {
	if p == nil {
		return nil
	}
	return append([]TimeRange(nil), p.forwardedSketchRanges...)
}

func (p *EgressPolicy) openForwardingAt(from time.Time) {
	p.mode = EgressForwarding
	p.forwardingRanges = appendAndMergeRanges(p.forwardingRanges, TimeRange{From: from})
}

func (p *EgressPolicy) closeForwardingAt(to time.Time) {
	if to.IsZero() {
		return
	}
	for i := range p.forwardingRanges {
		if !p.forwardingRanges[i].To.IsZero() {
			continue
		}
		if !p.forwardingRanges[i].From.IsZero() && !p.forwardingRanges[i].From.Before(to) {
			p.forwardingRanges = append(p.forwardingRanges[:i], p.forwardingRanges[i+1:]...)
			return
		}
		p.forwardingRanges[i].To = to
		return
	}
}

func appendAndMergeRanges(ranges []TimeRange, next TimeRange) []TimeRange {
	// Open ranges are valid when they represent a forwarding interval. They are
	// not returned from RangesToForward until clipped by send-delay eligibility.
	if !next.To.IsZero() && !validHalfOpenRange(next) {
		return ranges
	}

	out := append([]TimeRange(nil), ranges...)
	out = append(out, next)
	sort.Slice(out, func(i, j int) bool {
		if out[i].From.Equal(out[j].From) {
			return rangeToAfter(out[i].To, out[j].To)
		}
		if out[i].From.IsZero() {
			return true
		}
		if out[j].From.IsZero() {
			return false
		}
		return out[i].From.Before(out[j].From)
	})

	merged := out[:0]
	for _, r := range out {
		if len(merged) == 0 {
			merged = append(merged, r)
			continue
		}
		last := &merged[len(merged)-1]
		if rangesOverlapOrTouch(*last, r) {
			last.To = maxRangeTo(last.To, r.To)
			continue
		}
		merged = append(merged, r)
	}
	return append([]TimeRange(nil), merged...)
}

func subtractRanges(candidates, forwarded []TimeRange) []TimeRange {
	if len(forwarded) == 0 {
		return append([]TimeRange(nil), candidates...)
	}
	forwarded = append([]TimeRange(nil), forwarded...)
	sort.Slice(forwarded, func(i, j int) bool {
		if forwarded[i].From.Equal(forwarded[j].From) {
			return forwarded[i].To.Before(forwarded[j].To)
		}
		if forwarded[i].From.IsZero() {
			return true
		}
		if forwarded[j].From.IsZero() {
			return false
		}
		return forwarded[i].From.Before(forwarded[j].From)
	})

	var out []TimeRange
	for _, candidate := range candidates {
		pieces := []TimeRange{candidate}
		for _, done := range forwarded {
			var nextPieces []TimeRange
			for _, piece := range pieces {
				nextPieces = append(nextPieces, subtractRange(piece, done)...)
			}
			pieces = nextPieces
			if len(pieces) == 0 {
				break
			}
		}
		out = append(out, pieces...)
	}
	return out
}

func subtractRange(r, done TimeRange) []TimeRange {
	if !validHalfOpenRange(r) || !validHalfOpenRange(done) || !rangesOverlap(r, done) {
		return []TimeRange{r}
	}
	var out []TimeRange
	if rangeFromBefore(r.From, done.From) {
		left := TimeRange{From: r.From, To: minTime(r.To, done.From)}
		if validHalfOpenRange(left) {
			out = append(out, left)
		}
	}
	if done.To.Before(r.To) {
		right := TimeRange{From: maxTime(r.From, done.To), To: r.To}
		if validHalfOpenRange(right) {
			out = append(out, right)
		}
	}
	return out
}

func validHalfOpenRange(r TimeRange) bool {
	return !r.To.IsZero() && (r.From.IsZero() || r.From.Before(r.To))
}

func rangesOverlap(a, b TimeRange) bool {
	if !validHalfOpenRange(a) || !validHalfOpenRange(b) {
		return false
	}
	return rangeFromBefore(a.From, b.To) && rangeFromBefore(b.From, a.To)
}

func rangesOverlapOrTouch(a, b TimeRange) bool {
	if a.To.IsZero() {
		return true
	}
	return !a.To.Before(b.From)
}

func rangeFromBefore(from, t time.Time) bool {
	return from.IsZero() || from.Before(t)
}

func rangeToAfter(a, b time.Time) bool {
	if a.IsZero() {
		return true
	}
	if b.IsZero() {
		return false
	}
	return a.After(b)
}

func maxRangeTo(a, b time.Time) time.Time {
	if a.IsZero() || b.IsZero() {
		return time.Time{}
	}
	if a.Before(b) {
		return b
	}
	return a
}

func minTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.After(b) {
		return a
	}
	return b
}
