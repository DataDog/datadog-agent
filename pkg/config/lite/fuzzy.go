// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lite

import "strings"

// fuzzyTarget defines a config key to fuzzy-match against.
type fuzzyTarget struct {
	key         string
	maxDistance int
	field       *ConfigField
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

	// d stores 3 rows: d[0]=prev-prev, d[1]=prev, d[2]=current
	d := [3][]int{
		make([]int, lb+1),
		make([]int, lb+1),
		make([]int, lb+1),
	}

	for j := 0; j <= lb; j++ {
		d[1][j] = j
	}

	for i := 1; i <= la; i++ {
		d[2][0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[2][j] = min(
				d[1][j]+1,      // deletion
				d[2][j-1]+1,    // insertion
				d[1][j-1]+cost, // substitution
			)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[2][j] = min(d[2][j], d[0][j-2]+cost) // transposition
			}
		}
		d[0], d[1], d[2] = d[1], d[2], d[0]
	}
	return d[1][lb]
}

// fuzzyMatch returns the index of the best matching target in targets.
// Returns -1 if no match is within threshold or if ambiguous.
func fuzzyMatch(candidate string, targets []fuzzyTarget) int {
	bestIdx := -1
	bestDist := -1
	ambiguous := false

	for i, t := range targets {
		dist := damerauLevenshtein(candidate, t.key)
		// Also try with separators stripped (e.g. "apikey" vs "api_key")
		if normDist := damerauLevenshtein(stripSeparators(candidate), stripSeparators(t.key)); normDist < dist {
			dist = normDist
		}

		if dist > t.maxDistance {
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
func fuzzyExtract(cfg *LiteConfig, content string) {
	targets := []fuzzyTarget{
		{key: "api_key", maxDistance: 2, field: &cfg.APIKey},
		{key: "site", maxDistance: 1, field: &cfg.Site},
		{key: "dd_url", maxDistance: 2, field: &cfg.DDURL},
	}

	var unresolved []fuzzyTarget
	for _, t := range targets {
		if t.field.Source == SourceNone {
			unresolved = append(unresolved, t)
		}
	}
	if len(unresolved) == 0 {
		return
	}

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimRight(line, "\r")

		// Skip empty, comment, or indented lines.
		if len(line) == 0 || line[0] == '#' || line[0] == ' ' || line[0] == '\t' {
			continue
		}

		// Find earliest separator among : ; =
		sepIdx := strings.IndexAny(line, ":;=")
		if sepIdx <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:sepIdx])
		value := strings.TrimSpace(line[sepIdx+1:])

		// Strip trailing inline comments
		if idx := strings.Index(value, " #"); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
		if value == "" {
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
