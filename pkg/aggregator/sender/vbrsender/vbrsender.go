// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package vbrsender decorates a sender.SenderManager/sender.Sender to apply
// streaming, bounded-error compression (variable bit rate storage) to a
// check's Gauge/Count/Rate/MonotonicCount metrics, entirely on the sender
// side. This works for any check loader (Go, Python, ...), since every
// loader reaches the aggregator exclusively through this same interface.
package vbrsender

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/vbr"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	inner sender.SenderManager

	mu      sync.Mutex
	senders map[checkid.ID]*Sender
}

// Wrap returns a SenderManager that VBR-compresses every check it serves.
func Wrap(inner sender.SenderManager) *SenderManager {
	return &SenderManager{inner: inner, senders: make(map[checkid.ID]*Sender)}
}

// GetSender returns the VBR-wrapped Sender for id, creating and caching one
// on first use.
func (m *SenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.senders[id]; ok {
		return s, nil
	}
	real, err := m.inner.GetSender(id)
	if err != nil {
		return nil, err
	}
	s := newSender(real)
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

	// Rate: previous raw (value, timestamp), mirrors pkg/metrics/rate.go.
	hasPreviousRate   bool
	previousRateValue float64
	previousRateTs    float64

	// MonotonicCount: previous raw counter value, mirrors the diffing in
	// pkg/metrics/monotonic_count.go (reset detection only; this doesn't
	// replicate FlushFirstValue or the sum-across-multiple-calls-per-commit
	// behavior, since the sender side has no "commit" boundary to bound
	// that sum by — every call is reduced to its own diff instead).
	hasPreviousMonotonicCount bool
	previousMonotonicCount    float64
}

// Sender wraps a real sender.Sender. Gauge/Count/Rate/MonotonicCount are
// intercepted and compressed; every other method passes straight through
// via embedding.
type Sender struct {
	sender.Sender

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

func newSender(real sender.Sender) *Sender {
	return &Sender{
		Sender:   real,
		contexts: make(map[string]*contextState),
	}
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
	s.compress(kindMonotonicCount, metric, value, hostname, tags)
}

func contextKeyFor(metric, hostname string, tags []string) string {
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	return metric + "|" + hostname + "|" + strings.Join(sorted, ",")
}

func (s *Sender) compress(kind metricKind, metric string, rawValue float64, hostname string, tags []string) {
	s.compressAt(kind, metric, rawValue, hostname, tags, nowSeconds())
}

// compressAt is compress with an explicit sample timestamp, so tests can
// drive the compressor/window-flush deterministically without depending on
// real elapsed wall-clock time between calls.
func (s *Sender) compressAt(kind metricKind, metric string, rawValue float64, hostname string, tags []string, now float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := contextKeyFor(metric, hostname, tags)
	ctx, ok := s.contexts[key]
	if !ok {
		tagsCopy := make([]string, len(tags))
		copy(tagsCopy, tags)
		ctx = &contextState{
			metric:     metric,
			hostname:   hostname,
			tags:       tagsCopy,
			kind:       kind,
			compressor: vbr.New(defaultConfig),
		}
		s.contexts[key] = ctx
	}

	value, ok := reduce(ctx, rawValue, now)
	if ok {
		for _, bp := range ctx.compressor.Update(now, value) {
			s.ship(ctx, bp)
		}
	}

	s.maybeFlushWindow(now)
}

// reduce turns a raw sender call's value into the single scalar the
// compressor should see, replicating Rate's derivative and
// MonotonicCount's reset-aware diff locally (see pkg/metrics/rate.go and
// pkg/metrics/monotonic_count.go). ok is false when there isn't yet enough
// state to produce a value (first sample of a Rate/MonotonicCount series,
// or a detected counter reset) — matching how those types produce no serie
// on their first commit either.
func reduce(ctx *contextState, rawValue, ts float64) (float64, bool) {
	switch ctx.kind {
	case kindGauge, kindCount:
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
			return 0, false
		}
		diff := rawValue - ctx.previousMonotonicCount
		ctx.previousMonotonicCount = rawValue
		if diff < 0 {
			// underlying raw counter was reset; drop, matching
			// MonotonicCount.addSample's own reset handling.
			return 0, false
		}
		return diff, true
	}
	return rawValue, true
}

func (s *Sender) ship(ctx *contextState, bp vbr.Point) {
	switch ctx.kind {
	case kindGauge, kindRate:
		if err := s.Sender.GaugeWithTimestamp(ctx.metric, bp.Value, ctx.hostname, ctx.tags, bp.Ts); err != nil {
			log.Debugf("vbrsender: GaugeWithTimestamp(%s) failed: %s", ctx.metric, err)
		}
	case kindCount, kindMonotonicCount:
		if err := s.Sender.CountWithTimestamp(ctx.metric, bp.Value, ctx.hostname, ctx.tags, bp.Ts); err != nil {
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
