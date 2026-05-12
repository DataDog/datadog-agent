// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"sort"
	"strings"
)

// fuzzyKeys defines the Tier-5 fuzzy targets. maxDist is the Damerau-
// Levenshtein cutoff: we accept "apikey" (distance 1 from "api_key") but not
// keys that diverge by more.
var fuzzyKeys = []struct {
	field   func(*LiteConfig) *ConfigField
	name    string
	maxDist int
}{
	{func(c *LiteConfig) *ConfigField { return &c.APIKey }, "api_key", 2},
	{func(c *LiteConfig) *ConfigField { return &c.Site }, "site", 1},
	{func(c *LiteConfig) *ConfigField { return &c.DDURL }, "dd_url", 2},
}

// applyFuzzy walks every top-level key in raw and, for each unresolved target,
// collects all candidates within maxDist sorted by distance ascending. The
// best becomes the field's primary value; remaining candidates for api_key
// are stashed on cfg.APIKeyCandidates so the rescue path can retry on 401
// (e.g. when `app_key` and a typo'd `api_kye` are both distance 1).
func applyFuzzy(cfg *LiteConfig, raw []byte) {
	for i, k := range fuzzyKeys {
		f := k.field(cfg)
		if f.resolved() {
			continue
		}
		cands := fuzzyCandidatesFor(raw, k.name, k.maxDist)
		if len(cands) == 0 {
			continue
		}
		*f = cands[0]
		if i == 0 && len(cands) > 1 { // api_key only — extras feed rescue retries
			cfg.APIKeyCandidates = append(cfg.APIKeyCandidates, cands[1:]...)
		}
	}
}

// fuzzyCandidatesFor returns every top-level key within maxDist of target,
// sorted by distance ascending (stable on file order for ties).
func fuzzyCandidatesFor(raw []byte, target string, maxDist int) []ConfigField {
	targetStripped := stripSeparators(target)
	type scored struct {
		cand ConfigField
		dist int
	}
	var matches []scored
	for line := range strings.SplitSeq(string(raw), "\n") {
		key, value, ok := parseLine(line)
		if !ok {
			continue
		}
		lowerKey := strings.ToLower(key)
		d := damerauLevenshtein(lowerKey, target)
		if norm := damerauLevenshtein(stripSeparators(lowerKey), targetStripped); norm < d {
			d = norm
		}
		if d > maxDist {
			continue
		}
		val := cleanValue(value)
		if val == "" {
			continue
		}
		matches = append(matches, scored{
			cand: ConfigField{Value: val, Source: SourceFileFuzzy, MatchedKey: key},
			dist: d,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].dist < matches[j].dist })
	out := make([]ConfigField, len(matches))
	for i, m := range matches {
		out[i] = m.cand
	}
	return out
}

// damerauLevenshtein is the Damerau-Levenshtein edit distance (insertions,
// deletions, substitutions and adjacent transpositions) between two ASCII
// strings. Three rolling rows; allocation is O(n).
func damerauLevenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev2 := make([]int, lb+1)
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				curr[j] = min(curr[j], prev2[j-2]+cost)
			}
		}
		prev2, prev, curr = prev, curr, prev2
	}
	return prev[lb]
}
