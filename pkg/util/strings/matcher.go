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

// PrefixSuffix marks an entry in a matcher list as a prefix pattern: an entry
// ending with it matches every string starting with the rest of the entry.
const PrefixSuffix = "*"

// Rule is one entry of a matcher list: a pattern, plus the exceptions that
// override it.
type Rule struct {
	// Pattern is matched by equality, unless it ends with `PrefixSuffix` (or
	// the matcher is built with `matchPrefix`), in which case it is matched as
	// a prefix.
	Pattern string
	// Except holds the patterns that cancel this rule: a string matching
	// `Pattern` is matched by this rule only if none of `Except` matches it.
	// Entries follow the same exact/prefix convention as `Pattern`, and cannot
	// themselves carry exceptions.
	//
	// Exceptions are scoped to the rule they belong to: they narrow this rule
	// only. Another rule of the same list matching the same string still
	// matches it, so `foo.*` excepting `foo.keep` sitting next to a plain
	// `foo.keep` entry still matches `foo.keep`.
	Except []string
}

// Matcher test a string for match against a list of strings.
// See `NewMatcher` for details.
type Matcher struct {
	// exact contains the entries matched by equality.
	// Invariants:
	// - sorted and deduplicated,
	// - no entry has an element of `prefixes` as a prefix.
	exact []string
	// prefixes contains the entries matched by prefix and carrying no
	// exception, without their trailing `PrefixSuffix`.
	// Invariants:
	// - sorted and deduplicated,
	// - for all i, j such that i != j, !HasPrefix(prefixes[i], prefixes[j]).
	prefixes []string
	// guarded contains the prefixes of the rules carrying exceptions, without
	// their trailing `PrefixSuffix`. It holds the same invariants as
	// `prefixes`, so that at most one entry can be a prefix of a given string:
	// rules written on a longer prefix are held by that entry's `guard`, not
	// here. See `buildGuarded`.
	// Invariants:
	// - sorted and deduplicated,
	// - for all i, j such that i != j, !HasPrefix(guarded[i], guarded[j]),
	// - no entry has an element of `prefixes` as a prefix.
	guarded []string
	// guards[i] qualifies guarded[i]. It is a parallel array rather than a
	// field of a `guarded` struct so that the binary search of `Test` only
	// walks a dense slice of strings; a guard is read at most once, after its
	// prefix matched.
	// Invariant: len(guards) == len(guarded).
	guards []guard
}

// guard holds the exceptions attached to an entry of `Matcher.guarded`.
type guard struct {
	// except holds the exceptions of each rule written on this very prefix.
	// The prefix is cancelled for a string only when every one of them matches
	// it: a rule whose own exceptions do not match still matches the string.
	// Invariant: non-empty, and no element covers the whole guarded prefix.
	except []Matcher
	// nested holds the rules written on longer prefixes, which narrow this
	// entry further. It only ever has `guarded` entries.
	nested Matcher
}

// NewMatcher creates a new strings matcher.
//
// An entry ending with `*` is a prefix pattern: `foo.*` matches every string
// starting with `foo.` (including `foo.` itself). A `*` anywhere else in an
// entry is matched literally; there is no way to express an entry matching a
// literal trailing `*`.
//
// Use `matchPrefix` to treat every entry as a prefix, whether or not it ends
// with `*`. A trailing `*` is always stripped, so an entry written as a prefix
// pattern behaves the same regardless of `matchPrefix`.
//
// Use `NewRuleMatcher` for entries carrying exceptions.
func NewMatcher(data []string, matchPrefix bool) Matcher {
	var b matcherBuilder
	for _, entry := range data {
		b.add(entry, nil, matchPrefix)
	}
	return b.build()
}

// NewRuleMatcher creates a new strings matcher from entries that can each carry
// exceptions. Patterns follow the same convention as `NewMatcher`.
//
// A string is matched when at least one rule matches it and that rule's own
// exceptions do not: exceptions never cross rule boundaries.
func NewRuleMatcher(rules []Rule, matchPrefix bool) Matcher {
	var b matcherBuilder
	for _, rule := range rules {
		b.add(rule.Pattern, rule.Except, matchPrefix)
	}
	return b.build()
}

// matcherBuilder accumulates the rules of a matcher before they are sorted and
// compacted into it.
type matcherBuilder struct {
	exact    []string
	prefixes []string
	guarded  []guardedRule
}

// guardedRule is a prefix rule carrying a non-empty set of exceptions.
type guardedRule struct {
	prefix string
	except Matcher
}

func (b *matcherBuilder) add(pattern string, except []string, matchPrefix bool) {
	// Sub-slicing shares the backing bytes of `pattern`: stripping the
	// trailing `*` does not allocate.
	entry, isPrefix := strings.CutSuffix(pattern, PrefixSuffix)

	// Exceptions are plain patterns, so they compile to a matcher with no
	// exception of its own. Building one from an empty list allocates nothing.
	exceptions := NewMatcher(except, false)

	if !isPrefix && !matchPrefix {
		// Exceptions on an exact entry can only cancel it outright, in which
		// case it can never match.
		if exceptions.Test(entry) {
			return
		}
		b.exact = append(b.exact, entry)
		return
	}

	if exceptions.Len() == 0 {
		b.prefixes = append(b.prefixes, entry)
		return
	}

	// An exception covering the whole prefix cancels the rule for every string
	// it could match.
	if exceptions.coversAll(entry) {
		return
	}

	b.guarded = append(b.guarded, guardedRule{prefix: entry, except: exceptions})
}

func (b *matcherBuilder) build() Matcher {
	prefixes := compactPrefixes(b.prefixes)
	guarded, guards := compactGuarded(b.guarded, prefixes)

	return Matcher{
		// Only an exception-free prefix can absorb an exact entry: a guarded
		// one may be cancelled for that very entry.
		exact:    compactExact(b.exact, prefixes),
		prefixes: prefixes,
		guarded:  guarded,
		guards:   guards,
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

// compactGuarded turns the prefix rules carrying exceptions into the parallel
// arrays a `Matcher` stores, dropping the rules already matched
// unconditionally by one of `prefixes`, which must be compacted. A rule
// covered by an exception-free prefix is dead: that prefix matches everything
// below it whatever the longer rule's exceptions say.
//
// Rules on the same prefix are merged into a single entry holding all of their
// exceptions, and rules on a longer prefix are pushed into that entry's nested
// matcher, so that the result identifies unique prefixes like `prefixes` does.
// Neither can be flattened away: applying a rule's exceptions to the strings
// another rule matches unconditionally would let strings through that must be
// matched.
func compactGuarded(rules []guardedRule, prefixes []string) ([]string, []guard) {
	rules = slices.DeleteFunc(rules, func(rule guardedRule) bool {
		return testPrefixes(prefixes, rule.prefix)
	})
	if len(rules) == 0 {
		return nil, nil
	}

	slices.SortFunc(rules, func(a, b guardedRule) int {
		return strings.Compare(a.prefix, b.prefix)
	})

	return buildGuarded(rules)
}

// buildGuarded returns the entries of `rules` that no other entry is a prefix
// of, each holding the entries below it in its nested matcher. `rules` must be
// sorted by prefix.
func buildGuarded(rules []guardedRule) ([]string, []guard) {
	var guarded []string
	var guards []guard

	for i := 0; i < len(rules); {
		prefix := rules[i].prefix

		// Sorting groups the rules on `prefix` right after the first of them,
		// then every rule on a longer prefix.
		same := i + 1
		for same < len(rules) && rules[same].prefix == prefix {
			same++
		}
		below := same
		for below < len(rules) && strings.HasPrefix(rules[below].prefix, prefix) {
			below++
		}

		except := make([]Matcher, 0, same-i)
		for _, rule := range rules[i:same] {
			except = append(except, rule.except)
		}
		nestedGuarded, nestedGuards := buildGuarded(rules[same:below])

		guarded = append(guarded, prefix)
		guards = append(guards, guard{
			except: except,
			nested: Matcher{guarded: nestedGuarded, guards: nestedGuards},
		})

		i = below
	}

	return guarded, guards
}

// Len returns the number of distinct entries in the compiled matcher. Several
// rules written on the same prefix count as one.
func (m *Matcher) Len() int {
	if m == nil {
		return 0
	}
	length := len(m.exact) + len(m.prefixes) + len(m.guarded)
	for i := range m.guards {
		length += m.guards[i].nested.Len()
	}
	return length
}

// Test returns true if the given string is matched by one of the entries of
// the matcher: equal to one of its exact entries, or starting with one of its
// prefix entries without being matched by the exceptions of that entry.
func (m *Matcher) Test(name string) bool {
	if m == nil {
		return false
	}

	if len(m.prefixes) > 0 && testPrefixes(m.prefixes, name) {
		return true
	}

	if len(m.exact) > 0 {
		i := sort.SearchStrings(m.exact, name)
		if i < len(m.exact) && name == m.exact[i] {
			return true
		}
	}

	// Tested last: it is the only arm reading a second, nested matcher.
	return len(m.guarded) > 0 && m.testGuarded(name)
}

// coversAll returns true if every string starting with `prefix` is matched by
// the matcher, which is the case when one of its prefix entries is a prefix of
// `prefix`. It under-approximates -- `guarded` and `exact` are ignored -- so a
// false result only means "not provably covering".
func (m *Matcher) coversAll(prefix string) bool {
	return testPrefixes(m.prefixes, prefix)
}

// testGuarded returns true if `name` starts with one of the prefixes carrying
// exceptions and is matched by none of the exceptions written on that prefix,
// or is matched by one of the longer prefixes nested under it.
//
// `guarded` identifies unique prefixes, so at most one entry can be a prefix of
// `name` and a single binary search finds it. The recursion into `nested` costs
// one more search per level of prefix nesting actually matching `name`, which
// is the number of rules that have to be evaluated anyway.
func (m *Matcher) testGuarded(name string) bool {
	i := searchPrefixes(m.guarded, name)
	if i < 0 {
		return false
	}

	guard := &m.guards[i]
	for j := range guard.except {
		if !guard.except[j].Test(name) {
			return true
		}
	}

	return guard.nested.testGuarded(name)
}

// testPrefixes returns true if `name` starts with one of the entries of
// `prefixes`, which must be sorted and identify unique prefixes.
func testPrefixes(prefixes []string, name string) bool {
	return searchPrefixes(prefixes, name) >= 0
}

// searchPrefixes returns the index of the entry of `prefixes` that is a prefix
// of `name`, or -1 if there is none. `prefixes` must be sorted and identify
// unique prefixes, which makes that entry unique.
func searchPrefixes(prefixes []string, name string) int {
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
		return i - 1
	}
	if i < len(prefixes) && name == prefixes[i] {
		return i
	}
	return -1
}
