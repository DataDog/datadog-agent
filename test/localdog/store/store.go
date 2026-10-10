// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Options configures retention limits.
type Options struct {
	MaxLogs            int
	MaxSpans           int
	MaxPointsPerSeries int
	Retention          time.Duration
}

// DefaultOptions are sized for a laptop: a few hundred MB at most.
func DefaultOptions() Options {
	return Options{
		MaxLogs:            200_000,
		MaxSpans:           300_000,
		MaxPointsPerSeries: 10_000,
		Retention:          24 * time.Hour,
	}
}

// Store keeps logs, spans and metrics in memory. It is safe for concurrent use.
type Store struct {
	opts Options

	mu sync.RWMutex

	logs    []*Log // ordered by arrival, oldest first
	logSeq  uint64
	spans   []*Span
	spanSeq uint64
	// traces indexes spans by trace ID.
	traces map[string][]*Span

	series      map[string]*Series
	seriesNames map[string][]*Series

	hosts    map[string]int64
	services map[string]int64
	payloads map[string]int
	lastSeen int64
}

// New creates an empty store.
func New(opts Options) *Store {
	return &Store{
		opts:        opts,
		traces:      map[string][]*Span{},
		series:      map[string]*Series{},
		seriesNames: map[string][]*Series{},
		hosts:       map[string]int64{},
		services:    map[string]int64{},
		payloads:    map[string]int{},
	}
}

// RecordPayload counts a payload received on an intake route.
func (s *Store) RecordPayload(route string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads[route]++
	s.lastSeen = time.Now().UnixMilli()
}

func (s *Store) touch(host, service string, ts int64) {
	if host != "" {
		s.hosts[host] = max(s.hosts[host], ts)
	}
	if service != "" {
		s.services[service] = max(s.services[service], ts)
	}
}

// AddLogs appends logs, evicting the oldest ones past the retention limit.
func (s *Store) AddLogs(logs []*Log) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range logs {
		s.logSeq++
		if l.ID == "" {
			l.ID = "AQAAA" + strconv.FormatInt(l.Timestamp, 36) + strconv.FormatUint(s.logSeq, 36)
		}
		s.touch(l.Host, l.Service, l.Timestamp)
		s.logs = append(s.logs, l)
	}
	if over := len(s.logs) - s.opts.MaxLogs; over > 0 {
		s.logs = append([]*Log(nil), s.logs[over:]...)
	}
}

// AddSpans appends spans and indexes them by trace.
func (s *Store) AddSpans(spans []*Span) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	for _, sp := range spans {
		s.spanSeq++
		sp.ingestOrder = s.spanSeq
		sp.ReceivedAt = now
		s.touch(sp.Host, sp.Service, sp.Start/1e6)
		s.spans = append(s.spans, sp)
		s.traces[sp.TraceID] = append(s.traces[sp.TraceID], sp)
	}
	if over := len(s.spans) - s.opts.MaxSpans; over > 0 {
		for _, sp := range s.spans[:over] {
			s.removeFromTrace(sp)
		}
		s.spans = append([]*Span(nil), s.spans[over:]...)
	}
}

func (s *Store) removeFromTrace(sp *Span) {
	t := s.traces[sp.TraceID]
	for i, other := range t {
		if other == sp {
			t = append(t[:i], t[i+1:]...)
			break
		}
	}
	if len(t) == 0 {
		delete(s.traces, sp.TraceID)
	} else {
		s.traces[sp.TraceID] = t
	}
}

// SeriesKey builds a stable identifier for a metric name + tag set.
func SeriesKey(name, host string, tags []string) string {
	sorted := append([]string(nil), tags...)
	sort.Strings(sorted)
	return name + "|" + host + "|" + strings.Join(sorted, ",")
}

// AddPoints merges datapoints into the series identified by name/host/tags.
func (s *Store) AddPoints(name, metricType, host, unit string, interval int64, tags []string, points []Point) {
	if len(points) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := SeriesKey(name, host, tags)
	ser, ok := s.series[key]
	if !ok {
		sortedTags := append([]string(nil), tags...)
		sort.Strings(sortedTags)
		ser = &Series{Key: key, Name: name, Type: metricType, Host: host, Tags: sortedTags, Unit: unit}
		s.series[key] = ser
		s.seriesNames[name] = append(s.seriesNames[name], ser)
	}
	if unit != "" {
		ser.Unit = unit
	}
	if interval > 0 {
		ser.Interval = interval
	}
	s.touch(host, "", time.Now().UnixMilli())
	ser.Points = append(ser.Points, points...)
	if !sort.SliceIsSorted(ser.Points, func(i, j int) bool { return ser.Points[i].Timestamp < ser.Points[j].Timestamp }) {
		sort.SliceStable(ser.Points, func(i, j int) bool { return ser.Points[i].Timestamp < ser.Points[j].Timestamp })
	}
	if over := len(ser.Points) - s.opts.MaxPointsPerSeries; over > 0 {
		ser.Points = append([]Point(nil), ser.Points[over:]...)
	}
}

// Prune drops data older than the retention window.
func (s *Store) Prune() {
	cutoff := time.Now().Add(-s.opts.Retention)
	cutMs := cutoff.UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	i := sort.Search(len(s.logs), func(i int) bool { return s.logs[i].Timestamp >= cutMs })
	// logs arrive roughly in order; only trim a clean prefix
	if i > 0 {
		s.logs = append([]*Log(nil), s.logs[i:]...)
	}
	j := 0
	for j < len(s.spans) && s.spans[j].ReceivedAt < cutMs {
		s.removeFromTrace(s.spans[j])
		j++
	}
	if j > 0 {
		s.spans = append([]*Span(nil), s.spans[j:]...)
	}
	cutSec := cutoff.Unix()
	for key, ser := range s.series {
		k := sort.Search(len(ser.Points), func(i int) bool { return ser.Points[i].Timestamp >= cutSec })
		if k == len(ser.Points) {
			delete(s.series, key)
			list := s.seriesNames[ser.Name]
			for idx, other := range list {
				if other == ser {
					list = append(list[:idx], list[idx+1:]...)
					break
				}
			}
			if len(list) == 0 {
				delete(s.seriesNames, ser.Name)
			} else {
				s.seriesNames[ser.Name] = list
			}
			continue
		}
		if k > 0 {
			ser.Points = append([]Point(nil), ser.Points[k:]...)
		}
	}
}

// Reset clears everything.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = nil
	s.spans = nil
	s.traces = map[string][]*Span{}
	s.series = map[string]*Series{}
	s.seriesNames = map[string][]*Series{}
	s.hosts = map[string]int64{}
	s.services = map[string]int64{}
	s.payloads = map[string]int{}
}

// Logs calls fn for each log from newest to oldest until fn returns false.
func (s *Store) Logs(fn func(*Log) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.logs) - 1; i >= 0; i-- {
		if !fn(s.logs[i]) {
			return
		}
	}
}

// Spans calls fn for each span from newest to oldest until fn returns false.
func (s *Store) Spans(fn func(*Span) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.spans) - 1; i >= 0; i-- {
		if !fn(s.spans[i]) {
			return
		}
	}
}

// Trace returns the spans of a trace sorted by start time.
func (s *Store) Trace(traceID string) []*Span {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spans := append([]*Span(nil), s.traces[traceID]...)
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	return spans
}

// MetricNames returns all known metric names, sorted.
func (s *Store) MetricNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.seriesNames))
	for n := range s.seriesNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// SeriesFor returns copies of all series for a metric name, with points in [from, to] (unix seconds).
func (s *Store) SeriesFor(name string, from, to int64) []*Series {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Series, 0, len(s.seriesNames[name]))
	for _, ser := range s.seriesNames[name] {
		lo := sort.Search(len(ser.Points), func(i int) bool { return ser.Points[i].Timestamp >= from })
		hi := sort.Search(len(ser.Points), func(i int) bool { return ser.Points[i].Timestamp > to })
		cp := *ser
		cp.Points = append([]Point(nil), ser.Points[lo:hi]...)
		out = append(out, &cp)
	}
	return out
}

// Stats returns counters about the stored data.
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{
		Logs:         len(s.logs),
		Spans:        len(s.spans),
		Traces:       len(s.traces),
		MetricSeries: len(s.series),
		MetricNames:  len(s.seriesNames),
		Payloads:     map[string]int{},
		LastPayload:  s.lastSeen,
	}
	for _, ser := range s.series {
		st.Points += len(ser.Points)
	}
	for h := range s.hosts {
		st.Hosts = append(st.Hosts, h)
	}
	for svc := range s.services {
		st.Services = append(st.Services, svc)
	}
	sort.Strings(st.Hosts)
	sort.Strings(st.Services)
	for k, v := range s.payloads {
		st.Payloads[k] = v
	}
	return st
}
