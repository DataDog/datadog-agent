// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package quota enforces log quotas delivered over Remote Config in the logs pipeline.
package quota

import (
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var defaultEnforcer = NewEnforcer()

// Default returns the process-wide enforcer consulted by the logs processor.
func Default() *Enforcer {
	return defaultEnforcer
}

type quotaStats struct {
	droppedEvents atomic.Int64
	droppedBytes  atomic.Int64
	lastDrop      atomic.Int64

	tlmEvents telemetry.SimpleCounter
	tlmBytes  telemetry.SimpleCounter
}

type configEntry struct {
	quotas []*quota
	err    error
}

// ruleSet holds the unexpired quotas and the earliest of their deadlines.
type ruleSet struct {
	quotas     []*quota
	nextExpiry time.Time
	local      *hostView // nil when no host tags are known
}

// hostView is what local logs must still carry for each quota once the host's tags are removed.
type hostView struct {
	hostname string
	// full is the index of the first quota the host tags satisfy alone, or len(quotas).
	full     int
	residual [][]string
}

func newRuleSet(quotas []*quota, now time.Time, hostname string, hostTags []string) *ruleSet {
	rs := &ruleSet{}
	for _, q := range quotas {
		if q.expired(now) {
			continue
		}
		rs.quotas = append(rs.quotas, q)
		if rs.nextExpiry.IsZero() || q.deadline.Before(rs.nextExpiry) {
			rs.nextExpiry = q.deadline
		}
	}
	if len(hostTags) > 0 {
		rs.local = newHostView(rs.quotas, hostname, hostTags)
	}
	return rs
}

// newHostView expects hostTags sorted and lowercase.
func newHostView(quotas []*quota, hostname string, hostTags []string) *hostView {
	n := 0
	for _, q := range quotas {
		n += len(q.tags)
	}
	flat := make([]string, 0, n)
	hv := &hostView{hostname: hostname, full: len(quotas), residual: make([][]string, len(quotas))}
	for i, q := range quotas {
		start := len(flat)
		for _, t := range q.tags {
			if _, found := slices.BinarySearch(hostTags, t); !found {
				flat = append(flat, t)
			}
		}
		hv.residual[i] = flat[start:len(flat):len(flat)]
		if len(hv.residual[i]) == 0 && hv.full == len(quotas) {
			hv.full = i
		}
	}
	return hv
}

// localView returns the host view if a log with this hostname comes from the agent's host, or nil.
func (rs *ruleSet) localView(hostname string) *hostView {
	if hv := rs.local; hv != nil && (hostname == "" || equalFold(hostname, hv.hostname)) {
		return hv
	}
	return nil
}

// Enforcer holds the active quotas. Check is safe to call concurrently with Update.
type Enforcer struct {
	mu       sync.Mutex
	configs  map[string]*configEntry
	stats    map[string]*quotaStats
	lastSync time.Time
	hostname string
	hostTags []string

	now func() time.Time
	// tickEvery is how often the clock advances while quotas are active, or 0 to advance it only in tick.
	tickEvery time.Duration
	ticking   bool

	active atomic.Pointer[ruleSet]
	clock  atomic.Int64
}

// NewEnforcer returns an enforcer with no active quotas.
func NewEnforcer() *Enforcer {
	return &Enforcer{
		configs:   map[string]*configEntry{},
		stats:     map[string]*quotaStats{},
		now:       time.Now,
		tickEvery: time.Second,
	}
}

// Update replaces all configs, keyed by path, and returns each path's parse error.
func (e *Enforcer) Update(configs map[string][]byte) map[string]error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.configs = make(map[string]*configEntry, len(configs))
	result := make(map[string]error, len(configs))
	for path, raw := range configs {
		quotas, err := parseSnapshot(raw)
		e.configs[path] = &configEntry{quotas: quotas, err: err}
		result[path] = err
	}
	e.lastSync = e.now()

	var quotas []*quota
	stats := map[string]*quotaStats{}
	for _, path := range slices.Sorted(maps.Keys(e.configs)) {
		for _, q := range e.configs[path].quotas {
			if stats[q.key] == nil {
				stats[q.key] = e.stats[q.key]
				if stats[q.key] == nil {
					stats[q.key] = &quotaStats{
						tlmEvents: metrics.TlmQuotaDroppedEvents.WithValues(q.key),
						tlmBytes:  metrics.TlmQuotaDroppedBytes.WithValues(q.key),
					}
				}
			}
			q.stats = stats[q.key]
			quotas = append(quotas, q)
		}
	}
	for k := range e.stats {
		if stats[k] == nil {
			metrics.TlmQuotaDroppedEvents.Delete(k)
			metrics.TlmQuotaDroppedBytes.Delete(k)
		}
	}
	e.stats = stats
	e.active.Store(newRuleSet(quotas, e.lastSync, e.hostname, e.hostTags))
	if e.advance(e.lastSync) && e.tickEvery > 0 && !e.ticking {
		e.ticking = true
		go e.runClock()
	}
	return result
}

// SetHostTags sets the tags that logs from hostname, or with no hostname, match quotas on without carrying them.
func (e *Enforcer) SetHostTags(tags []string, hostname string) {
	norm := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != "" {
			norm = append(norm, asciiLower(t))
		}
	}
	slices.Sort(norm)
	norm = slices.Compact(norm)

	e.mu.Lock()
	defer e.mu.Unlock()
	if hostname == e.hostname && slices.Equal(norm, e.hostTags) {
		return
	}
	e.hostname, e.hostTags = hostname, norm
	if rs := e.active.Load(); rs != nil {
		e.active.Store(newRuleSet(rs.quotas, e.now(), hostname, norm))
	}
}

// HostTags returns the hostname and normalized tags last passed to SetHostTags.
func (e *Enforcer) HostTags() (hostname string, tags []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.hostname, slices.Clone(e.hostTags)
}

// Active reports whether any quota is enforcing.
func (e *Enforcer) Active() bool {
	rs := e.active.Load()
	return rs != nil && len(rs.quotas) > 0
}

func (e *Enforcer) runClock() {
	t := time.NewTicker(e.tickEvery)
	defer t.Stop()
	for range t.C {
		if !e.tick() {
			return
		}
	}
}

// tick advances the clock to now and reports whether quotas remain active.
func (e *Enforcer) tick() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.advance(e.now()) {
		e.ticking = false
		return false
	}
	return true
}

// advance sets the clock to now, drops expired quotas, and reports whether any remain. e.mu must be held.
func (e *Enforcer) advance(now time.Time) bool {
	e.clock.Store(now.Unix())
	rs := e.active.Load()
	if len(rs.quotas) > 0 && !now.Before(rs.nextExpiry) {
		rs = newRuleSet(rs.quotas, now, e.hostname, e.hostTags)
		e.active.Store(rs)
	}
	return len(rs.quotas) > 0
}

// Check reports whether msg should be dropped.
func (e *Enforcer) Check(msg *message.Message) bool {
	rs := e.active.Load()
	if rs == nil || len(rs.quotas) == 0 {
		return false
	}
	q := rs.match(msg.Origin, msg.Hostname)
	if q == nil {
		return false
	}
	size := msg.Size()
	q.stats.droppedEvents.Add(1)
	q.stats.droppedBytes.Add(size)
	q.stats.lastDrop.Store(e.clock.Load())
	q.stats.tlmEvents.Inc()
	q.stats.tlmBytes.Add(float64(size))
	return true
}

// ConfigStatus describes one received config for agent status.
type ConfigStatus struct {
	Path   string
	Error  string
	Quotas []QuotaStatus
}

// QuotaStatus describes one quota for agent status.
type QuotaStatus struct {
	Tags string
	// HostTags lists the quota's tags that the agent's host tags supply for local logs.
	HostTags      string
	State         string
	ExpiresAt     string
	DroppedEvents int64
	DroppedBytes  int64
	LastDrop      string
}

// Status returns the state of every received config.
func (e *Enforcer) Status() (lastSync string, configs []ConfigStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()
	for _, path := range slices.Sorted(maps.Keys(e.configs)) {
		entry := e.configs[path]
		cs := ConfigStatus{Path: path}
		if entry.err != nil {
			cs.Error = entry.err.Error()
		}
		for _, q := range entry.quotas {
			qs := QuotaStatus{
				Tags:          q.key,
				HostTags:      strings.Join(hostCovered(q.tags, e.hostTags), ","),
				State:         "enforcing",
				ExpiresAt:     formatTime(q.deadline),
				DroppedEvents: q.stats.droppedEvents.Load(),
				DroppedBytes:  q.stats.droppedBytes.Load(),
			}
			if q.expired(now) {
				qs.State = "expired"
			}
			if last := q.stats.lastDrop.Load(); last > 0 {
				qs.LastDrop = formatTime(time.Unix(last, 0))
			}
			cs.Quotas = append(cs.Quotas, qs)
		}
		configs = append(configs, cs)
	}
	if !e.lastSync.IsZero() {
		lastSync = formatTime(e.lastSync)
	}
	return lastSync, configs
}

func hostCovered(tags, hostTags []string) []string {
	var out []string
	for _, t := range tags {
		if _, found := slices.BinarySearch(hostTags, t); found {
			out = append(out, t)
		}
	}
	return out
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
