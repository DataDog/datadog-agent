// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lite

import "strings"

// fuzzyTarget pairs a configKey with the field it should populate.
type fuzzyTarget struct {
	configKey
	field *ConfigField
}

// denylist contains real config keys that are close to our targets but must not match.
var denylist = map[string]bool{
	"app_key": true, // distance 1 from api_key
}

// damerauLevenshtein computes the Damerau-Levenshtein distance between two strings,
// handling insertions, deletions, substitutions, and adjacent transpositions.
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
			curr[j] = min(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				curr[j] = min(curr[j], prev2[j-2]+cost) // transposition
			}
		}
		prev2, prev, curr = prev, curr, prev2
	}
	return prev[lb]
}

// fuzzyMatch returns the index of the best matching target.
// Returns -1 if no match is within threshold or if ambiguous (equidistant to two targets).
func fuzzyMatch(candidate string, targets []fuzzyTarget) int {
	bestIdx := -1
	bestDist := -1
	ambiguous := false
	strippedCandidate := stripSeparators(candidate)

	for i, t := range targets {
		dist := damerauLevenshtein(candidate, t.name)
		if normDist := damerauLevenshtein(strippedCandidate, t.strippedName); normDist < dist {
			dist = normDist
		}

		if dist > t.maxFuzzyDist {
			continue
		}

		if bestDist == -1 || dist < bestDist {
			bestIdx = i
			bestDist = dist
			ambiguous = false
		} else if dist == bestDist {
			ambiguous = true
		}
	}

	if ambiguous {
		return -1
	}
	return bestIdx
}

// fuzzyExtract scans file content line-by-line and uses fuzzy matching to
// resolve config fields that were not found by exact regex matching.
func fuzzyExtract(fields []*ConfigField, content string) {
	var unresolved []fuzzyTarget
	for i, ck := range configKeys {
		if fields[i].Source == SourceNone {
			unresolved = append(unresolved, fuzzyTarget{ck, fields[i]})
		}
	}
	if len(unresolved) == 0 {
		return
	}

	for line := range strings.SplitSeq(content, "\n") {
		key, value, ok := parseLine(line)
		if !ok {
			continue
		}

		lowerKey := strings.ToLower(key)
		if denylist[lowerKey] {
			continue
		}

		idx := fuzzyMatch(lowerKey, unresolved)
		if idx < 0 {
			continue
		}

		v := cleanValue(value)
		if v == "" {
			continue
		}
		unresolved[idx].field.Value = v
		unresolved[idx].field.Source = SourceFile
		unresolved[idx].field.MatchedKey = key
		unresolved = append(unresolved[:idx], unresolved[idx+1:]...)

		if len(unresolved) == 0 {
			return
		}
	}
}
