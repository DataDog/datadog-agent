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

// Matcher matches metric names against a list of strings.
// See NewBlocklistMatcher and NewAllowlistMatcher.
type Matcher struct {
	data        []string
	matchPrefix bool
	exclusive   bool // allowlist: ShouldDrop is true when name is not on the list
}

// NewBlocklistMatcher creates a blocklist matcher. ShouldDrop is true when name is on the list.
func NewBlocklistMatcher(data []string, matchPrefix bool) Matcher {
	return newMatcher(data, matchPrefix, false)
}

// NewAllowlistMatcher creates an allowlist matcher. ShouldDrop is true when name is not on the list.
func NewAllowlistMatcher(data []string, matchPrefix bool) Matcher {
	return newMatcher(data, matchPrefix, true)
}

func newMatcher(data []string, matchPrefix bool, exclusive bool) Matcher {
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
		exclusive:   exclusive,
	}
}

// ShouldDrop reports whether name should be dropped. For a blocklist, true means name is listed.
// For an allowlist, true means name is not listed. Returns false when the matcher has no patterns.
func (m *Matcher) ShouldDrop(name string) bool {
	if !m.isConfigured() {
		return false
	}
	matched := m.matches(name)
	if m.exclusive {
		return !matched
	}
	return matched
}

func (m *Matcher) matches(name string) bool {
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

func (m *Matcher) isConfigured() bool {
	return m != nil && len(m.data) > 0
}
