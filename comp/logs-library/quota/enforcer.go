// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package quota enforces log quotas delivered over Remote Config in the logs pipeline.
package quota

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

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
}

type configEntry struct {
	quotas []*quota
	err    error
}

type ruleSet struct {
	quotas []*quota
}

// Enforcer holds the active quotas. Check is safe to call concurrently with Update.
type Enforcer struct {
	mu       sync.Mutex
	configs  map[string]*configEntry
	stats    map[string]*quotaStats
	lastSync time.Time

	now    func() time.Time
	active atomic.Pointer[ruleSet]
}

// NewEnforcer returns an enforcer with no active quotas.
func NewEnforcer() *Enforcer {
	return &Enforcer{
		configs: map[string]*configEntry{},
		stats:   map[string]*quotaStats{},
		now:     time.Now,
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

	rs := &ruleSet{}
	stats := map[string]*quotaStats{}
	for _, path := range slices.Sorted(maps.Keys(e.configs)) {
		for _, q := range e.configs[path].quotas {
			k := q.key()
			if stats[k] == nil {
				stats[k] = e.stats[k]
				if stats[k] == nil {
					stats[k] = &quotaStats{}
				}
			}
			q.stats = stats[k]
			rs.quotas = append(rs.quotas, q)
		}
	}
	e.stats = stats
	e.active.Store(rs)
	return result
}

// Check reports whether msg should be dropped.
func (e *Enforcer) Check(msg *message.Message) bool {
	rs := e.active.Load()
	if rs == nil || len(rs.quotas) == 0 {
		return false
	}
	now := e.now()
	var tags []string
	tagsLoaded := false
	for _, q := range rs.quotas {
		if q.expired(now) {
			continue
		}
		if !tagsLoaded && len(q.tags) > 0 {
			tags, tagsLoaded = msg.Origin.Tags(), true
		}
		if !q.matches(tags) {
			continue
		}
		size := msg.Size()
		q.stats.droppedEvents.Add(1)
		q.stats.droppedBytes.Add(size)
		q.stats.lastDrop.Store(now.Unix())
		metrics.TlmQuotaDroppedEvents.Inc(q.key())
		metrics.TlmQuotaDroppedBytes.Add(float64(size), q.key())
		return true
	}
	return false
}

// ConfigStatus describes one received config for agent status.
type ConfigStatus struct {
	Path   string
	Error  string
	Quotas []QuotaStatus
}

// QuotaStatus describes one quota for agent status.
type QuotaStatus struct {
	Tags          string
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
				Tags:          q.key(),
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

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
