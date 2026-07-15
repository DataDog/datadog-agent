// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package vbrsender decorates a sender.SenderManager/sender.Sender to apply
// streaming, bounded-error compression (variable bit rate storage) to a
// check's Gauge/Count/Rate/MonotonicCount/GaugeWithTimestamp/
// CountWithTimestamp metrics, entirely on the sender side. This works for
// any check loader (Go, Python, ...), since every loader reaches the
// aggregator exclusively through this same interface.
package vbrsender

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/vbr"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// tlmSamples and tlmBreakpoints count, per check and metric name, how many
// raw samples reached the compressor and how many breakpoints it shipped.
// Their ratio is the compression ratio for that metric; computed at query
// time rather than stored directly, since a ratio gauge can't be usefully
// aggregated across time or hosts the way two counters can.
//
// tlmContexts tracks how many distinct contexts (metric+tags combinations)
// are being compressed for a check. Contexts never expire once created (see
// contextState), so this is the signal to watch for unbounded growth from a
// check whose tag set churns over time.
//
// tlmScaleDeviationSum/tlmScaleDeviationCount together track, per sample,
// |value - compressor.Scale()| — how far the raw (already check-kind-reduced)
// value strays from the compressor's current EWMA estimate of the signal's
// magnitude, the basis for its tolerance (see vbr.Compressor.Scale). Their
// ratio (sum/count) is the average deviation; a chronically large one
// signals the EWMA is mistracking the signal (e.g. during a sustained level
// shift), which is otherwise invisible from samples_total/breakpoints_total
// alone. Two plain counters instead of a Histogram: the built-in "telemetry"
// core check that exports DefaultMetric:true metrics to the backend
// (pkg/collector/corechecks/telemetry) only handles Prometheus GAUGE and
// COUNTER metric families — HISTOGRAM falls into its default case and is
// silently dropped (logged at debug level only), so a Histogram here would
// never actually reach Datadog.
//
// exportedMetric opts every vbrsender telemetry metric into the built-in
// "telemetry" core check (pkg/collector/corechecks/telemetry), the only
// path that turns an internal Prometheus counter into a real
// datadog.agent.<subsystem>.<name> metric in the backend. Without this,
// NewCounter/NewGauge/NewHistogram only populate the internal registry
// backing the /telemetry HTTP endpoint (debugging/flares), never shipped
// anywhere.
var exportedMetric = telemetry.Options{DefaultMetric: true}

var (
	tlmSamples = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"vbrsender", "samples_total",
		[]string{"check_name", "metric_name"},
		"Number of raw samples fed into the VBR compressor, by check and metric name",
		exportedMetric)
	tlmBreakpoints = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"vbrsender", "breakpoints_total",
		[]string{"check_name", "metric_name"},
		"Number of breakpoints shipped by the VBR compressor, by check and metric name",
		exportedMetric)
	tlmContexts = telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
		"vbrsender", "contexts",
		[]string{"check_name"},
		"Number of distinct metric contexts being VBR-compressed, by check name",
		exportedMetric)
	tlmScaleDeviationSum = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"vbrsender", "scale_deviation_sum",
		[]string{"check_name", "metric_name"},
		"Running sum of |value - EWMA scale| across all samples, by check and metric name — divide by vbrsender_scale_deviation_count for the average",
		exportedMetric)
	tlmScaleDeviationCount = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"vbrsender", "scale_deviation_count",
		[]string{"check_name", "metric_name"},
		"Number of samples observed for vbrsender_scale_deviation_sum, by check and metric name",
		exportedMetric)
)

// defaultConfig holds the global (not per-metric) VBR compressor
// parameters. Placeholder values pending real-world tuning.
var defaultConfig = vbr.Config{
	Epsilon: 0.02,
	Alpha:   0.3,
	Floor:   1e-3,
	Warmup:  2,
}

// windowDuration is how often a compressed context force-closes and ships a
// point even if nothing has changed, matching the aggregator's default
// flush interval. There's no access to the real flush tick from the sender
// side, so this is tracked independently by wall-clock time.
const windowDuration = 15 * time.Second

// timeNow is a seam for testing; production code always uses time.Now.
var timeNow = time.Now

// SenderManager wraps a sender.SenderManager so every Sender it returns
// applies VBR compression.
type SenderManager struct {
	inner  sender.SenderManager
	dryRun bool

	// shadowHostSuffix and defaultHostname together implement shadow mode;
	// see Wrap's doc comment. shadowHostSuffix is "" when shadow mode is
	// off (the common case) or when Wrap couldn't resolve defaultHostname.
	shadowHostSuffix string
	defaultHostname  string

	mu      sync.Mutex
	senders map[checkid.ID]*Sender
}

// Wrap returns a SenderManager that VBR-compresses every check it serves.
// With dryRun true, every sample still runs through the compressor (so the
// samples_total/breakpoints_total telemetry reflects what compression would
// do), but the check's original, uncompressed calls are what actually reach
// the real sender — nothing forwarded by the compressor itself ships.
//
// With shadowHostSuffix non-empty, the check's original, uncompressed calls
// ALSO still ship (like dry-run), but every breakpoint the compressor
// produces ships too, as an ADDITIONAL series under hostname+shadowHostSuffix
// (falling back to the agent's own resolved default hostname when the check
// passed ""). This lets the two series be graphed side by side (e.g.
// `avg:metric{*} by {host}`) to visually compare compression fidelity —
// at the cost of doubling shipped metric volume for compressed checks, so
// it's meant for short, deliberate comparison windows, not left on
// indefinitely. Shadow mode takes precedence over dryRun, since its entire
// purpose requires both series to exist (see Sender.ship). If the agent's
// default hostname can't be resolved, shadow mode is disabled entirely
// (falling back to dryRun's behavior) rather than risk appending the
// suffix to "" and collapsing every host's shadow series into one.
func Wrap(inner sender.SenderManager, dryRun bool, shadowHostSuffix string) *SenderManager {
	m := &SenderManager{inner: inner, dryRun: dryRun, senders: make(map[checkid.ID]*Sender)}
	if shadowHostSuffix != "" {
		h, err := hostname.Get(context.Background())
		if err != nil {
			log.Warnf("vbrsender: could not resolve the agent's default hostname, disabling shadow mode: %s", err)
		} else {
			m.shadowHostSuffix = shadowHostSuffix
			m.defaultHostname = h
		}
	}
	return m
}

// vbrCompressedCheckNames returns the set of check names that should get
// VBR-compressed metrics, from the checks.vbr_compression_checks config
// setting. Read fresh on every cache-miss GetSender call; a check whose
// sender is already cached keeps whatever decision was made at that time
// until it's rescheduled (see GetSender), since neither config key has
// hot-reload wiring today.
func vbrCompressedCheckNames() map[string]bool {
	names := setup.Datadog().GetStringSlice("checks.vbr_compression_checks")
	m := make(map[string]bool, len(names))
	for _, name := range names {
		m[name] = true
	}
	return m
}

// GetSender returns a Sender for id: VBR-compressed and cached if id's check
// name is in the vbr_compression_checks allowlist, otherwise passed straight
// through to the inner manager (which already caches per ID on its own, so
// no local caching is needed for that path). This is the single place that
// decides whether a check gets compressed, regardless of which loader
// (Go, Python, ...) requested the sender — unlike selecting between two
// different manager instances upstream, which Python's sender-resolution
// path (a package-level global captured once at loader-construction time,
// see pkg/collector/aggregator) would silently bypass.
func (m *SenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.senders[id]; ok {
		return s, nil
	}
	checkName := checkid.IDToCheckName(id)
	if !vbrCompressedCheckNames()[checkName] {
		return m.inner.GetSender(id)
	}
	real, err := m.inner.GetSender(id)
	if err != nil {
		return nil, err
	}
	s := newSender(real, m.dryRun, checkName, m.shadowHostSuffix, m.defaultHostname)
	m.senders[id] = s
	return s, nil
}

// SetSender installs a preconstructed Sender for id, bypassing compression
// (used by tests/advanced callers that want full control over the Sender).
func (m *SenderManager) SetSender(s sender.Sender, id checkid.ID) error {
	m.mu.Lock()
	delete(m.senders, id)
	m.mu.Unlock()
	return m.inner.SetSender(s, id)
}

// DestroySender forgets the wrapped Sender for id, if any, and destroys the
// underlying real one.
func (m *SenderManager) DestroySender(id checkid.ID) {
	m.mu.Lock()
	delete(m.senders, id)
	m.mu.Unlock()
	m.inner.DestroySender(id)
}

// GetDefaultSender returns the real default sender, uncompressed: the
// default sender isn't tied to a single check's config, so it isn't a
// candidate for compression.
func (m *SenderManager) GetDefaultSender() (sender.Sender, error) {
	return m.inner.GetDefaultSender()
}

type metricKind int

const (
	kindGauge metricKind = iota
	kindCount
	kindRate
	kindMonotonicCount
	// kindGaugeWithTimestamp/kindCountWithTimestamp mirror kindGauge/
	// kindCount for compression purposes (see reduce/ship's pendingSum
	// handling) but are kept distinct so forwardRaw knows to re-forward
	// via the *WithTimestamp sender methods, preserving the caller's own
	// timestamp instead of silently replacing it with nowSeconds() — see
	// GaugeWithTimestamp's doc comment.
	kindGaugeWithTimestamp
	kindCountWithTimestamp
)

// contextState holds one context's VBR compressor plus whatever extra
// state its kind needs to locally reduce raw sender calls into the single
// scalar-per-call value the compressor expects (Rate/MonotonicCount).
type contextState struct {
	metric   string
	hostname string
	tags     []string
	kind     metricKind

	compressor *vbr.Compressor

	tlmSamples             telemetry.SimpleCounter
	tlmBreakpoints         telemetry.SimpleCounter
	tlmScaleDeviationSum   telemetry.SimpleCounter
	tlmScaleDeviationCount telemetry.SimpleCounter

	// Rate: previous raw (value, timestamp), mirrors pkg/metrics/rate.go.
	hasPreviousRate   bool
	previousRateValue float64
	previousRateTs    float64

	// MonotonicCount: previous raw counter value, mirrors the diffing in
	// pkg/metrics/monotonic_count.go (reset detection only; the
	// sum-across-multiple-calls-per-commit behavior is handled by
	// pendingSum below instead, since the sender side has no "commit"
	// boundary to bound a sum by — every call is reduced to its own diff,
	// and pendingSum accumulates those diffs until something ships).
	hasPreviousMonotonicCount bool
	previousMonotonicCount    float64

	// pendingSum is meaningful only for kindCount/kindMonotonicCount: real
	// Count/MonotonicCount semantics sum every value received since the
	// last flush, not just report the latest one. pendingSum accumulates
	// every value reduce() produces, and ship() drains it (to 0) exactly
	// once whenever it ships a breakpoint for this context, guaranteeing
	// every received value is shipped exactly once in aggregate — even
	// though a single shipped point's timestamp may not exactly match
	// every value folded into it (bounded by windowDuration; the same
	// class of approximation real Count already makes by summing since
	// the last flush and stamping the sum with the flush time). Do not
	// "fix" this by trying to attribute pendingSum precisely to whichever
	// point is closing — the compressor's closed point has no memory of a
	// running sum to split, so there is nothing to attribute it to.
	pendingSum float64
}

// Sender wraps a real sender.Sender. Gauge/Count/Rate/MonotonicCount are
// intercepted and compressed; every other method passes straight through
// via embedding.
type Sender struct {
	sender.Sender

	checkName string

	// dryRun: samples still run through the compressor for measurement
	// (tlmSamples/tlmBreakpoints), but ship() never actually forwards a
	// breakpoint, and the check's original call is forwarded unmodified
	// instead (see compressAt/forwardRaw).
	dryRun bool

	// shadowHostSuffix/defaultHostname: see Wrap's shadow-mode doc comment.
	// shadowHostSuffix is "" when shadow mode is off.
	shadowHostSuffix string
	defaultHostname  string

	tlmContexts telemetry.SimpleGauge

	mu sync.Mutex
	// contexts and lastFlushTs all live in the same time domain: the
	// sample timestamps flowing through Update()/FlushWindow(), not an
	// independently-read wall clock. In production that domain happens to
	// be wall-clock-derived (see nowSeconds — Gauge/Count/Rate/
	// MonotonicCount don't carry a timestamp, so "when the call happened"
	// is the only signal available), but the window-flush decision itself
	// only ever compares sample timestamps to each other.
	contexts    map[string]*contextState
	lastFlushTs float64
}

func newSender(real sender.Sender, dryRun bool, checkName string, shadowHostSuffix string, defaultHostname string) *Sender {
	return &Sender{
		Sender:           real,
		checkName:        checkName,
		dryRun:           dryRun,
		shadowHostSuffix: shadowHostSuffix,
		defaultHostname:  defaultHostname,
		tlmContexts:      tlmContexts.WithValues(checkName),
		contexts:         make(map[string]*contextState),
	}
}

// shadowHostnameFor returns the hostname a shadow-mode compressed
// breakpoint should ship under: hostname with shadowHostSuffix appended,
// falling back to the agent's own resolved default hostname when hostname
// is empty. This mirrors what the real sender fills in downstream for the
// raw series (see checkSender.sendMetricSample's defaultHostname
// backfill), so appending the suffix directly to "" can't accidentally
// collapse every host's shadow series into one.
func (s *Sender) shadowHostnameFor(hostname string) string {
	if hostname == "" {
		hostname = s.defaultHostname
	}
	return hostname + s.shadowHostSuffix
}

// Gauge compresses metric instead of forwarding every call.
func (s *Sender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.compress(kindGauge, metric, value, hostname, tags)
}

// Count compresses metric instead of forwarding every call.
func (s *Sender) Count(metric string, value float64, hostname string, tags []string) {
	s.compress(kindCount, metric, value, hostname, tags)
}

// Rate compresses metric instead of forwarding every call.
func (s *Sender) Rate(metric string, value float64, hostname string, tags []string) {
	s.compress(kindRate, metric, value, hostname, tags)
}

// MonotonicCount compresses metric instead of forwarding every call.
func (s *Sender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.compressMonotonicCount(metric, value, hostname, tags, false)
}

// MonotonicCountWithFlushFirstValue compresses metric instead of forwarding
// every call. flushFirstValue is not sticky per-context state — real checks
// (e.g. the kubelet provider) pass a different value call to call for the
// same metric+tags — so it's threaded through per call, matching
// pkg/metrics/monotonic_count.go's own re-read-every-call behavior.
func (s *Sender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	s.compressMonotonicCount(metric, value, hostname, tags, flushFirstValue)
}

func (s *Sender) compressMonotonicCount(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	s.compressAt(kindMonotonicCount, metric, value, hostname, tags, nowSeconds(), flushFirstValue)
}

// GaugeWithTimestamp compresses metric instead of forwarding every call.
// Unlike Gauge, the caller supplies its own timestamp — some checks' samples
// carry a meaningful collection time that can lag real time slightly (e.g.
// GPU metrics collected via eBPF, see pkg/collector/corechecks/gpu). That
// timestamp is fed to the compressor as the sample's own Ts (not
// nowSeconds()) and threaded through to forwardRaw for dry-run/shadow mode,
// so it's never silently replaced by "whenever vbrsender happened to
// process this call".
func (s *Sender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.compressAt(kindGaugeWithTimestamp, metric, value, hostname, tags, timestamp, false)
	return nil
}

// CountWithTimestamp compresses metric instead of forwarding every call;
// see GaugeWithTimestamp for why the caller-provided timestamp matters.
func (s *Sender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.compressAt(kindCountWithTimestamp, metric, value, hostname, tags, timestamp, false)
	return nil
}

func contextKeyFor(metric, hostname string, tags []string) string {
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	return metric + "|" + hostname + "|" + strings.Join(sorted, ",")
}

func (s *Sender) compress(kind metricKind, metric string, rawValue float64, hostname string, tags []string) {
	s.compressAt(kind, metric, rawValue, hostname, tags, nowSeconds(), false)
}

// compressAt is compress with an explicit sample timestamp, so tests can
// drive the compressor/window-flush deterministically without depending on
// real elapsed wall-clock time between calls. flushFirstValue is only
// meaningful for kindMonotonicCount (see reduce()); other kinds ignore it.
func (s *Sender) compressAt(kind metricKind, metric string, rawValue float64, hostname string, tags []string, now float64, flushFirstValue bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dryRun || s.shadowHostSuffix != "" {
		// The real, unmodified call is what actually ships in dry-run mode;
		// in shadow mode it ships too (as the series to compare against),
		// while ship() below additionally ships the compressed breakpoint
		// under a different hostname. Either way, the compressor below only
		// measures (dry-run) or additionally produces (shadow) what
		// compression would do. now is only used for the *WithTimestamp
		// kinds, to preserve the caller's own timestamp when re-forwarding.
		s.forwardRaw(kind, metric, rawValue, hostname, tags, now, flushFirstValue)
	}

	key := contextKeyFor(metric, hostname, tags)
	ctx, ok := s.contexts[key]
	if !ok {
		tagsCopy := make([]string, len(tags))
		copy(tagsCopy, tags)
		ctx = &contextState{
			metric:                 metric,
			hostname:               hostname,
			tags:                   tagsCopy,
			kind:                   kind,
			compressor:             vbr.New(defaultConfig),
			tlmSamples:             tlmSamples.WithValues(s.checkName, metric),
			tlmBreakpoints:         tlmBreakpoints.WithValues(s.checkName, metric),
			tlmScaleDeviationSum:   tlmScaleDeviationSum.WithValues(s.checkName, metric),
			tlmScaleDeviationCount: tlmScaleDeviationCount.WithValues(s.checkName, metric),
		}
		s.contexts[key] = ctx
		s.tlmContexts.Inc()
	}

	value, ok := reduce(ctx, rawValue, now, flushFirstValue)
	if ok {
		ctx.tlmSamples.Inc()
		if ctx.kind == kindCount || ctx.kind == kindMonotonicCount || ctx.kind == kindCountWithTimestamp {
			// Accumulate before Update(), unconditionally: whether or not
			// this call causes a breakpoint, the value must count toward
			// whatever eventually ships next. See pendingSum's doc comment.
			ctx.pendingSum += value
		}
		bps := ctx.compressor.Update(now, value)
		// Read Scale() after Update(): the compressor folds this sample's
		// value into the EWMA as the first step of Update(), so this is the
		// freshest estimate, matching the tolerance Update() itself just
		// used to accept or reject this same sample.
		ctx.tlmScaleDeviationSum.Add(math.Abs(value - ctx.compressor.Scale()))
		ctx.tlmScaleDeviationCount.Inc()
		for _, bp := range bps {
			s.ship(ctx, bp)
		}
	}

	s.maybeFlushWindow(now)
}

// forwardRaw calls the same method the check originally called, on the
// real underlying sender, with the raw value as given — used only in
// dry-run/shadow mode, letting the real sender's own aggregation (e.g.
// Rate's own derivative) run exactly as if this decorator weren't present.
// ts is only used for the *WithTimestamp kinds, to preserve the caller's
// own timestamp instead of losing it by re-forwarding via the plain
// (no-timestamp) sender methods.
func (s *Sender) forwardRaw(kind metricKind, metric string, value float64, hostname string, tags []string, ts float64, flushFirstValue bool) {
	switch kind {
	case kindGauge:
		s.Sender.Gauge(metric, value, hostname, tags)
	case kindCount:
		s.Sender.Count(metric, value, hostname, tags)
	case kindRate:
		s.Sender.Rate(metric, value, hostname, tags)
	case kindMonotonicCount:
		// MonotonicCountWithFlushFirstValue(..., false) is behaviorally
		// identical to plain MonotonicCount (matches checkSender's own
		// implementation), so always use this one form regardless of
		// which method the check originally called.
		s.Sender.MonotonicCountWithFlushFirstValue(metric, value, hostname, tags, flushFirstValue)
	case kindGaugeWithTimestamp:
		if err := s.Sender.GaugeWithTimestamp(metric, value, hostname, tags, ts); err != nil {
			log.Debugf("vbrsender: GaugeWithTimestamp(%s) failed: %s", metric, err)
		}
	case kindCountWithTimestamp:
		if err := s.Sender.CountWithTimestamp(metric, value, hostname, tags, ts); err != nil {
			log.Debugf("vbrsender: CountWithTimestamp(%s) failed: %s", metric, err)
		}
	}
}

// reduce turns a raw sender call's value into the single scalar the
// compressor should see, replicating Rate's derivative and
// MonotonicCount's reset-aware diff locally (see pkg/metrics/rate.go and
// pkg/metrics/monotonic_count.go). ok is false when there isn't yet enough
// state to produce a value (first sample of a Rate/MonotonicCount series,
// or a detected counter reset) — matching how those types produce no serie
// on their first commit either. flushFirstValue is only consulted for
// kindMonotonicCount.
func reduce(ctx *contextState, rawValue, ts float64, flushFirstValue bool) (float64, bool) {
	switch ctx.kind {
	case kindGauge, kindCount, kindGaugeWithTimestamp, kindCountWithTimestamp:
		return rawValue, true

	case kindRate:
		if !ctx.hasPreviousRate {
			ctx.previousRateValue, ctx.previousRateTs, ctx.hasPreviousRate = rawValue, ts, true
			return 0, false
		}
		dt := ts - ctx.previousRateTs
		delta := rawValue - ctx.previousRateValue
		ctx.previousRateValue, ctx.previousRateTs = rawValue, ts
		if dt <= 0 {
			return 0, false
		}
		rate := delta / dt
		if rate < 0 {
			// underlying counter was reset; matches Rate.flush's own guard.
			return 0, false
		}
		return rate, true

	case kindMonotonicCount:
		if !ctx.hasPreviousMonotonicCount {
			ctx.previousMonotonicCount, ctx.hasPreviousMonotonicCount = rawValue, true
			if flushFirstValue {
				// The very first-ever sample ships as-is instead of
				// waiting for a second sample to diff against, matching
				// MonotonicCount.addSample's flushFirstValue handling
				// (assumption: the raw counter started at 0).
				return rawValue, true
			}
			return 0, false
		}
		diff := rawValue - ctx.previousMonotonicCount
		ctx.previousMonotonicCount = rawValue
		if diff < 0 {
			if flushFirstValue {
				// Not a drop: the raw counter is assumed to have reset to
				// 0, so the current value is the count since reset,
				// matching MonotonicCount.addSample's flushFirstValue
				// reset-baseline handling.
				return rawValue, true
			}
			// underlying raw counter was reset; drop, matching
			// MonotonicCount.addSample's own reset handling.
			return 0, false
		}
		return diff, true
	}
	return rawValue, true
}

func (s *Sender) ship(ctx *contextState, bp vbr.Point) {
	ctx.tlmBreakpoints.Inc()

	// For Count/MonotonicCount, ship the accumulated pendingSum instead of
	// bp.Value — see pendingSum's doc comment. This must happen
	// unconditionally, before the dryRun check below, so the compressor's
	// simulated state (and this bookkeeping) advances identically whether
	// or not anything actually gets forwarded.
	shipValue := bp.Value
	if ctx.kind == kindCount || ctx.kind == kindMonotonicCount || ctx.kind == kindCountWithTimestamp {
		shipValue = ctx.pendingSum
		ctx.pendingSum = 0
	}

	shipHostname := ctx.hostname
	switch {
	case s.shadowHostSuffix != "":
		// Shadow mode: forwardRaw already shipped the check's original call
		// under the real hostname (like dry-run); ship this breakpoint too,
		// as an ADDITIONAL series under a distinct hostname, so the two can
		// be graphed side by side (e.g. `by {host}`) to compare compression
		// fidelity. Takes precedence over dryRun (see Wrap's doc comment).
		shipHostname = s.shadowHostnameFor(ctx.hostname)
	case s.dryRun:
		// Telemetry still counts this as a would-be breakpoint; only the
		// actual forwarding is suppressed, since forwardRaw already shipped
		// the check's original, uncompressed call for this sample.
		return
	}
	switch ctx.kind {
	case kindGauge, kindRate, kindGaugeWithTimestamp:
		if err := s.Sender.GaugeWithTimestamp(ctx.metric, shipValue, shipHostname, ctx.tags, bp.Ts); err != nil {
			log.Debugf("vbrsender: GaugeWithTimestamp(%s) failed: %s", ctx.metric, err)
		}
	case kindCount, kindMonotonicCount, kindCountWithTimestamp:
		if err := s.Sender.CountWithTimestamp(ctx.metric, shipValue, shipHostname, ctx.tags, bp.Ts); err != nil {
			log.Debugf("vbrsender: CountWithTimestamp(%s) failed: %s", ctx.metric, err)
		}
	}
}

// maybeFlushWindow force-closes every open compressor segment once
// windowDuration (in sample-timestamp terms, not wall-clock terms) has
// elapsed since the last flush, so a compressed metric keeps shipping a
// point every window even when its signal is flat. Must be called with
// s.mu held.
func (s *Sender) maybeFlushWindow(now float64) {
	if now-s.lastFlushTs < windowDuration.Seconds() {
		return
	}
	s.lastFlushTs = now
	for _, ctx := range s.contexts {
		for _, bp := range ctx.compressor.FlushWindow(now) {
			s.ship(ctx, bp)
		}
	}
}

func nowSeconds() float64 {
	return float64(timeNow().UnixNano()) / float64(time.Second)
}
