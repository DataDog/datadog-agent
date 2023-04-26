// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package limiter

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type entry struct {
	current  int // number of contexts currently in aggregator
	accepted int // number of accepted samples
	rejected int // number of rejected samples
	tags     []string
}

// Limiter tracks number of contexts based on origin detection metrics
// and rejects samples if the number goes over the limit.
//
// Not thread safe.
type Limiter struct {
	key   string
	tags  []string
	limit int
	usage map[string]*entry
}

// New returns a new instance of limiter.
//
// If limit is zero or less the limiter is disabled.
func New(limit int, key string, tags []string) *Limiter {
	if limit <= 0 {
		return nil
	}

	if !strings.HasSuffix(key, ":") {
		key += ":"
	}

	hasKey := false
	tags = append([]string{}, tags...)
	for i := range tags {
		if !strings.HasSuffix(tags[i], ":") {
			tags[i] += ":"
		}
		hasKey = hasKey || key == tags[i]
	}

	if !hasKey {
		tags = append(tags, key)
	}

	return &Limiter{
		key:   key,
		tags:  tags,
		limit: limit,
		usage: map[string]*entry{},
	}
}

func (l *Limiter) identify(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, l.key) {
			return t
		}
	}
	return ""
}

func (l *Limiter) extractTags(src []string) []string {
	dst := make([]string, 0, len(l.tags))

	for _, t := range src {
		for _, p := range l.tags {
			if strings.HasPrefix(t, p) {
				dst = append(dst, t)
			}
		}
	}

	return dst
}

// Track is called for each new context. Returns true if the sample should be accepted, false
// otherwise.
func (l *Limiter) Track(tags []string) bool {
	if l == nil {
		return true
	}

	id := l.identify(tags)

	e := l.usage[id]
	if e == nil {
		e = &entry{
			tags: l.extractTags(tags),
		}
		l.usage[id] = e
	}

	if e.current >= l.limit {
		e.rejected++
		return false
	}

	e.current++
	e.accepted++
	return true
}

// Remove is called when context is expired to decrement current usage.
func (l *Limiter) Remove(tags []string) {
	if l == nil {
		return
	}

	id := l.identify(tags)

	if e := l.usage[id]; e != nil {
		e.current--
		if e.current <= 0 {
			delete(l.usage, id)
		}
	}
}

// SendTelemetry appends limiter metrics to the series sink.
func (l *Limiter) SendTelemetry(timestamp float64, series metrics.SerieSink, hostname string, constTags []string) {
	if l == nil {
		return
	}

	for _, e := range l.usage {
		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.limit",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, e.tags),
			MType:  metrics.APIGaugeType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(l.limit)}},
		})

		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.current",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, e.tags),
			MType:  metrics.APIGaugeType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(e.current)}},
		})

		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.accepted",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, e.tags),
			MType:  metrics.APICountType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(e.accepted)}},
		})
		e.accepted = 0

		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.rejected",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, e.tags),
			MType:  metrics.APICountType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(e.rejected)}},
		})
		e.rejected = 0
	}
}
