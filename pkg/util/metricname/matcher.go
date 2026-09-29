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

// Matcher tests a metric name for match against a list of metric names and/or
// prefix rules. See `NewMatcher` and `NewMatcherWithPrefixRules` for details.
type Matcher struct {
	// exact contains the entries matched by equality.
	exact []string
	// prefixes contains the entries matched by prefix with no exceptions.
	prefixes []string
	// rules contains prefix rules that still carry at least one exception
	// after compaction. Unlike `prefixes`, entries here are not compacted
	// against each other: two rules whose prefixes nest can each still
	// independently match, because their exceptions differ.
	rules []compiledPrefixRule
}

// PrefixRule is a single metric_filterlist_prefix entry: it matches every
// metric name starting with Prefix, except a name matched by one of
// ExceptExact (exact names) or ExceptPrefix (prefixes).
//
// Prefix and every entry of ExceptPrefix are prefixes; every entry of
// ExceptExact is a complete metric name. None of them ever carry a `*`
// marker: unlike a plain `metric_filterlist` entry, a PrefixRule field is
// always what its name says it is, `*` included, since a literal `*` can
// never appear in (and therefore never match) a normalized metric name.
//
// An empty Prefix matches every metric name, exactly like an empty entry of a
// matchPrefix-enabled plain list (see NewMatcher). The same holds for an
// empty entry of ExceptPrefix: it excepts every metric name, neutering the
// rule entirely. An empty entry of ExceptExact, by contrast, can never except
// anything, since no metric name the intake stores is ever empty.
//
// PrefixRule values passed to NewMatcherWithPrefixRules are expected to
// already be normalized; see NormalizePrefixRules.
type PrefixRule struct {
	Prefix       string
	ExceptExact  []string
	ExceptPrefix []string
}

// compiledPrefixRule is the compiled form of a PrefixRule kept in
// Matcher.rules: a rule that still has at least one exception after
// compaction.
type compiledPrefixRule struct {
	prefix string
	// exceptExact is sorted and deduplicated.
	exceptExact []string
	// exceptPrefix is sorted and identifies unique prefixes (see
	// compactPrefixes).
	exceptPrefix []string
}

// NewMatcher creates a new metric name matcher from a plain list of entries,
// with no prefix-rule (metric_filterlist_prefix) entries. It is equivalent to
// discarding the second return value of `NewMatcherWithPrefixRules(data,
// matchPrefix, nil)`.
//
// Entries are taken verbatim: they are expected to already be normalized,
// i.e. to be metric names as the backend stores and displays them, which is
// what users copy into a filter list. `Test` normalizes the name it is
// given, so the comparison happens in that same name space.
//
// Use `matchPrefix` to treat every entry as a prefix. There is no per-entry
// way to mark a single `metric_filterlist` entry as a prefix: that is what
// `metric_filterlist_prefix` (see `PrefixRule`) is for.
func NewMatcher(data []string, matchPrefix bool) Matcher {
	m, _ := NewMatcherWithPrefixRules(data, matchPrefix, nil)
	return m
}

// NewMatcherWithPrefixRules creates a new metric name matcher combining a
// plain list of entries (see NewMatcher) with a list of prefix rules (see
// PrefixRule), typically loaded from `metric_filterlist` (`data`,
// `matchPrefix`) and `metric_filterlist_prefix` (`rules`) respectively.
//
// `rules` must already be normalized; see NormalizePrefixRules.
//
// The second return value lists, for logging purposes, the prefix of every
// rule dropped because a broader, exception-free prefix elsewhere in `data`
// (via `matchPrefix`) or in `rules` already matches every name that rule
// could ever match: such a rule's exceptions can never take effect (see
// Matcher.rules), so keeping it would only cost memory and CPU for no
// behavioral difference.
func NewMatcherWithPrefixRules(data []string, matchPrefix bool, rules []PrefixRule) (Matcher, []string) {
	var exact, prefixes []string

	for _, entry := range data {
		if matchPrefix {
			prefixes = append(prefixes, entry)
			continue
		}
		exact = append(exact, entry)
	}

	var compiled []compiledPrefixRule
	for _, rule := range rules {
		exceptExact := compactExact(rule.ExceptExact, nil)
		exceptPrefix := compactPrefixes(rule.ExceptPrefix)

		if len(exceptExact) == 0 && len(exceptPrefix) == 0 {
			// No exception survives compaction: this rule is a bare prefix.
			prefixes = append(prefixes, rule.Prefix)
			continue
		}

		compiled = append(compiled, compiledPrefixRule{
			prefix:       rule.Prefix,
			exceptExact:  exceptExact,
			exceptPrefix: exceptPrefix,
		})
	}

	prefixes = compactPrefixes(prefixes)
	exact = compactExact(exact, prefixes)

	// Drop any exception-bearing rule whose prefix is already covered,
	// unconditionally, by an entry of `prefixes`: such a rule can never keep
	// any of its exceptions in effect (see Matcher.rules).
	var dropped []string
	var kept []compiledPrefixRule
	for _, rule := range compiled {
		if len(prefixes) > 0 && testPrefixes(prefixes, rule.prefix) {
			dropped = append(dropped, rule.prefix)
			continue
		}
		kept = append(kept, rule)
	}
	compiled = kept

	return Matcher{
		exact:    exact,
		prefixes: prefixes,
		rules:    compiled,
	}, dropped
}

// NormalizeEntries returns `entries` normalized into the name space `Matcher`
// compares in, along with the entries that were dropped because no metric name
// the intake stores can match them (see `NormalizeAppend` and
// `NormalizePrefixAppend` for what that rules out).
//
// Entries are the raw strings a user or Remote Config provides. `matchPrefix`
// normalizes every entry as a prefix rather than as a complete metric name;
// there is no per-entry marker: a `metric_filterlist` entry is always either
// entirely exact or entirely a prefix, for the whole list, controlled by
// `matchPrefix`. A prefix rule with its own, independent, per-entry prefix
// behavior belongs in `metric_filterlist_prefix` (see `PrefixRule` and
// `NormalizePrefixRules`), not here.
//
// Dropped entries are returned rather than logged: this package has no logger,
// and the caller knows which setting they came from.
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
// `prefixes` and `rules` — a derived matcher that must still match every
// prefix (bare or with exceptions) this one matches (e.g. a
// histogram-aggregate name derived from a metric matched by prefix) has
// nothing new to compute there, so both slices are shared rather than
// rebuilt — restricted to the exact entries for which `keep` returns true.
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
		rules:    m.rules,
	}
}

// Len returns the number of entries in the compiled matcher: exact entries,
// bare prefixes, and prefix rules that still carry at least one exception.
func (m *Matcher) Len() int {
	if m == nil {
		return 0
	}
	return len(m.exact) + len(m.prefixes) + len(m.rules)
}

// MatchesAll reports whether the matcher matches every metric name the intake
// stores, which is what an entry of an empty prefix (or of a lone `*` in a
// pre-#55504 sense — see NewMatcher) asks for. Compaction leaves that empty
// prefix as the only entry, since every name starts with it. An
// exception-bearing rule of an empty prefix does not make this true, since by
// definition at least one name (one of its exceptions) is kept: such a rule
// never folds into `prefixes` and is therefore never counted here, correctly.
func (m *Matcher) MatchesAll() bool {
	if m == nil {
		return false
	}
	return len(m.prefixes) == 1 && m.prefixes[0] == ""
}

// Test returns true if the given metric name is equal to one of the exact
// entries of the matcher, starts with one of its bare prefix entries, or
// starts with the prefix of one of its prefix rules without being excepted by
// that rule.
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
		if i < len(m.exact) && name == m.exact[i] {
			return true
		}
	}

	for i := range m.rules {
		if matchesPrefixRule(&m.rules[i], name) {
			return true
		}
	}

	return false
}

// matchesPrefixRule reports whether `name` is matched by `rule`: it starts
// with `rule.prefix` and is not covered by one of its exceptions. `name` must
// already be normalized, exactly as required by Matcher.search.
func matchesPrefixRule(rule *compiledPrefixRule, name string) bool {
	if !strings.HasPrefix(name, rule.prefix) {
		return false
	}

	if len(rule.exceptPrefix) > 0 && testPrefixes(rule.exceptPrefix, name) {
		return false
	}

	if len(rule.exceptExact) > 0 {
		i := sort.SearchStrings(rule.exceptExact, name)
		if i < len(rule.exceptExact) && name == rule.exceptExact[i] {
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
