// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package quota enforces log quota rules (LOGS_QUOTA) in the logs pipeline.
package quota

import (
	"fmt"
	"sort"
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

type configKey struct {
	source string
	path   string
}

type configEntry struct {
	raw        []byte
	quota      *quota
	err        error
	superseded bool
}

type seenVersion struct {
	version    int64
	receivedAt time.Time
}

type ruleSet struct {
	quotas []*quota
}

// Enforcer holds the active quota rules. Check is safe to call concurrently
// with Update.
type Enforcer struct {
	mu       sync.Mutex
	configs  map[configKey]*configEntry
	seen     map[string]seenVersion
	stats    map[string]*quotaStats
	hostTags func() []string
	lastSync time.Time

	now    func() time.Time
	active atomic.Pointer[ruleSet]
}

// NewEnforcer returns an enforcer with no active rules.
func NewEnforcer() *Enforcer {
	return &Enforcer{
		configs:  map[configKey]*configEntry{},
		seen:     map[string]seenVersion{},
		stats:    map[string]*quotaStats{},
		hostTags: func() []string { return nil },
		now:      time.Now,
	}
}

// SetHostTagsProvider sets the function used to resolve host_tag clauses.
func (e *Enforcer) SetHostTagsProvider(f func() []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.hostTags = f
	e.rebuild()
}

// Update replaces all configs from source with configs, keyed by path. A nil
// error in the result means that path was acknowledged.
func (e *Enforcer) Update(source string, configs map[string][]byte) map[string]error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for k := range e.configs {
		if k.source == source {
			if _, ok := configs[k.path]; !ok {
				delete(e.configs, k)
			}
		}
	}
	for path, raw := range configs {
		k := configKey{source, path}
		if old, ok := e.configs[k]; ok && string(old.raw) == string(raw) {
			continue
		}
		e.configs[k] = &configEntry{raw: raw}
	}
	e.lastSync = e.now()
	e.rebuild()

	result := make(map[string]error, len(configs))
	for path := range configs {
		result[path] = e.configs[configKey{source, path}].err
	}
	return result
}

// rebuild must be called with e.mu held.
func (e *Enforcer) rebuild() {
	now := e.now()
	hostTags := e.hostTags()

	parsed := map[configKey]*payload{}
	newest := map[string]int64{}
	for k, entry := range e.configs {
		entry.quota, entry.err, entry.superseded = nil, nil, false
		p, err := parsePayload(entry.raw)
		if err != nil {
			entry.err = err
			continue
		}
		parsed[k] = p
		if v, ok := newest[p.QuotaID]; !ok || p.Version > v {
			newest[p.QuotaID] = p.Version
		}
	}

	stale := map[string]int64{}
	for id, v := range newest {
		s, ok := e.seen[id]
		switch {
		case ok && v < s.version:
			stale[id] = s.version
		case !ok || v > s.version:
			e.seen[id] = seenVersion{version: v, receivedAt: now}
		}
	}
	for id := range e.seen {
		if _, ok := newest[id]; !ok {
			delete(e.seen, id)
		}
	}

	rs := &ruleSet{}
	claimed := map[string]bool{}
	for _, k := range sortedKeys(parsed) {
		p := parsed[k]
		entry := e.configs[k]
		if have, ok := stale[p.QuotaID]; ok {
			entry.err = fmt.Errorf("stale version %d, already applied %d", p.Version, have)
			continue
		}
		if p.Version < newest[p.QuotaID] || claimed[p.QuotaID] {
			entry.superseded = true
			continue
		}
		q, err := compile(p, e.seen[p.QuotaID].receivedAt, hostTags)
		if err != nil {
			entry.err = err
			continue
		}
		if e.stats[q.id] == nil {
			e.stats[q.id] = &quotaStats{}
		}
		q.stats = e.stats[q.id]
		entry.quota = q
		claimed[q.id] = true
		rs.quotas = append(rs.quotas, q)
	}
	e.active.Store(rs)
}

func sortedKeys(m map[configKey]*payload) []configKey {
	keys := make([]configKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		return keys[i].path < keys[j].path
	})
	return keys
}

// Check reports whether msg should be dropped, and by which quota.
func (e *Enforcer) Check(msg *message.Message) (quotaID string, drop bool) {
	rs := e.active.Load()
	if rs == nil || len(rs.quotas) == 0 {
		return "", false
	}
	now := e.now()
	for _, q := range rs.quotas {
		if q.expired(now) || !q.matches(msg) {
			continue
		}
		size := msg.Size()
		q.stats.droppedEvents.Add(1)
		q.stats.droppedBytes.Add(size)
		q.stats.lastDrop.Store(now.Unix())
		metrics.TlmQuotaDroppedEvents.Inc(q.id)
		metrics.TlmQuotaDroppedBytes.Add(float64(size), q.id)
		return q.id, true
	}
	return "", false
}

// QuotaStatus describes one received config for agent status.
type QuotaStatus struct {
	Source        string
	Path          string
	QuotaID       string
	Name          string
	Version       int64
	State         string
	Error         string
	Match         string
	IssuedAt      string
	ReceivedAt    string
	ExpiresAt     string
	DroppedEvents int64
	DroppedBytes  int64
	LastDrop      string
}

// Status returns the state of every received config.
func (e *Enforcer) Status() (lastSync string, quotas []QuotaStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()
	for k, entry := range e.configs {
		s := QuotaStatus{Source: k.source, Path: k.path}
		switch {
		case entry.err != nil:
			s.State = "error"
			s.Error = entry.err.Error()
		case entry.superseded:
			s.State = "superseded"
		case entry.quota != nil:
			q := entry.quota
			s.QuotaID, s.Name, s.Version, s.Match = q.id, q.name, q.version, q.matchString()
			s.ReceivedAt, s.ExpiresAt = formatTime(q.receivedAt), formatTime(q.deadline)
			s.IssuedAt = formatTime(q.issuedAt)
			s.DroppedEvents = q.stats.droppedEvents.Load()
			s.DroppedBytes = q.stats.droppedBytes.Load()
			if last := q.stats.lastDrop.Load(); last > 0 {
				s.LastDrop = formatTime(time.Unix(last, 0))
			}
			switch {
			case q.expired(now):
				s.State = "expired"
			case q.hostExcluded:
				s.State = "not applicable (host tags)"
			default:
				s.State = "enforcing"
			}
		}
		quotas = append(quotas, s)
	}
	sort.Slice(quotas, func(i, j int) bool {
		if quotas[i].Source != quotas[j].Source {
			return quotas[i].Source < quotas[j].Source
		}
		return quotas[i].Path < quotas[j].Path
	})
	if !e.lastSync.IsZero() {
		lastSync = formatTime(e.lastSync)
	}
	return lastSync, quotas
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
