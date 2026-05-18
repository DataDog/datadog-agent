// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package strings

import (
	"slices"
	"sort"
	"strings"
)

// Matcher test a string for match against a list of strings.
// See `NewMatcher` for details.
type Matcher struct {
	data        []string
	matchPrefix bool
}

// NewMatcher creates a new strings matcher.
// Use `matchPrefix` to  create a prefixes matcher.
func NewMatcher(data []string, matchPrefix bool) Matcher {
	data = slices.Clone(data)
	sort.Strings(data)

	if matchPrefix && len(data) > 0 {
		// Make sure that elements identify unique prefixes.
		i := 0
		for j := 1; j < len(data); j++ {
			if strings.HasPrefix(data[j], data[i]) {
				continue
			}
			i++
			data[i] = data[j]
		}

		data = data[:i+1]
	}

	// Invariants for data:
	// For all i, j such that i < j, data[i] < data[j].
	// for all i, j such that i != j, !HasPrefix(data[i], data[j]).
	return Matcher{
		data:        data,
		matchPrefix: matchPrefix,
	}
}

// Test returns true if the given string matches one in the matcher list.
// or is matching by prefix if the matcher has been created with `matchPrefix`.
func (m *Matcher) Test(name string) bool {
	if m == nil {
		return false
	}

	if len(m.data) == 0 {
		return false
	}

	i := sort.SearchStrings(m.data, name)

	// SearchStrings returns an index such that either:
	// - data[i] == name
	// - data[i-1] < name (if i > 0) && data[i] > name (if i < len(m.data))
	//
	// If for some j, data[j] is a prefix of name, then:
	//
	// - j < i, because any prefix of a string is less than string itself,
	//
	// - if j < i - 1, then strings in range [j+1, i-1] would have
	// data[j] as a prefix, which is impossible by construction of
	// data.
	//
	// Thus j must be i - 1.
	if m.matchPrefix && i > 0 && strings.HasPrefix(name, m.data[i-1]) {
		return true
	}
	if i < len(m.data) {
		return name == m.data[i]
	}

	return false
}

// IsConfigured reports whether the matcher has patterns to match against.
func (m *Matcher) IsConfigured() bool {
	return m != nil && len(m.data) > 0
}

// ShouldDropMetric reports whether a metric name should be dropped at flush time.
// metricBlockList drops matching names; metricAllowList drops names that do not match when configured.
// A nil matcher means that list is disabled.
func ShouldDropMetric(name string, metricBlockList, metricAllowList *Matcher) bool {
	if metricBlockList != nil && metricBlockList.IsConfigured() && metricBlockList.Test(name) {
		return true
	}
	if metricAllowList != nil && metricAllowList.IsConfigured() && !metricAllowList.Test(name) {
		return true
	}
	return false
}
