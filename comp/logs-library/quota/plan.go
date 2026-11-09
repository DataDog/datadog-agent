// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quota

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// plan is the result of matching one rule set against one tag snapshot under one tag filter.
type plan struct {
	rs     *ruleSet
	filter sources.TagFilter
	// full is the index of the first quota the snapshot satisfies alone, or len(rs.quotas).
	full int
	// rest[i] lists the tags quota i still needs from the message, for i < full, or nil if the filter drops one.
	rest    [][]string
	anchors map[string][]int
	// serviceAnchors holds the anchors on service:<value> keyed by value, for messages carrying their own service.
	serviceAnchors map[string][]int
}

func newPlan(rs *ruleSet, snap *message.TagSnapshot, f sources.TagFilter) *plan {
	have := map[string]struct{}{}
	for t := range snap.All() {
		have[t] = struct{}{}
	}
	p := &plan{rs: rs, filter: f, full: len(rs.quotas), anchors: map[string][]int{}}
	var flat []string
	for i, q := range rs.quotas {
		if f != nil && !q.retainedBy(f) {
			p.rest = append(p.rest, nil)
			continue
		}
		start := len(flat)
		for _, t := range q.tags {
			if _, ok := have[t]; !ok {
				flat = append(flat, t)
			}
		}
		rest := flat[start:len(flat):len(flat)]
		if len(rest) == 0 {
			p.full = i
			break
		}
		p.rest = append(p.rest, rest)
		p.anchors[rest[0]] = append(p.anchors[rest[0]], i)
		if service, ok := strings.CutPrefix(rest[0], "service:"); ok {
			if p.serviceAnchors == nil {
				p.serviceAnchors = map[string][]int{}
			}
			p.serviceAnchors[service] = append(p.serviceAnchors[service], i)
		}
	}
	return p
}

// match returns the index of the first quota matched by the snapshot plus msgTags and service, or len(rs.quotas).
func (p *plan) match(msgTags []string, service string) int {
	best := p.full
	if len(p.anchors) == 0 {
		return best
	}
	for _, t := range msgTags {
		if candidates := p.anchors[t]; len(candidates) > 0 {
			best = p.first(candidates, best, msgTags, service)
		}
	}
	if service != "" && len(p.serviceAnchors) > 0 {
		best = p.first(p.serviceAnchors[service], best, msgTags, service)
	}
	return best
}

// first returns the earliest candidate before best whose remaining tags the message has, or best.
func (p *plan) first(candidates []int, best int, msgTags []string, service string) int {
	for _, i := range candidates {
		if i >= best {
			break
		}
		if hasAll(msgTags, service, p.rest[i][1:]) {
			return i
		}
	}
	return best
}

func hasAll(msgTags []string, service string, want []string) bool {
	for _, t := range want {
		if !slices.Contains(msgTags, t) && !isService(t, service) {
			return false
		}
	}
	return true
}

func isService(tag, service string) bool {
	if service == "" {
		return false
	}
	v, ok := strings.CutPrefix(tag, "service:")
	return ok && v == service
}

func (rs *ruleSet) planFor(snap *message.TagSnapshot, f sources.TagFilter) *plan {
	if p, _ := snap.Cached().(*plan); p != nil && p.rs == rs && p.filter == f {
		return p
	}
	p := newPlan(rs, snap, f)
	snap.SetCached(p)
	return p
}

// match returns the first quota whose tags all survive into o's transport tags, or nil.
func (rs *ruleSet) match(o *message.Origin) *quota {
	var f sources.TagFilter
	if o != nil && o.LogSource != nil {
		f, _ = o.LogSource.TagFilter()
	}
	if snap, msgTags, service, ok := o.TagSnapshot(); ok {
		if i := rs.planFor(snap, f).match(msgTags, service); i < len(rs.quotas) {
			return rs.quotas[i]
		}
		return nil
	}
	for _, q := range rs.quotas {
		if q.matches(o) && q.retainedBy(f) {
			return q
		}
	}
	return nil
}
