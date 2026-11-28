// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quota

import (
	"strings"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// plan is the result of matching one rule set against one tag snapshot under one tag filter.
type plan struct {
	rs     *ruleSet
	filter sources.TagFilter
	// local plans match the host view's residual tags. other caches the opposite plan for mixed local and remote streams.
	local bool
	other atomic.Pointer[plan]
	// full is the index of the first quota the snapshot satisfies alone, or len(rs.quotas).
	full int
	// rest[i] lists the tags quota i still needs from the message, for i < full, or nil if the filter drops one.
	rest    [][]string
	anchors map[string][]int
	// serviceAnchors holds the anchors on service:<value> keyed by value, for messages carrying their own service.
	serviceAnchors map[string][]int
}

func newPlan(rs *ruleSet, snap *message.TagSnapshot, f sources.TagFilter, local bool) *plan {
	have := map[string]struct{}{}
	for t := range snap.All() {
		have[asciiLower(t)] = struct{}{}
	}
	p := &plan{rs: rs, filter: f, local: local, full: len(rs.quotas), anchors: map[string][]int{}}
	var flat []string
	for i, q := range rs.quotas {
		tags := q.tags
		if local {
			tags = rs.local.residual[i]
		}
		if !retainedBy(f, tags) {
			p.rest = append(p.rest, nil)
			continue
		}
		start := len(flat)
		for _, t := range tags {
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
		if candidates := lookupLower(p.anchors, t); len(candidates) > 0 {
			best = p.first(candidates, best, msgTags, service)
		}
	}
	if service != "" && len(p.serviceAnchors) > 0 {
		best = p.first(lookupLower(p.serviceAnchors, service), best, msgTags, service)
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
		if !containsFold(msgTags, t) && !isService(t, service) {
			return false
		}
	}
	return true
}

func containsFold(tags []string, tag string) bool {
	for _, t := range tags {
		if equalFold(t, tag) {
			return true
		}
	}
	return false
}

func isService(tag, service string) bool {
	if service == "" {
		return false
	}
	v, ok := strings.CutPrefix(tag, "service:")
	return ok && equalFold(v, service)
}

// lookupLower returns m[asciiLower(k)], folding short keys on the stack instead of allocating.
func lookupLower(m map[string][]int, k string) []int {
	var buf [128]byte
	if len(k) > len(buf) {
		return m[asciiLower(k)]
	}
	for i := 0; i < len(k); i++ {
		buf[i] = lowerASCII(k[i])
	}
	return m[string(buf[:len(k)])]
}

// equalFold is strings.EqualFold restricted to ASCII, matching how tag filters fold.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] && lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				b[j] = lowerASCII(b[j])
			}
			return string(b)
		}
	}
	return s
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func (rs *ruleSet) planFor(snap *message.TagSnapshot, f sources.TagFilter, local bool) *plan {
	p, _ := snap.Cached().(*plan)
	if p == nil || p.rs != rs || p.filter != f {
		p = newPlan(rs, snap, f, local)
		snap.SetCached(p)
		return p
	}
	if p.local == local {
		return p
	}
	if o := p.other.Load(); o != nil {
		return o
	}
	o := newPlan(rs, snap, f, local)
	p.other.Store(o)
	return o
}

// match returns the first quota whose tags all survive into o's transport tags or, for local logs, come from the host, or nil.
func (rs *ruleSet) match(o *message.Origin, hostname string) *quota {
	var hv *hostView
	if rs.local != nil {
		if hv = rs.localView(hostname); hv != nil && hv.full == 0 {
			return rs.quotas[0]
		}
	}
	var f sources.TagFilter
	if o != nil && o.LogSource != nil {
		f, _ = o.LogSource.TagFilter()
	}
	if snap, msgTags, service, ok := o.TagSnapshot(); ok {
		if i := rs.planFor(snap, f, hv != nil).match(msgTags, service); i < len(rs.quotas) {
			return rs.quotas[i]
		}
		return nil
	}
	if hv != nil {
		for i, tags := range hv.residual {
			if hasTags(o, tags) && retainedBy(f, tags) {
				return rs.quotas[i]
			}
		}
		return nil
	}
	for _, q := range rs.quotas {
		if hasTags(o, q.tags) && retainedBy(f, q.tags) {
			return q
		}
	}
	return nil
}
