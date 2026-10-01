// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"sort"
	"strings"
	"unsafe"
)

// Matcher tests metric names against exact entries and prefix rules.
type Matcher struct {
	// sorted, deduplicated, and not covered by prefixes.
	exact []string
	// metric_filterlist prefixes; sorted, deduplicated, and compacted.
	prefixes []string
	// metric_filterlist_prefix prefixes; sorted, deduplicated, and compacted.
	rulePrefixes []string
	// global exceptions for metric_filterlist_prefix.
	exceptExact  []string
	exceptPrefix []string
}

// PrefixRule is a metric_filterlist_prefix entry.
// Prefix and ExceptPrefix are prefixes; ExceptExact is a full metric name.
// Empty Prefix matches all names, and empty ExceptPrefix excepts all names.
// NewMatcherWithPrefixRules expects normalized rules.
type PrefixRule struct {
	Prefix       string
	ExceptExact  []string
	ExceptPrefix []string
}

// NewMatcher creates a matcher for metric_filterlist only.
// Entries must already be normalized. matchPrefix applies to the whole list.
func NewMatcher(data []string, matchPrefix bool) Matcher {
	return NewMatcherWithPrefixRules(data, matchPrefix, nil)
}

// NewMatcherWithPrefixRules combines metric_filterlist and metric_filterlist_prefix.
// rules must already be normalized.
func NewMatcherWithPrefixRules(data []string, matchPrefix bool, rules []PrefixRule) Matcher {
	var exact, prefixes []string

	for _, entry := range data {
		if matchPrefix {
			prefixes = append(prefixes, entry)
			continue
		}
		exact = append(exact, entry)
	}

	var rulePrefixes []string
	var exceptExact []string
	var exceptPrefix []string
	for _, rule := range rules {
		rulePrefixes = append(rulePrefixes, rule.Prefix)
		exceptExact = append(exceptExact, rule.ExceptExact...)
		exceptPrefix = append(exceptPrefix, rule.ExceptPrefix...)
	}

	prefixes = compactPrefixes(prefixes)
	exact = compactExact(exact, prefixes)
	exceptPrefix = compactPrefixes(exceptPrefix)
	exceptExact = compactExact(exceptExact, exceptPrefix)

	rulePrefixes = compactPrefixes(rulePrefixes)

	return Matcher{
		exact:        exact,
		prefixes:     prefixes,
		rulePrefixes: rulePrefixes,
		exceptExact:  exceptExact,
		exceptPrefix: exceptPrefix,
	}
}

// NormalizeEntries normalizes raw metric_filterlist entries.
// matchPrefix applies to the whole list; per-entry prefixes belong in
// metric_filterlist_prefix. Dropped entries cannot match stored metric names.
func NormalizeEntries(entries []string, matchPrefix bool) (normalized, dropped []string) {
	normalized = make([]string, 0, len(entries))
	// Reuse this stack buffer to normalize every entry.
	var buf [MaxLength]byte

	for _, entry := range entries {
		var key []byte
		var ok bool
		if matchPrefix {
			key, ok = NormalizePrefixAppend(buf[:0], entry)
		} else {
			key, ok = NormalizeAppend(buf[:0], entry)
		}
		if !ok {
			dropped = append(dropped, entry)
			continue
		}

		normalized = append(normalized, string(key))
	}

	return normalized, dropped
}

// compactPrefixes sorts `prefixes` and removes the entries identifying a
// prefix already identified by a shorter entry, so that elements identify
// unique prefixes.
//
// `prefixes` is reordered and compacted in place: it must be a slice the caller
// owns, which is the case for the one `NewMatcherWithPrefixRules` builds by
// appending.
func compactPrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return nil
	}

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
//
// As in `compactPrefixes`, `exact` is reordered and compacted in place.
func compactExact(exact, prefixes []string) []string {
	if len(exact) == 0 {
		return nil
	}

	sort.Strings(exact)

	i := 0
	for _, name := range exact {
		if i > 0 && name == exact[i-1] {
			continue
		}
		if len(prefixes) > 0 && testPrefixes(prefixes, name) {
			continue
		}
		exact[i] = name
		i++
	}
	if i == 0 {
		return nil
	}

	return exact[:i]
}

// RestrictExact filters exact entries and shares prefix state.
// Histogram aggregate matchers must keep prefix behavior without recompiling it.
func (m Matcher) RestrictExact(keep func(string) bool) Matcher {
	var exact []string
	for _, e := range m.exact {
		if keep(e) {
			exact = append(exact, e)
		}
	}
	return Matcher{
		exact:        exact,
		prefixes:     m.prefixes,
		rulePrefixes: m.rulePrefixes,
		exceptExact:  m.exceptExact,
		exceptPrefix: m.exceptPrefix,
	}
}

// Len returns the number of compiled entries.
func (m *Matcher) Len() int {
	if m == nil {
		return 0
	}
	return len(m.exact) + len(m.prefixes) + len(m.rulePrefixes)
}

// MatchesAll reports whether an unconditional empty prefix matches all names.
func (m *Matcher) MatchesAll() bool {
	if m == nil {
		return false
	}
	if len(m.prefixes) == 1 && m.prefixes[0] == "" {
		return true
	}
	return len(m.rulePrefixes) == 1 && m.rulePrefixes[0] == "" && len(m.exceptExact) == 0 && len(m.exceptPrefix) == 0
}

// Test reports whether name matches an exact entry, metric_filterlist prefix,
// or unexcepted metric_filterlist_prefix entry.
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
	if len(m.exact) > 0 {
		i := sort.SearchStrings(m.exact, name)
		if i < len(m.exact) && name == m.exact[i] {
			return true
		}
	}

	if len(m.prefixes) > 0 && testPrefixes(m.prefixes, name) {
		return true
	}

	if len(m.rulePrefixes) == 0 || !testPrefixes(m.rulePrefixes, name) {
		return false
	}

	if len(m.exceptPrefix) > 0 && testPrefixes(m.exceptPrefix, name) {
		return false
	}

	if len(m.exceptExact) > 0 {
		i := sort.SearchStrings(m.exceptExact, name)
		if i < len(m.exceptExact) && name == m.exceptExact[i] {
			return false
		}
	}

	return true
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
