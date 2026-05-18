// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"cmp"
	"slices"
	"time"

	lookback "github.com/DataDog/datadog-agent/comp/lookback/def"
)

const defaultIntervalNs = int64(time.Second)

// aggregateRecords filters recs to keySet ∩ [start, stop), then groups by
// contextKey (hash-group sort) and streams them through interval-width bucket
// aggregation. It is the core of the Flush read path.
//
// intervalNs is the bucket width in nanoseconds; ≤0 defaults to 1 second.
// resolve maps a context key to (name, tags, ok).
func aggregateRecords(
	recs []record,
	keySet map[uint64]struct{},
	start, stop int64,
	intervalNs int64,
	resolve func(uint64) (string, []string, bool),
) []lookback.Bucket {
	if intervalNs <= 0 {
		intervalNs = defaultIntervalNs
	}

	// Step 1: filter to keySet and time range.
	filtered := make([]record, 0, len(recs))
	for _, r := range recs {
		if _, ok := keySet[r.contextKey]; !ok {
			continue
		}
		if r.tsNs < start || r.tsNs >= stop {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return nil
	}

	// Step 2: group by contextKey → slice of indices (hash-group sort).
	groups := make(map[uint64][]int, len(keySet))
	for i, r := range filtered {
		groups[r.contextKey] = append(groups[r.contextKey], i)
	}

	// Step 3: sort unique keys.
	keys := make([]uint64, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	// Step 4: per key, sort indices by tsNs, then stream-aggregate.
	var buckets []lookback.Bucket
	for _, ck := range keys {
		name, tags, ok := resolve(ck)
		if !ok {
			continue
		}
		idxs := groups[ck]
		slices.SortFunc(idxs, func(a, b int) int {
			return cmp.Compare(filtered[a].tsNs, filtered[b].tsNs)
		})

		var curTs int64 = -1
		var cur *lookback.Bucket
		for _, i := range idxs {
			r := filtered[i]
			tsBucket := (r.tsNs / intervalNs) * intervalNs
			if tsBucket != curTs {
				buckets = append(buckets, lookback.Bucket{
					Name:  name,
					Tags:  tags,
					Ts:    tsBucket,
					Count: 1,
					Sum:   r.value,
					Min:   r.value,
					Max:   r.value,
				})
				curTs = tsBucket
				cur = &buckets[len(buckets)-1]
			} else {
				cur.Count++
				cur.Sum += r.value
				if r.value < cur.Min {
					cur.Min = r.value
				}
				if r.value > cur.Max {
					cur.Max = r.value
				}
			}
		}
	}
	return buckets
}
