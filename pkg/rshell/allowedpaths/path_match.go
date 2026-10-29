// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"path/filepath"
	"strings"
)

// resolvePath is the shared, purely lexical root-selection algorithm. Neither
// containment nor the most-specific grant selection accesses the filesystem.
func resolvePath[T any](roots []T, absPath string, rootPath func(*T) string, preferEqualLengthRoot func(candidate, best *T) bool) (*T, string, bool) {
	var best *T
	var bestRel string
	var bestLen int
	for i := range roots {
		candidate := &roots[i]
		candidatePath := rootPath(candidate)
		rel, err := filepath.Rel(candidatePath, absPath)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		candidateLen := len(candidatePath)
		longerMatch := best == nil || candidateLen > bestLen
		tieMatch := best != nil &&
			candidateLen == bestLen &&
			preferEqualLengthRoot != nil &&
			preferEqualLengthRoot(candidate, best)
		if longerMatch || tieMatch {
			best = candidate
			bestRel = rel
			bestLen = candidateLen
		}
	}
	return best, bestRel, best != nil
}
