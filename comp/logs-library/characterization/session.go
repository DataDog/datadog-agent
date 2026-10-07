// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package characterization owns bounded, session-scoped logs load observations.
package characterization

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SchemaVersion    = 1
	Kind             = "datadog-agent-logs-characterization"
	MaxSources       = 4096
	maxGroups        = 64
	EnabledConfigKey = "logs_config.experimental_characterization.enabled"
)

var (
	ErrSessionActive   = errors.New("a logs characterization session is already active")
	ErrSessionNotFound = errors.New("logs characterization session not found")
	ErrInvalidDuration = errors.New("duration must be between 1 second and 24 hours")

	Default = NewManager()
)

var histogramBounds = [...]float64{64, 128, 256, 512, 1024, 4096, 16384, 65536, 262144, 1048576}
var interarrivalBounds = [...]float64{0.001, 0.01, 0.1, 1, 5, 30, 60, 300}

// MessageObservation is the allowlisted, non-content input accepted by a session.
type MessageObservation struct {
	ObservedAt    time.Time
	ContentBytes  int
	RawBytes      int
	TagCount      int
	TagBytes      int
	SourceType    string
	Pipeline      string
	PayloadFamily string
	HasService    bool
	HasSource     bool
	SourceHash    uint64
	HasSourceID   bool
}

// Histogram is a fixed-size distribution. Counts includes one overflow bucket.
type Histogram struct {
	Bounds []float64 `json:"bounds"`
	Counts []uint64  `json:"counts"`
	Count  uint64    `json:"count"`
	Sum    float64   `json:"sum"`
	Min    float64   `json:"min,omitempty"`
	Max    float64   `json:"max,omitempty"`
}

// Aggregate contains numeric observations for one bounded grouping.
type Aggregate struct {
	Events          uint64    `json:"events"`
	ContentBytes    uint64    `json:"content_bytes"`
	RawBytes        uint64    `json:"raw_bytes"`
	TagCount        uint64    `json:"tag_count"`
	TagBytes        uint64    `json:"tag_bytes"`
	WithService     uint64    `json:"with_service"`
	WithSource      uint64    `json:"with_source"`
	MessageSizes    Histogram `json:"message_sizes"`
	RawSizes        Histogram `json:"raw_sizes"`
	TagCounts       Histogram `json:"tag_counts"`
	TagSizes        Histogram `json:"tag_sizes"`
	Interarrivals   Histogram `json:"interarrival_seconds"`
	SourceCount     uint64    `json:"source_count"`
	MissingSourceID uint64    `json:"missing_source_id"`
}

// Group identifies an allowlisted source-type and internal pipeline grouping.
type Group struct {
	SourceType string    `json:"source_type"`
	Pipeline   string    `json:"pipeline"`
	Aggregate  Aggregate `json:"aggregate"`
}

// RateWindow is a fixed ten-second ingress bucket used to preserve burst shape.
type RateWindow struct {
	StartedAt    time.Time `json:"started_at"`
	EndsAt       time.Time `json:"ends_at"`
	Events       uint64    `json:"events"`
	ContentBytes uint64    `json:"content_bytes"`
	RawBytes     uint64    `json:"raw_bytes"`
}

// Lifecycle contains file lifecycle signals observed inside the same session window.
type Lifecycle struct {
	Rotations         uint64    `json:"rotations"`
	RotationIntervals Histogram `json:"rotation_interval_seconds"`
}

// Snapshot is the stable, versioned result of one bounded observation window.
type Snapshot struct {
	SchemaVersion            int                  `json:"schema_version"`
	Kind                     string               `json:"kind"`
	SessionID                string               `json:"session_id"`
	State                    string               `json:"state"`
	StartedAt                time.Time            `json:"started_at"`
	EndsAt                   time.Time            `json:"ends_at"`
	EndedAt                  time.Time            `json:"ended_at,omitempty"`
	RequestedDurationSeconds float64              `json:"requested_duration_seconds"`
	Totals                   Aggregate            `json:"totals"`
	PayloadFamilies          map[string]Aggregate `json:"payload_families"`
	Groups                   []Group              `json:"groups"`
	RateWindows              []RateWindow         `json:"rate_windows"`
	Lifecycle                *Lifecycle           `json:"lifecycle,omitempty"`
	SourceCardinalityCapped  bool                 `json:"source_cardinality_capped"`
	GroupLimitReached        bool                 `json:"group_limit_reached"`
}

type sourceIdentity struct {
	sourceType string
	pipeline   string
	hash       uint64
}

type groupKey struct {
	sourceType string
	pipeline   string
}

type session struct {
	mu          sync.Mutex
	snapshot    Snapshot
	groups      map[groupKey]*Aggregate
	sources     map[sourceIdentity]struct{}
	lastIngress map[groupKey]time.Time
	rateWindows map[int]*RateWindow
	closed      bool
}

// Manager permits at most one active session and retains only the latest result.
type Manager struct {
	control sync.Mutex
	active  atomic.Pointer[session]
	latest  *Snapshot
	now     func() time.Time
}

// NewManager creates an empty session manager.
func NewManager() *Manager { return &Manager{now: time.Now} }

// Start opens a bounded observation window.
func (m *Manager) Start(duration time.Duration) (Snapshot, error) {
	if duration < time.Second || duration > 24*time.Hour {
		return Snapshot{}, ErrInvalidDuration
	}
	m.control.Lock()
	defer m.control.Unlock()
	m.finalizeExpiredLocked(m.now())
	if m.active.Load() != nil {
		return Snapshot{}, ErrSessionActive
	}
	now := m.now()
	s := &session{
		snapshot: Snapshot{
			SchemaVersion:            SchemaVersion,
			Kind:                     Kind,
			SessionID:                newSessionID(),
			State:                    "active",
			StartedAt:                now,
			EndsAt:                   now.Add(duration),
			RequestedDurationSeconds: duration.Seconds(),
			Totals:                   newAggregate(),
			PayloadFamilies:          make(map[string]Aggregate),
		},
		groups:      make(map[groupKey]*Aggregate),
		sources:     make(map[sourceIdentity]struct{}),
		lastIngress: make(map[groupKey]time.Time),
		rateWindows: make(map[int]*RateWindow),
	}
	m.active.Store(s)
	return s.copySnapshot(), nil
}

// Record adds an allowlisted observation when its timestamp falls in the active window.
func (m *Manager) Record(observation MessageObservation) {
	s := m.active.Load()
	if s == nil || observation.ObservedAt.Before(s.snapshot.StartedAt) || !observation.ObservedAt.Before(s.snapshot.EndsAt) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	family := normalizePayloadFamily(observation.PayloadFamily)
	key := groupKey{sourceType: normalizeSourceType(observation.SourceType), pipeline: observation.Pipeline}
	group := s.groups[key]
	if group == nil {
		if len(s.groups) >= maxGroups {
			s.snapshot.GroupLimitReached = true
			key = groupKey{sourceType: "other", pipeline: "other"}
		}
		group = s.groups[key]
		if group == nil {
			value := newAggregate()
			group = &value
			s.groups[key] = group
		}
	}
	updateAggregate(&s.snapshot.Totals, observation)
	familyAggregate := s.snapshot.PayloadFamilies[family]
	if familyAggregate.MessageSizes.Bounds == nil {
		familyAggregate = newAggregate()
	}
	updateAggregate(&familyAggregate, observation)
	s.snapshot.PayloadFamilies[family] = familyAggregate
	updateAggregate(group, observation)
	s.recordRateWindow(observation)
	if previous, found := s.lastIngress[key]; found && observation.ObservedAt.After(previous) {
		seconds := observation.ObservedAt.Sub(previous).Seconds()
		observeHistogram(&group.Interarrivals, seconds)
		observeHistogram(&s.snapshot.Totals.Interarrivals, seconds)
	}
	s.lastIngress[key] = observation.ObservedAt
	if !observation.HasSourceID {
		group.MissingSourceID++
		s.snapshot.Totals.MissingSourceID++
		return
	}
	identity := sourceIdentity{sourceType: key.sourceType, pipeline: key.pipeline, hash: observation.SourceHash}
	if _, found := s.sources[identity]; found {
		return
	}
	if len(s.sources) >= MaxSources {
		s.snapshot.SourceCardinalityCapped = true
		return
	}
	s.sources[identity] = struct{}{}
	group.SourceCount++
	s.snapshot.Totals.SourceCount++
}

// RecordRotation adds one aggregate-only file lifecycle observation.
func (m *Manager) RecordRotation(observedAt time.Time, intervalSeconds float64) {
	s := m.active.Load()
	if s == nil || observedAt.Before(s.snapshot.StartedAt) || !observedAt.Before(s.snapshot.EndsAt) || intervalSeconds <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.snapshot.Lifecycle == nil {
		s.snapshot.Lifecycle = &Lifecycle{RotationIntervals: newHistogram(interarrivalBounds[:])}
	}
	s.snapshot.Lifecycle.Rotations++
	observeHistogram(&s.snapshot.Lifecycle.RotationIntervals, intervalSeconds)
}

func (s *session) recordRateWindow(observation MessageObservation) {
	const width = 10 * time.Second
	index := int(observation.ObservedAt.Sub(s.snapshot.StartedAt) / width)
	window := s.rateWindows[index]
	if window == nil {
		startedAt := s.snapshot.StartedAt.Add(time.Duration(index) * width)
		endsAt := startedAt.Add(width)
		if endsAt.After(s.snapshot.EndsAt) {
			endsAt = s.snapshot.EndsAt
		}
		window = &RateWindow{StartedAt: startedAt, EndsAt: endsAt}
		s.rateWindows[index] = window
	}
	window.Events++
	window.ContentBytes += nonnegative(observation.ContentBytes)
	window.RawBytes += nonnegative(observation.RawBytes)
}

// Status returns the active session or the latest completed result.
func (m *Manager) Status() (Snapshot, error) {
	m.control.Lock()
	defer m.control.Unlock()
	m.finalizeExpiredLocked(m.now())
	if s := m.active.Load(); s != nil {
		return s.copySnapshot(), nil
	}
	if m.latest == nil {
		return Snapshot{}, ErrSessionNotFound
	}
	return cloneSnapshot(*m.latest), nil
}

// Stop closes the active session. An empty ID stops whichever session is active.
func (m *Manager) Stop(sessionID string) (Snapshot, error) {
	m.control.Lock()
	defer m.control.Unlock()
	m.finalizeExpiredLocked(m.now())
	s := m.active.Load()
	if s != nil {
		if sessionID != "" && sessionID != s.snapshot.SessionID {
			return Snapshot{}, ErrSessionNotFound
		}
		return m.finalizeLocked(s, m.now()), nil
	}
	if m.latest == nil || (sessionID != "" && sessionID != m.latest.SessionID) {
		return Snapshot{}, ErrSessionNotFound
	}
	return cloneSnapshot(*m.latest), nil
}

func (m *Manager) finalizeExpiredLocked(now time.Time) {
	if s := m.active.Load(); s != nil && !now.Before(s.snapshot.EndsAt) {
		m.finalizeLocked(s, s.snapshot.EndsAt)
	}
}

func (m *Manager) finalizeLocked(s *session, endedAt time.Time) Snapshot {
	m.active.CompareAndSwap(s, nil)
	s.mu.Lock()
	s.closed = true
	s.snapshot.State = "completed"
	s.snapshot.EndedAt = endedAt
	s.materializeGroups()
	result := cloneSnapshot(s.snapshot)
	s.mu.Unlock()
	m.latest = &result
	return result
}

func (s *session) copySnapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.materializeGroups()
	return cloneSnapshot(s.snapshot)
}

func (s *session) materializeGroups() {
	s.snapshot.Groups = s.snapshot.Groups[:0]
	for key, aggregate := range s.groups {
		s.snapshot.Groups = append(s.snapshot.Groups, Group{SourceType: key.sourceType, Pipeline: key.pipeline, Aggregate: *aggregate})
	}
	sort.Slice(s.snapshot.Groups, func(i, j int) bool {
		if s.snapshot.Groups[i].SourceType == s.snapshot.Groups[j].SourceType {
			return s.snapshot.Groups[i].Pipeline < s.snapshot.Groups[j].Pipeline
		}
		return s.snapshot.Groups[i].SourceType < s.snapshot.Groups[j].SourceType
	})
	s.snapshot.RateWindows = s.snapshot.RateWindows[:0]
	for _, window := range s.rateWindows {
		s.snapshot.RateWindows = append(s.snapshot.RateWindows, *window)
	}
	sort.Slice(s.snapshot.RateWindows, func(i, j int) bool {
		return s.snapshot.RateWindows[i].StartedAt.Before(s.snapshot.RateWindows[j].StartedAt)
	})
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Totals = cloneAggregate(snapshot.Totals)
	families := make(map[string]Aggregate, len(snapshot.PayloadFamilies))
	for family, aggregate := range snapshot.PayloadFamilies {
		families[family] = cloneAggregate(aggregate)
	}
	snapshot.PayloadFamilies = families
	groups := make([]Group, len(snapshot.Groups))
	for i, group := range snapshot.Groups {
		groups[i] = group
		groups[i].Aggregate = cloneAggregate(group.Aggregate)
	}
	snapshot.Groups = groups
	snapshot.RateWindows = append([]RateWindow(nil), snapshot.RateWindows...)
	if snapshot.Lifecycle != nil {
		lifecycle := *snapshot.Lifecycle
		lifecycle.RotationIntervals = cloneHistogram(lifecycle.RotationIntervals)
		snapshot.Lifecycle = &lifecycle
	}
	return snapshot
}

func cloneAggregate(aggregate Aggregate) Aggregate {
	aggregate.MessageSizes = cloneHistogram(aggregate.MessageSizes)
	aggregate.RawSizes = cloneHistogram(aggregate.RawSizes)
	aggregate.TagCounts = cloneHistogram(aggregate.TagCounts)
	aggregate.TagSizes = cloneHistogram(aggregate.TagSizes)
	aggregate.Interarrivals = cloneHistogram(aggregate.Interarrivals)
	return aggregate
}

func cloneHistogram(histogram Histogram) Histogram {
	histogram.Bounds = append([]float64(nil), histogram.Bounds...)
	histogram.Counts = append([]uint64(nil), histogram.Counts...)
	return histogram
}

func newAggregate() Aggregate {
	return Aggregate{
		MessageSizes: newHistogram(histogramBounds[:]), RawSizes: newHistogram(histogramBounds[:]),
		TagCounts: newHistogram(histogramBounds[:]), TagSizes: newHistogram(histogramBounds[:]),
		Interarrivals: newHistogram(interarrivalBounds[:]),
	}
}

func newHistogram(bounds []float64) Histogram {
	return Histogram{Bounds: append([]float64(nil), bounds...), Counts: make([]uint64, len(bounds)+1)}
}

func updateAggregate(aggregate *Aggregate, observation MessageObservation) {
	aggregate.Events++
	aggregate.ContentBytes += nonnegative(observation.ContentBytes)
	aggregate.RawBytes += nonnegative(observation.RawBytes)
	aggregate.TagCount += nonnegative(observation.TagCount)
	aggregate.TagBytes += nonnegative(observation.TagBytes)
	if observation.HasService {
		aggregate.WithService++
	}
	if observation.HasSource {
		aggregate.WithSource++
	}
	observeHistogram(&aggregate.MessageSizes, float64(max(0, observation.ContentBytes)))
	observeHistogram(&aggregate.RawSizes, float64(max(0, observation.RawBytes)))
	observeHistogram(&aggregate.TagCounts, float64(max(0, observation.TagCount)))
	observeHistogram(&aggregate.TagSizes, float64(max(0, observation.TagBytes)))
}

func observeHistogram(histogram *Histogram, value float64) {
	index := len(histogram.Bounds)
	for i, bound := range histogram.Bounds {
		if value <= bound {
			index = i
			break
		}
	}
	histogram.Counts[index]++
	if histogram.Count == 0 {
		histogram.Min = value
		histogram.Max = value
	}
	histogram.Count++
	histogram.Sum += value
	if value < histogram.Min {
		histogram.Min = value
	}
	if value > histogram.Max {
		histogram.Max = value
	}
}

func nonnegative(value int) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func normalizePayloadFamily(value string) string {
	switch value {
	case "empty", "plain", "json", "datadog_json", "apache_common", "syslog5424":
		return value
	default:
		return "unknown"
	}
}

func normalizeSourceType(value string) string {
	switch value {
	case "tcp", "udp", "file", "docker", "containerd", "journald", "integration", "windows_event", "string_channel", "unknown":
		return value
	default:
		return "unknown"
	}
}

func newSessionID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(value[:])
}
