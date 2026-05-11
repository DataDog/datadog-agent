// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import "strings"

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

// fuzzyDenylist contains real config keys that sit close to one of our
// targets but must never be promoted to it. Without this a customer who set
// `app_key` (one edit from `api_key`) would have it surfaced as api_key.
var fuzzyDenylist = map[string]bool{
	"app_key":              true,
	"api_keys":             true,
	"api_key_password":     true,
	"logs_no_ssl":          true,
	"debugger_api_key":     true,
	"symdb_api_key":        true,
	"additional_endpoints": true,
	"non_local_traffic":    true,
	"dd_url_secure":        true,
}

// applyFuzzy walks the file line by line and, for each top-level key, picks
// the closest unresolved target within its allowed distance. Ambiguous ties
// match neither.
func applyFuzzy(cfg *LiteConfig, raw []byte) {
	for line := range strings.SplitSeq(string(raw), "\n") {
		key, value, ok := parseLine(line)
		if !ok {
			continue
		}
		lowerKey := strings.ToLower(key)
		if fuzzyDenylist[lowerKey] {
			continue
		}

		idx := bestFuzzyMatch(lowerKey, cfg)
		if idx < 0 {
			continue
		}
		f := fuzzyKeys[idx].field(cfg)
		if f.resolved() {
			continue
		}

		val := cleanValue(value)
		if val == "" {
			continue
		}
		f.Value = val
		f.Source = SourceFileFuzzy
		f.MatchedKey = key
	}
}

// bestFuzzyMatch returns the index of the closest unresolved target within
// its allowed distance, or -1 for no acceptable match.
func bestFuzzyMatch(candidate string, cfg *LiteConfig) int {
	strippedCand := stripSeparators(candidate)

	bestIdx := -1
	bestDist := -1
	ambiguous := false

	for i, k := range fuzzyKeys {
		if k.field(cfg).resolved() {
			continue
		}

		d := damerauLevenshtein(candidate, k.name)
		if normalised := damerauLevenshtein(strippedCand, stripSeparators(k.name)); normalised < d {
			d = normalised
		}
		if d > k.maxDist {
			continue
		}

		switch {
		case bestDist == -1 || d < bestDist:
			bestIdx = i
			bestDist = d
			ambiguous = false
		case d == bestDist:
			ambiguous = true
		}
	}

	if ambiguous {
		return -1
	}
	return bestIdx
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
