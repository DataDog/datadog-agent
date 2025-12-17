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
	tags        map[string]TagMatcher
}

type TagMatcher struct {
	Tags []string `yaml:"tags"`
	// If Negated == false, we keep only the provided tags (allow list).
	// If Negated == true, we strip the provided tags (deny list).
	Negated bool `yaml:"tags_negated"`
}

// NewMatcher creates a new strings matcher.
// Use `matchPrefix` to  create a prefixes matcher.
func NewMatcher(data []string, matchPrefix bool, tags map[string]TagMatcher) Matcher {
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

	// Ensure all tags are sorted
	for _, v := range tags {
		sort.Strings(v.Tags)
	}

	// Invariants for data:
	// For all i, j such that i < j, data[i] < data[j].
	// for all i, j such that i != j, !HasPrefix(data[i], data[j]).
	return Matcher{
		data:        data,
		matchPrefix: matchPrefix,
		tags:        tags,
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

// ShouldStripTags returns true if it has been configured to strip tags
// from the given metric name.
func (m *Matcher) ShouldStripTags(name string) (TagMatcher, bool) {
	tags, ok := m.tags[name]
	return tags, ok
}

// KeepTag will return true if the given tagname should be kept.
func (tm TagMatcher) KeepTag(tag string) bool {
	return slices.Contains(tm.Tags, tag) != tm.Negated
}
