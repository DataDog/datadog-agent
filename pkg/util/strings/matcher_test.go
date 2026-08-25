// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package strings

import (
	"fmt"
	"math/rand"
	"slices"
	stdstrings "strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMatcher(t *testing.T) {
	check := func(data []string) []string {
		b := NewMatcher(data, true)
		return b.prefixes
	}

	assert.Equal(t, []string(nil), check([]string{}))
	assert.Equal(t, []string{"a"}, check([]string{"a"}))
	assert.Equal(t, []string{"a"}, check([]string{"a", "aa"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "aa", "b", "bb"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "b", "bb"}))
}

func TestNewMatcherPatterns(t *testing.T) {
	cases := []struct {
		name        string
		list        []string
		matchPrefix bool
		exact       []string
		prefixes    []string
	}{
		{
			name: "empty",
			list: []string{},
		},
		{
			name:  "exact only",
			list:  []string{"foo.bar", "aaa"},
			exact: []string{"aaa", "foo.bar"},
		},
		{
			name:     "prefix only",
			list:     []string{"foo.*", "aaa.*"},
			prefixes: []string{"aaa.", "foo."},
		},
		{
			name:     "mixed",
			list:     []string{"foo.bar", "foo.baz.*", "zzz"},
			exact:    []string{"foo.bar", "zzz"},
			prefixes: []string{"foo.baz."},
		},
		{
			// A prefix entry absorbs the exact entries it already matches.
			name:     "exact covered by prefix",
			list:     []string{"foo.baz.qux", "foo.baz.*", "foo.baz.", "foo.bar"},
			exact:    []string{"foo.bar"},
			prefixes: []string{"foo.baz."},
		},
		{
			name:     "redundant prefixes",
			list:     []string{"app.*", "app.metrics.*", "app.metrics.http.*"},
			prefixes: []string{"app."},
		},
		{
			name:  "duplicated entries",
			list:  []string{"foo", "foo", "foo"},
			exact: []string{"foo"},
		},
		{
			name:     "duplicated prefixes",
			list:     []string{"foo.*", "foo.*"},
			prefixes: []string{"foo."},
		},
		{
			// `*` is only special as a trailing character.
			name:     "star in the middle is literal",
			list:     []string{"foo.*.bar", "foo.*.baz.*"},
			exact:    []string{"foo.*.bar"},
			prefixes: []string{"foo.*.baz."},
		},
		{
			// A lone `*` matches everything, and absorbs every other entry.
			name:     "match all",
			list:     []string{"*", "foo", "bar.*"},
			prefixes: []string{""},
		},
		{
			// The trailing `*` is stripped whether or not matchPrefix is set, so
			// that a prefix pattern behaves the same in both modes.
			name:        "match prefix strips the star",
			list:        []string{"foo.*", "bar"},
			matchPrefix: true,
			prefixes:    []string{"bar", "foo."},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewMatcher(c.list, c.matchPrefix)
			assert.Equal(t, c.exact, m.exact, "exact entries")
			assert.Equal(t, c.prefixes, m.prefixes, "prefix entries")
			assert.Equal(t, len(c.exact)+len(c.prefixes), m.Len())
		})
	}
}

func TestIsStringMatchingPatterns(t *testing.T) {
	list := []string{"foo.bar", "foo.baz.*", "zzz", "app.*"}

	cases := []struct {
		result bool
		name   string
	}{
		// exact entries
		{true, "foo.bar"},
		{false, "foo.ba"},
		{false, "foo.barbaz"},
		{true, "zzz"},
		// prefix entries
		{true, "foo.baz."},
		{true, "foo.baz.count"},
		{false, "foo.baz"},
		{true, "app."},
		{true, "app.metrics.http"},
		{false, "ap"},
		// a prefix entry matches the prefix itself, without its trailing `*`
		{true, "app."},
		// unrelated
		{false, ""},
		{false, "other.metric"},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%q", c.name), func(t *testing.T) {
			m := NewMatcher(list, false)
			assert.Equal(t, c.result, m.Test(c.name))
		})
	}
}

func TestIsStringMatchingBarePrefix(t *testing.T) {
	// `foo*` must match `foo` itself: the prefix entry and the tested name are
	// equal, which is the case the binary search has to handle separately.
	m := NewMatcher([]string{"foo*"}, false)
	assert.True(t, m.Test("foo"))
	assert.True(t, m.Test("foobar"))
	assert.False(t, m.Test("fo"))

	matchAll := NewMatcher([]string{"*"}, false)
	assert.True(t, matchAll.Test("anything"))
	assert.True(t, matchAll.Test(""))
}

func TestNilMatcher(t *testing.T) {
	var m *Matcher
	assert.False(t, m.Test("foo"))
	assert.Equal(t, 0, m.Len())

	empty := Matcher{}
	assert.False(t, empty.Test("foo"))
	assert.Equal(t, 0, empty.Len())
}

func TestIsStringMatching(t *testing.T) {
	cases := []struct {
		result      bool
		name        string
		list        []string
		matchPrefix bool
	}{
		{false, "some", []string{}, false},
		{false, "some", []string{}, true},
		{false, "foo", []string{"bar", "baz"}, false},
		{false, "foo", []string{"bar", "baz"}, true},
		{false, "bar", []string{"foo", "baz"}, false},
		{false, "bar", []string{"foo", "baz"}, true},
		{true, "baz", []string{"foo", "baz"}, false},
		{true, "baz", []string{"foo", "baz"}, true},
		{false, "foobar", []string{"foo", "baz"}, false},
		{true, "foobar", []string{"foo", "baz"}, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v-%v-%v", c.name, c.list, c.matchPrefix),
			func(t *testing.T) {
				b := NewMatcher(c.list, c.matchPrefix)
				assert.Equal(t, c.result, b.Test(c.name))
			})
	}
}

func randomString(size uint) string {
	letterBytes := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	var builder stdstrings.Builder
	for range size {
		builder.WriteByte(letterBytes[rand.Intn(len(letterBytes))])
	}

	return builder.String()
}

func BenchmarkStringsMatcher(b *testing.B) {
	words := []string{
		"foo",
		"longer.name.but.still.small",
		"very.long.string.with.some.good.amount.of.chars.for.a.metric",
		"bar",
	}
	for i := 1000; i <= 10000; i += 1000 {
		b.Run(fmt.Sprintf("strings-matcher-%d", i), func(b *testing.B) {
			var values []string
			for range i {
				values = append(values, randomString(50))
			}
			benchmarkStringsMatcher(b, words, values)
		})
	}
}

func benchmarkStringsMatcher(b *testing.B, words, values []string) {
	b.ReportAllocs()
	b.ResetTimer()

	// first and last will match
	words[0] = values[0]
	words[3] = values[100]

	matcher := NewMatcher(values, false)

	for n := 0; n < b.N; n++ {
		matcher.Test(words[n%len(words)])
	}
}

// BenchmarkStringsMatcherMixed measures the cost of a list mixing exact and
// prefix entries, where `Test` has to probe both sets, against the exact-only
// list of the same size.
func BenchmarkStringsMatcherMixed(b *testing.B) {
	const size = 5000

	var values []string
	for range size {
		values = append(values, randomString(50))
	}

	mixed := slices.Clone(values)
	// Turn one entry out of two into a prefix pattern.
	for i := 0; i < len(mixed); i += 2 {
		mixed[i] += PrefixSuffix
	}

	words := []string{
		"foo",
		"longer.name.but.still.small",
		"very.long.string.with.some.good.amount.of.chars.for.a.metric",
		"bar",
	}

	for name, list := range map[string][]string{"exact-only": values, "mixed": mixed} {
		b.Run(name, func(b *testing.B) {
			benchmarkStringsMatcher(b, slices.Clone(words), list)
		})
	}
}
