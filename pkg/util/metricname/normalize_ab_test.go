// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// REVIEW AID -- DELETE BEFORE MERGING.
//
// This file carries the previous two-pass implementation so the single-pass merge
// on this branch can be measured against it in one process, over identical
// fixtures. Cross-run comparison on a laptop is worthless here: the same
// benchmark has swung 500-850 ns/op between runs on this machine.
//
// Reproduce with, and note that each sub-benchmark must be run in ISOLATION --
// running old before new in the same process flatters new by ~20% through cache
// effects, which is how the first version of this measurement misled me:
//
//	for w in new-one-pass old-two-pass; do
//	  for c in already-normalized hyphenated leading-digit trailing-hyphen lading-mix; do
//	    bazel test //pkg/util/metricname:metricname_test --test_output=all \
//	      --test_arg=-test.bench="BenchmarkABPass/$c/$w\$" --test_arg=-test.run='^$' \
//	      --test_arg=-test.benchtime=2000000x --test_arg=-test.count=3 \
//	      --cache_test_results=no
//	  done
//	done
//
// Measured on a 12th Gen i9-12900H, ns/op, 0 allocs on every path:
//
//	probe shape                          two-pass   single-pass
//	already normalised                     ~138        ~144      neutral
//	hyphenated      (deviates at idx 3)    ~188        ~220      15% worse
//	leading digit   (deviates at idx 0)    ~194        ~217      6% worse
//	trailing hyphen (deviates at idx 87)   ~251        ~164      38% better
//	realistic lading mix                   ~620        ~627      neutral
//
// The shape of that result is structural, not an artefact. For a name deviating
// at position k of n, two-pass costs k+n byte-ops while single-pass costs
// n+memcpy(k). The merge can only win when k is large: a long clean prefix
// followed by a late rewrite. Real metric names deviate early or not at all, so
// `trailing-hyphen` -- the only clear win -- is a probe I invented, not a shape
// that occurs. A minBulkCopy threshold to skip short copies was also tried and
// did not recover the hyphenated case, which shows the cost is the extra plumbing
// rather than the memmove.

package metricname

import (
	"sort"
	"strings"
	"testing"
	"unsafe"
)

// oldIsNormalized is the predicate this branch removes. It spelled the rewrite
// rules out a second time, which is why FuzzIsNormalizedMatchesNormalize existed.
func oldIsNormalized(name string) bool {
	if len(name) == 0 || len(name) > maxLength {
		return false
	}
	if !isAlpha(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		switch c := name[i]; {
		case isAlphaNum(c) || c == '.':
		case c == '_':
			if !isAlphaNum(name[i-1]) {
				return false
			}
			if i == len(name)-1 || !isAlphaNum(name[i+1]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// oldNormalizeAppend is the previous builder: it always rebuilt from the first
// letter, with no notion of a surviving prefix.
func oldNormalizeAppend(dst []byte, name string) ([]byte, bool) {
	start, ok := firstAlpha(name)
	if !ok {
		return dst, false
	}
	for i := start; i < len(name); i++ {
		switch c := name[i]; {
		case isAlphaNum(c):
			dst = append(dst, c)
		case c == '.':
			switch dst[len(dst)-1] {
			case '_':
				dst[len(dst)-1] = '.'
			default:
				dst = append(dst, '.')
			}
		default:
			switch dst[len(dst)-1] {
			case '.', '_':
			default:
				dst = append(dst, '_')
			}
		}
	}
	if dst[len(dst)-1] == '_' {
		dst = dst[:len(dst)-1]
	}
	return dst, true
}

// oldTest mirrors Matcher.Test as it was, so the two differ only in normalisation.
func (m *Matcher) oldTest(name string) bool {
	if m == nil || len(m.data) == 0 {
		return false
	}
	if oldIsNormalized(name) {
		return m.oldSearch(name)
	}
	var buf [maxLength]byte
	key, ok := oldNormalizeAppend(buf[:0], name)
	if !ok {
		return false
	}
	return m.oldSearch(unsafe.String(unsafe.SliceData(key), len(key)))
}

func (m *Matcher) oldSearch(name string) bool {
	i := sort.SearchStrings(m.data, name)
	if m.matchPrefix && i > 0 && strings.HasPrefix(name, m.data[i-1]) {
		return true
	}
	if i < len(m.data) {
		return name == m.data[i]
	}
	return false
}

// TestOldAndNewAgree guards the comparison itself: if the two implementations
// ever disagree, the benchmark below is measuring two different functions and its
// numbers mean nothing.
func TestOldAndNewAgree(t *testing.T) {
	alphabet := []byte{'a', 'B', '1', '.', '_', '-', 0xC3}
	var buf []byte
	var rec func(depth int)
	rec = func(depth int) {
		name := string(buf)
		wantStr, wantOK := func() (string, bool) {
			var b [maxLength]byte
			out, ok := oldNormalizeAppend(b[:0], name)
			if !ok {
				return name, false
			}
			return string(out), true
		}()
		gotStr, gotOK := normalize(name)
		if wantOK != gotOK || (wantOK && wantStr != gotStr) {
			t.Fatalf("old and new disagree for %q: old (%q,%v), new (%q,%v)",
				name, wantStr, wantOK, gotStr, gotOK)
		}
		if oldIsNormalized(name) != isNormalized(name) {
			t.Fatalf("old and new predicates disagree for %q", name)
		}
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			buf = append(buf, c)
			rec(depth - 1)
			buf = buf[:len(buf)-1]
		}
	}
	rec(5)
}

// BenchmarkABPass compares the two implementations over identical fixtures.
// Run each sub-benchmark in isolation; see the file comment.
func BenchmarkABPass(b *testing.B) {
	normEntries, normProbes := benchVariant(".")
	hyphEntries, hyphProbes := benchVariant("-")

	digitProbes := make([]string, len(normProbes))
	for i, n := range normProbes {
		digitProbes[i] = "9" + n
	}
	tailEntries := make([]string, len(normEntries))
	tailProbes := make([]string, len(normEntries))
	for i, n := range normEntries {
		tailEntries[i] = n[:len(n)-1]
		tailProbes[i] = n[:len(n)-1] + "-"
	}

	for _, c := range []struct {
		name    string
		entries []string
		probes  []string
	}{
		{"already-normalized", normEntries, normProbes},
		{"hyphenated", hyphEntries, hyphProbes},
		{"leading-digit", normEntries, digitProbes},
		{"trailing-hyphen", tailEntries, tailProbes},
		{"lading-mix", buildNames(10_000, "tail"), ladingNames(benchProbeCount, 2)},
	} {
		m := NewMatcher(c.entries, false)
		b.Run(c.name+"/old-two-pass", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				m.oldTest(c.probes[i%len(c.probes)])
			}
		})
		b.Run(c.name+"/new-one-pass", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				m.Test(c.probes[i%len(c.probes)])
			}
		})
	}
}
