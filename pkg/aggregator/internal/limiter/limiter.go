// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

package limiter

import (
	"math"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type entry struct {
	current  int // number of contexts currently in aggregator
	rejected int // number of rejected samples

	lastExpireCount int // expireCount when seen last

	telemetryTags []string
}

// Limiter tracks number of contexts based on origin detection metrics
// and rejects samples if the number goes over the limit.
//
// Not thread safe.
type Limiter struct {
	keyTagName        string
	telemetryTagNames []string
	limit             int
	global            int // global limit
	current           int // sum(usage[*].current)
	usage             map[string]*entry

	// expireCount ensure eventual removal of entries that created an entry, but were never able
	// to create contexts due to the global limit.
	expireCount         int
	expireCountInterval int
}

// New returns a limiter with a per-sender limit.
//
// limit is the maximum number of contexts per sender. If zero or less, the limiter is disabled.
//
// keyTagName is the origin-detection tag name that will be used to identify the senders. Contexts
// that have no tag will be tracked as a single sender and the limit will still be applied.
//
// telemetryTagNames are additional tags that will be copied to the per-sender telemetry. Telemetry
// tags should have the same values for all containers that have the same key tag value and will be
// tracked as a single origin (e.g. if key is pod_name, then kube_namespace and kube_deployment are
// valid telemetry tags, but container_id is not). Only tags from the first sample will be used for
// all telemetry for the given sender.
func New(limit int, keyTagName string, telemetryTagNames []string) *Limiter {
	if limit <= 0 {
		return nil
	}

	return newLimiter(limit, math.MaxInt, 0, keyTagName, telemetryTagNames)
}

// NewGlobal returns a limiter with a global limit which will be equally split between senders
// will be equally distributed between origins.
func NewGlobal(global int, expireCountInterval int, key string, tags []string) *Limiter {
	if global <= 0 || global == math.MaxInt {
		return nil
	}

	return newLimiter(0, global, expireCountInterval, key, tags)
}

func newLimiter(limit, global int, expireCountInterval int, keyTagName string, telemetryTagNames []string) *Limiter {
	// Make sure all names end with a colon, so we don't accidentally match a part of the tag name, only the full name.
	// e.g. keyTagName="pod_name" should not match the tag "pod_name_alias:foo"
	if !strings.HasSuffix(keyTagName, ":") {
		keyTagName += ":"
	}

	hasKey := false
	telemetryTagNames = append([]string{}, telemetryTagNames...)
	for i := range telemetryTagNames {
		if !strings.HasSuffix(telemetryTagNames[i], ":") {
			telemetryTagNames[i] += ":"
		}
		hasKey = hasKey || keyTagName == telemetryTagNames[i]
	}

	// Make sure key tag is always set on the telemetry metrics.
	if !hasKey {
		telemetryTagNames = append(telemetryTagNames, keyTagName)
	}

	return &Limiter{
		keyTagName:          keyTagName,
		telemetryTagNames:   telemetryTagNames,
		limit:               limit,
		global:              global,
		usage:               map[string]*entry{},
		expireCountInterval: expireCountInterval,
	}
}

// getSenderId finds sender identifier given a set of origin detection tags.
//
// If the key tag is not found, returns empty string.
// method getSenderId should be getSenderID
func (l *Limiter) getSenderId(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, l.keyTagName) {
			return t
		}
	}
	return ""
}

// extractTelemetryTags returns a slice of tags that have l.telemetryTagNames prefixes.
func (l *Limiter) extractTelemetryTags(src []string) []string {
	dst := make([]string, 0, len(l.telemetryTagNames))

	for _, t := range src {
		for _, p := range l.telemetryTagNames {
			if strings.HasPrefix(t, p) {
				dst = append(dst, t)
			}
		}
	}

	return dst
}

func (l *Limiter) updateLimit() {
	if l.global < math.MaxInt && len(l.usage) > 0 {
		l.limit = l.global / len(l.usage)
	}
}

// Track is called for each new context. Returns true if the sample should be accepted, false
// otherwise.
func (l *Limiter) Track(tags []string) bool {
	if l == nil {
		return true
	}

	id := l.getSenderId(tags)

	e := l.usage[id]
	if e == nil {
		e = &entry{
			telemetryTags: l.extractTelemetryTags(tags),
		}
		l.usage[id] = e
		l.updateLimit()
	}

	e.lastExpireCount = l.expireCount

	if e.current >= l.limit || l.current >= l.global {
		e.rejected++
		return false
	}

	l.current++
	e.current++
	return true
}

// Remove is called when context is expired to decrement current usage.
func (l *Limiter) Remove(tags []string) {
	if l == nil {
		return
	}

	id := l.getSenderId(tags)

	if e := l.usage[id]; e != nil {
		l.current--
		e.current--
		if e.current <= 0 {
			delete(l.usage, id)
			l.updateLimit()
		}
	}
}

// IsOverLimit returns true if the context sender is over the limit and the context should be
// dropped.
func (l *Limiter) IsOverLimit(tags []string) bool {
	if l == nil {
		return false
	}

	if e := l.usage[l.getSenderId(tags)]; e != nil {
		return e.current > l.limit
	}

	return false
}

// ExpireEntries is called once per flush cycle to do internal bookkeeping and cleanups.
func (l *Limiter) ExpireEntries() {
	if l == nil {
		return
	}

	if l.expireCountInterval >= 0 {
		l.expireCount++
		tooOld := l.expireCount - l.expireCountInterval
		for id, e := range l.usage {
			if e.current == 0 && e.lastExpireCount < tooOld {
				delete(l.usage, id)
				l.updateLimit()
			}
		}
	}
}

// SendTelemetry appends limiter metrics to the series sink.
func (l *Limiter) SendTelemetry(timestamp float64, series metrics.SerieSink, hostname string, constTags []string) {
	if l == nil {
		return
	}

	droppedTags := append([]string{}, constTags...)
	droppedTags = append(droppedTags, "reason:too_many_contexts")

	series.Append(&metrics.Serie{
		Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.num_origins",
		Host:   hostname,
		Tags:   tagset.NewCompositeTags(constTags, nil),
		MType:  metrics.APIGaugeType,
		Points: []metrics.Point{{Ts: timestamp, Value: float64(len(l.usage))}},
	})

	if l.global < math.MaxInt {
		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.global_limit",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, nil),
			MType:  metrics.APIGaugeType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(l.global)}},
		})
	}

	for _, e := range l.usage {
		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.limit",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, e.telemetryTags),
			MType:  metrics.APIGaugeType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(l.limit)}},
		})

		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_context_limiter.current",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(constTags, e.telemetryTags),
			MType:  metrics.APIGaugeType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(e.current)}},
		})

		series.Append(&metrics.Serie{
			Name:   "datadog.agent.aggregator.dogstatsd_samples_dropped",
			Host:   hostname,
			Tags:   tagset.NewCompositeTags(droppedTags, e.telemetryTags),
			MType:  metrics.APICountType,
			Points: []metrics.Point{{Ts: timestamp, Value: float64(e.rejected)}},
		})
		e.rejected = 0
	}
}
