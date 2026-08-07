// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package noisyneighbor

import "sort"

type watchlistRotator struct {
	generation  uint64
	lastSampled map[uint64]uint64
}

func (r *watchlistRotator) selectNext(live []uint64, limit int) []uint64 {
	if r.lastSampled == nil {
		r.lastSampled = make(map[uint64]uint64)
	}
	unique := make(map[uint64]struct{}, len(live))
	for _, id := range live {
		if id != 0 {
			unique[id] = struct{}{}
		}
	}
	for id := range r.lastSampled {
		if _, ok := unique[id]; !ok {
			delete(r.lastSampled, id)
		}
	}
	ordered := make([]uint64, 0, len(unique))
	for id := range unique {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := r.lastSampled[ordered[i]], r.lastSampled[ordered[j]]
		if left != right {
			return left < right
		}
		return ordered[i] < ordered[j]
	})
	if limit < len(ordered) {
		ordered = ordered[:limit]
	}
	r.generation++
	for _, id := range ordered {
		r.lastSampled[id] = r.generation
	}
	return ordered
}
