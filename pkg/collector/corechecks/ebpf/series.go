// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// seriesMessage is the message every series log line carries, which log
// filters and the pipeline's parsing rule match on.
const seriesMessage = "agent metrics batch"

type metricType string

const (
	metricTypeGauge metricType = "gauge"
	metricTypeCount metricType = "count"
)

// series holds every metric reported about one entity (a map, a program, a
// module, or the global scope) during one check run. Each series is logged
// as one line.
type series struct {
	// key is the identity tags in canonical form, used to look up and order series.
	key string
	// tags is the identity tags plus extra tags merged in from individual metrics.
	tags   map[string]string
	values map[string]float64
	types  map[string]metricType
}

// gauge records a gauge on the series.
func (s *series) gauge(name string, value float64) {
	s.add(name, metricTypeGauge, value, nil)
}

// add records one metric on the series. extraTags are tags only this metric
// carries; they are merged into the series tags.
func (s *series) add(name string, typ metricType, value float64, extraTags map[string]string) {
	for k, v := range extraTags {
		if prev, ok := s.tags[k]; ok && prev != v {
			log.Debugf("ebpf check: series %q: tag %s of %s is %q, keeping earlier value %q", s.key, k, name, v, prev)
			continue
		}
		s.tags[k] = v
	}
	if _, ok := s.values[name]; ok {
		log.Debugf("ebpf check: series %q: metric %s reported twice, keeping the last value", s.key, name)
	}
	s.values[name] = value
	s.types[name] = typ
}

type seriesPayload struct {
	Message   string                `json:"message"`
	Timestamp int64                 `json:"timestamp"`
	Tags      map[string]string     `json:"tags"`
	Values    map[string]float64    `json:"values"`
	Types     map[string]metricType `json:"types"`
}

// marshal encodes the series as one JSON log line. Metric names become keys
// with dots replaced by underscores, since Datadog reads dots in an attribute
// name as nesting.
func (s *series) marshal(timestamp time.Time) ([]byte, error) {
	p := seriesPayload{
		Message:   seriesMessage,
		Timestamp: timestamp.UnixMilli(),
		Tags:      s.tags,
		Values:    make(map[string]float64, len(s.values)),
		Types:     make(map[string]metricType, len(s.types)),
	}
	for name, value := range s.values {
		key := strings.ReplaceAll(name, ".", "_")
		p.Values[key] = value
		p.Types[key] = s.types[name]
	}
	return json.Marshal(p)
}

// seriesSet collects the series of one check run, keyed by identity.
type seriesSet struct {
	byKey map[string]*series
}

func newSeriesSet() *seriesSet {
	return &seriesSet{byKey: make(map[string]*series)}
}

// get returns the series identified by identity, creating it if needed.
func (ss *seriesSet) get(identity map[string]string) *series {
	key := seriesKey(identity)
	if s, ok := ss.byKey[key]; ok {
		return s
	}
	s := &series{
		key:    key,
		tags:   make(map[string]string, len(identity)),
		values: make(map[string]float64),
		types:  make(map[string]metricType),
	}
	maps.Copy(s.tags, identity)
	ss.byKey[key] = s
	return s
}

// nonEmpty returns the series that recorded at least one metric, in no
// particular order.
func (ss *seriesSet) nonEmpty() []*series {
	out := make([]*series, 0, len(ss.byKey))
	for _, s := range ss.byKey {
		if len(s.values) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// seriesKey renders identity tags in a canonical, sorted form.
func seriesKey(identity map[string]string) string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(identity)) {
		b.WriteString(k)
		b.WriteByte(':')
		b.WriteString(identity[k])
		b.WriteByte(',')
	}
	return b.String()
}
