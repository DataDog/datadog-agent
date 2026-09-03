// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"slices"
	"sort"
	"strings"
	"unsafe"
)

// PrefixSuffix marks an entry in a matcher list as a prefix pattern: an entry
// ending with it matches every metric name starting with the rest of the entry.
const PrefixSuffix = "*"

// Matcher tests a metric name for match against a list of metric names.
// See `NewMatcher` for details.
type Matcher struct {
	// exact contains the entries matched by equality.
	// Invariants:
	// - sorted and deduplicated,
	// - no entry has an element of `prefixes` as a prefix.
	exact []string
	// prefixes contains the entries matched by prefix, without their
	// trailing `PrefixSuffix`.
	// Invariants:
	// - sorted and deduplicated,
	// - for all i, j such that i != j, !HasPrefix(prefixes[i], prefixes[j]).
	prefixes []string
}

// NewMatcher creates a new metric name matcher.
//
// Entries are taken verbatim, apart from the trailing `*` described below. They
// are expected to already be normalized, i.e. to be metric names as the backend
// stores and displays them, which is what users copy into a filter list. `Test`
// normalizes the name it is given, so the comparison happens in that same name
// space.
//
// An entry ending with `*` is a prefix pattern: `foo.*` matches every metric
// name starting with `foo.` (including `foo.` itself). A `*` anywhere else in an
// entry is matched literally, and can therefore never match, since a normalized
// name contains no `*`.
//
// Use `matchPrefix` to treat every entry as a prefix, whether or not it ends
// with `*`. A trailing `*` is always stripped, so an entry written as a prefix
// pattern behaves the same regardless of `matchPrefix`.
func NewMatcher(data []string, matchPrefix bool) Matcher {
	var exact, prefixes []string

	for _, entry := range data {
		if prefix, ok := strings.CutSuffix(entry, PrefixSuffix); ok {
			// Sub-slicing shares the backing bytes of `entry`: stripping the
			// trailing `*` does not allocate.
			prefixes = append(prefixes, prefix)
			continue
		}
		if matchPrefix {
			prefixes = append(prefixes, entry)
			continue
		}
		exact = append(exact, entry)
	}

	prefixes = compactPrefixes(prefixes)
	exact = compactExact(exact, prefixes)

	return Matcher{
		exact:    exact,
		prefixes: prefixes,
	}
}

// compactPrefixes sorts `prefixes` and removes the entries identifying a
// prefix already identified by a shorter entry, so that elements identify
// unique prefixes.
func compactPrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return nil
	}

	prefixes = slices.Clone(prefixes)
	sort.Strings(prefixes)

	i := 0
	for j := 1; j < len(prefixes); j++ {
		// Sorting guarantees that any entry having prefixes[i] as a prefix
		// comes right after it, so keeping the last retained entry is enough.
		if strings.HasPrefix(prefixes[j], prefixes[i]) {
			continue
		}
		i++
		prefixes[i] = prefixes[j]
	}

	return prefixes[:i+1]
}

// compactExact sorts `exact`, deduplicates it and removes the entries already
// matched by one of `prefixes`, which must already be compacted.
func compactExact(exact, prefixes []string) []string {
	if len(exact) == 0 {
		return nil
	}

	exact = slices.Clone(exact)
	sort.Strings(exact)
	exact = slices.Compact(exact)

	if len(prefixes) == 0 {
		return exact
	}

	exact = slices.DeleteFunc(exact, func(name string) bool {
		return testPrefixes(prefixes, name)
	})
	if len(exact) == 0 {
		return nil
	}

	return exact
}

// RestrictExact returns a Matcher that shares this Matcher's compiled
// `prefixes` — a derived matcher that must still match every prefix this one
// matches (e.g. a histogram-aggregate name derived from a metric matched by
// prefix) has nothing new to compute there, so the slice is shared rather
// than rebuilt — restricted to the exact entries for which `keep` returns
// true.
//
// m.exact is already sorted, deduplicated, and free of entries covered by a
// prefix; filtering by `keep` preserves all three properties, so the result
// needs no re-sorting or re-compaction against `prefixes`.
func (m Matcher) RestrictExact(keep func(string) bool) Matcher {
	var exact []string
	for _, e := range m.exact {
		if keep(e) {
			exact = append(exact, e)
		}
	}
	return Matcher{
		exact:    exact,
		prefixes: m.prefixes,
	}
}

// Len returns the number of entries in the compiled matcher.
func (m *Matcher) Len() int {
	if m == nil {
		return 0
	}
	return len(m.exact) + len(m.prefixes)
}

// Test returns true if the given metric name is equal to one of the exact
// entries of the matcher, or starts with one of its prefix entries.
//
// The name is normalized before being compared. The Agent sees names exactly as
// they were submitted, but the intake rewrites them on ingest, so a raw name
// such as `my metric-name` is stored (and shown to users, and therefore
// configured in filter lists) as `my_metric_name`. Matching the raw name would
// let those metrics through the filter list and still have them show up in
// Datadog. Names the intake would reject never match.
//
// Test never allocates. Names that are already normalized are compared as
// given, and the rest are normalized into a stack buffer.
func (m *Matcher) Test(name string) bool {
	if m == nil {
		return false
	}

	if m.Len() == 0 {
		return false
	}

	// Fast path: already normalized, so compare the name as given.
	if isNormalized(name) {
		return m.search(name)
	}

	var buf [MaxLength]byte
	key, ok := NormalizeAppend(buf[:0], name)
	if !ok {
		return false
	}

	// Safe: the string aliases buf, search only reads it for comparison and
	// never retains it, and buf is not written again while it is alive.
	return m.search(unsafe.String(unsafe.SliceData(key), len(key)))
}

// search looks name up in the compiled lists. name must already be normalized.
func (m *Matcher) search(name string) bool {
	if len(m.prefixes) > 0 && testPrefixes(m.prefixes, name) {
		return true
	}

	if len(m.exact) > 0 {
		i := sort.SearchStrings(m.exact, name)
		if i < len(m.exact) {
			return name == m.exact[i]
		}
	}

	return false
}

// testPrefixes returns true if `name` starts with one of the entries of
// `prefixes`, which must be sorted and identify unique prefixes.
func testPrefixes(prefixes []string, name string) bool {
	i := sort.SearchStrings(prefixes, name)

	// SearchStrings returns an index such that either:
	// - prefixes[i] == name
	// - prefixes[i-1] < name (if i > 0) && prefixes[i] > name (if i < len)
	//
	// If for some j, prefixes[j] is a strict prefix of name, then:
	//
	// - j < i, because any prefix of a string is less than the string itself,
	//
	// - if j < i - 1, then entries in range [j+1, i-1] would have prefixes[j]
	// as a prefix, which is impossible by construction of prefixes.
	//
	// Thus j must be i - 1, and the only other candidate is prefixes[i] being
	// equal to name.
	if i > 0 && strings.HasPrefix(name, prefixes[i-1]) {
		return true
	}
	return i < len(prefixes) && name == prefixes[i]
}
