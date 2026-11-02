// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quota

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// plan is the result of matching one rule set against one tag snapshot.
type plan struct {
	rs *ruleSet
	// full is the index of the first quota the snapshot satisfies alone, or len(rs.quotas).
	full int
	// rest[i] lists the tags quota i still needs from the message, for i < full.
	rest    [][]string
	anchors map[string][]int
}

func newPlan(rs *ruleSet, snap *message.TagSnapshot) *plan {
	have := map[string]struct{}{}
	for t := range snap.All() {
		have[t] = struct{}{}
	}
	p := &plan{rs: rs, full: len(rs.quotas), anchors: map[string][]int{}}
	var flat []string
	for i, q := range rs.quotas {
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
	}
	return p
}

// match returns the index of the first quota matched by the snapshot plus msgTags, or len(rs.quotas).
func (p *plan) match(msgTags []string) int {
	best := p.full
	if len(p.anchors) == 0 {
		return best
	}
	for _, t := range msgTags {
		for _, i := range p.anchors[t] {
			if i >= best {
				break
			}
			if containsAll(msgTags, p.rest[i][1:]) {
				best = i
				break
			}
		}
	}
	return best
}

func containsAll(have, want []string) bool {
	for _, t := range want {
		if !slices.Contains(have, t) {
			return false
		}
	}
	return true
}

func (rs *ruleSet) planFor(snap *message.TagSnapshot) *plan {
	if p, _ := snap.Cached().(*plan); p != nil && p.rs == rs {
		return p
	}
	p := newPlan(rs, snap)
	snap.SetCached(p)
	return p
}

func (rs *ruleSet) match(o *message.Origin) *quota {
	if snap, msgTags, ok := o.TagSnapshot(); ok {
		if i := rs.planFor(snap).match(msgTags); i < len(rs.quotas) {
			return rs.quotas[i]
		}
		return nil
	}
	for _, q := range rs.quotas {
		if q.matches(o) {
			return q
		}
	}
	return nil
}
