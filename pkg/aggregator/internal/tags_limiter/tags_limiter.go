// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

// Package tags_limiter TODO comment
// don't use an underscore in package name
package tags_limiter

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type entry struct {
	count uint64
	tags  []string
}

// Limiter exported type should have comment or be unexported
type Limiter struct {
	limit   int
	dropped map[ckey.TagsKey]*entry
}

// New exported function should have comment or be unexported
func New(limit int) *Limiter {
	if limit <= 0 {
		return nil
	}

	return &Limiter{
		limit:   limit,
		dropped: map[ckey.TagsKey]*entry{},
	}
}

// Check exported method should have comment or be unexported
func (l *Limiter) Check(taggerKey ckey.TagsKey, taggerTags, metricTags []string) bool {
	if l == nil {
		return true
	}

	if len(taggerTags)+len(metricTags) > l.limit {
		if e, ok := l.dropped[taggerKey]; !ok {
			e = &entry{
				count: 1,
				tags:  taggerTags,
			}
			l.dropped[taggerKey] = e
		} else {
			e.count++
		}

		return false
	}

	return true
}

// SendTelemetry exported method should have comment or be unexported
func (l *Limiter) SendTelemetry(timestamp float64, series metrics.SerieSink, hostname string, constTags []string) {
	if l == nil {
		return
	}

	constTags = append([]string{}, constTags...)
	constTags = append(constTags, "reason:too_many_tags")

	for _, e := range l.dropped {
		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_samples_dropped",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, e.tags),
			MType:  metrics.APICountType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(e.count)}},
		})
	}

	l.dropped = map[ckey.TagsKey]*entry{}
}
