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

// Matcher tests a metric name for match against a list of metric names.
// See `NewMatcher` for details.
type Matcher struct {
	data        []string
	matchPrefix bool
}

// NewMatcher creates a new metric name matcher.
// Use `matchPrefix` to  create a prefixes matcher.
//
// Entries are taken verbatim. They are expected to already be normalized, i.e.
// to be metric names as the backend stores and displays them, which is what
// users copy into a filter list. `Test` normalizes the name it is given, so the
// comparison happens in that same name space.
//
// Entries are deliberately *not* normalized here. Doing so is a no-op for any
// already-normalized entry, because `Normalize` is idempotent, so the only
// entries it would affect are ones that cannot match any stored metric name in
// the first place. For those, leaving the entry alone means it matches nothing,
// whereas rewriting it can widen it: as a prefix, `foo_` would become `foo` and
// start matching unrelated names such as `foobar`.
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

// Len returns the number of entries in the compiled matcher.
func (m *Matcher) Len() int {
	if m == nil {
		return 0
	}
	return len(m.data)
}

// Test returns true if the given metric name matches one in the matcher list,
// or is matching by prefix if the matcher has been created with `matchPrefix`.
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

	if len(m.data) == 0 {
		return false
	}

	// Fast path: already normalized, so compare the name as given.
	if IsNormalized(name) {
		return m.search(name)
	}

	// Slow path. Normalizing allocates if it has to return a string, and this
	// runs per DogStatsD sample, so build the normalized form in a stack buffer
	// instead. normalizeAppend cannot exceed MaxLength (see its doc comment), so
	// append never reallocates and buf never escapes.
	var buf [MaxLength]byte
	key, ok := normalizeAppend(buf[:0], name)
	if !ok {
		return false
	}

	// Safe: the string aliases buf, search only reads it for comparison and
	// never retains it, and buf is not written again while it is alive.
	return m.search(unsafe.String(unsafe.SliceData(key), len(key)))
}

// search looks name up in the compiled list. name must already be normalized.
func (m *Matcher) search(name string) bool {
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
