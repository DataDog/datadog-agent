// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"fmt"
	"math/rand"
	"strings"
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

	// Entries are taken verbatim, never rewritten. A non-normalized entry is
	// kept as-is and simply matches nothing, rather than being rewritten into
	// something that matches more than the user asked for.
	assert.Equal(t, []string{"a-b", "a_b"}, check([]string{"a-b", "a_b"}))
}

// TestNewMatcherDoesNotRewritePrefixEntries guards the failure mode that
// normalizing entries would reintroduce: normalizing a prefix entry as if it
// were a complete metric name strips its trailing separator, which widens it.
// `redis.checkpoint_` must not start behaving like `redis.checkpoint`.
func TestNewMatcherDoesNotRewritePrefixEntries(t *testing.T) {
	m := NewMatcher([]string{"redis.checkpoint_"}, true)

	assert.Equal(t, []string{"redis.checkpoint_"}, m.prefixes, "prefix entry must be kept verbatim")

	// In the family the user asked for.
	assert.True(t, m.Test("redis.checkpoint_bytes"))
	assert.True(t, m.Test("redis.checkpoint-bytes"), "raw name normalizes into the family")

	// Adjacent names that merely share the shorter prefix must be left alone.
	assert.False(t, m.Test("redis.checkpointing.count"))
	assert.False(t, m.Test("redis.checkpointed"))
}

// TestIsStringMatchingNormalizesNames asserts that filter list matching happens
// in the same name space the backend stores, so a metric submitted with a raw
// name is filtered by its normalized name.
func TestIsStringMatchingNormalizesNames(t *testing.T) {
	cases := []struct {
		result      bool
		name        string
		list        []string
		matchPrefix bool
	}{
		// The submitted name needs normalizing, the configured entry is the
		// normalized name the user sees in Datadog.
		{true, "my metric-name", []string{"my_metric_name"}, false},
		{true, "custom.metric one", []string{"custom.metric_one"}, false},
		{true, "host.cpu%util", []string{"host.cpu_util"}, false},
		{true, "1app.requests", []string{"app.requests"}, false},
		{true, "caf\u00e9.requests", []string{"caf.requests"}, false},

		// Distinct raw names that normalize to the same thing are both filtered.
		{true, "multiple-norm-1", []string{"multiple_norm_1"}, false},
		{true, "multiple_norm-1", []string{"multiple_norm_1"}, false},

		// Entries are expected to already be normalized. A non-normalized entry
		// matches nothing rather than being rewritten, so a misconfigured entry
		// under-filters instead of silently over-filtering.
		{false, "my_metric_name", []string{"my metric-name"}, false},
		{false, "my metric-name", []string{"my-metric-name"}, false},

		// Normalization must not make unrelated names collide.
		{false, "my.metric", []string{"my_metric"}, false},
		{false, "other metric", []string{"my_metric"}, false},

		// Prefix matching also works on the normalized name.
		{true, "custom.metric name.count", []string{"custom.metric_name"}, true},
		{false, "custom.metric name.count", []string{"custom.other"}, true},

		// Names the intake rejects outright never match.
		{false, "", []string{"foo"}, false},
		{false, "123", []string{"foo"}, false},
		{false, strings.Repeat("foo", 200), []string{"foo"}, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v-%v-%v", c.name, c.list, c.matchPrefix),
			func(t *testing.T) {
				b := NewMatcher(c.list, c.matchPrefix)
				assert.Equal(t, c.result, b.Test(c.name))
			})
	}
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
			name:        "prefix only",
			list:        []string{"foo.", "aaa."},
			matchPrefix: true,
			prefixes:    []string{"aaa.", "foo."},
		},
		{
			name:        "redundant prefixes",
			list:        []string{"app.", "app.metrics.", "app.metrics.http."},
			matchPrefix: true,
			prefixes:    []string{"app."},
		},
		{
			name:  "duplicated entries",
			list:  []string{"foo", "foo", "foo"},
			exact: []string{"foo"},
		},
		{
			name:        "duplicated prefixes",
			list:        []string{"foo.", "foo."},
			matchPrefix: true,
			prefixes:    []string{"foo."},
		},
		{
			name:  "star has no special meaning",
			list:  []string{"foo.*.bar", "foo.*.baz.*"},
			exact: []string{"foo.*.bar", "foo.*.baz.*"},
		},
		{
			name:        "empty entry with match prefix matches all",
			list:        []string{"", "foo", "bar"},
			matchPrefix: true,
			prefixes:    []string{""},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewMatcher(c.list, c.matchPrefix)
			assert.Equal(t, c.exact, m.exact, "exact entries")
			assert.Equal(t, c.prefixes, m.prefixes, "prefix entries")
			assert.Empty(t, m.rules)
			assert.Equal(t, len(c.exact)+len(c.prefixes), m.Len())
		})
	}
}

func TestIsStringMatchingPatterns(t *testing.T) {
	list := []string{"foo.bar", "zzz"}
	prefixList := []string{"foo.baz.", "app."}

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
		// the name is normalized before being matched
		{true, "foo.baz.count-per-second"},
		{true, "app.metrics per second"},
		{false, "app metrics"},
		// unrelated
		{false, ""},
		{false, "other.metric"},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%q", c.name), func(t *testing.T) {
			m, dropped := NewMatcherWithPrefixRules(list, false, prefixRulesFrom(prefixList))
			assert.Empty(t, dropped)
			assert.Equal(t, c.result, m.Test(c.name))
		})
	}
}

func TestIsStringMatchingBarePrefix(t *testing.T) {
	// Covers the equality case in testPrefixes' binary-search path.
	m, dropped := NewMatcherWithPrefixRules(nil, false, prefixRulesFrom([]string{"foo"}))
	assert.Empty(t, dropped)
	assert.True(t, m.Test("foo"))
	assert.True(t, m.Test("foobar"))
	assert.False(t, m.Test("fo"))

	matchAll, dropped := NewMatcherWithPrefixRules(nil, false, prefixRulesFrom([]string{""}))
	assert.Empty(t, dropped)
	assert.True(t, matchAll.Test("anything"))
	assert.False(t, matchAll.Test(""))
	assert.False(t, matchAll.Test("123"))
}

func prefixRulesFrom(prefixes []string) []PrefixRule {
	rules := make([]PrefixRule, 0, len(prefixes))
	for _, p := range prefixes {
		rules = append(rules, PrefixRule{Prefix: p})
	}
	return rules
}

func TestNewMatcherWithPrefixRulesBareRulesFoldIntoPrefixes(t *testing.T) {
	// Exception-free rules use the compacted prefix fast path.
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "redis."},
		{Prefix: "postgresql.", ExceptExact: nil, ExceptPrefix: nil},
	})
	assert.Empty(t, dropped)
	assert.Equal(t, []string{"postgresql.", "redis."}, m.prefixes)
	assert.Empty(t, m.rules)
}

func TestNewMatcherWithPrefixRulesExceptExact(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "postgresql.", ExceptExact: []string{"postgresql.connections"}},
	})
	assert.Empty(t, dropped)
	assert.Len(t, m.rules, 1)

	assert.True(t, m.Test("postgresql.locks"))
	assert.False(t, m.Test("postgresql.connections"), "excepted exact name is kept")
	assert.False(t, m.Test("postgres.other"))
}

func TestNewMatcherWithPrefixRulesExceptPrefix(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "postgresql.", ExceptPrefix: []string{"postgresql.locks."}},
	})
	assert.Empty(t, dropped)

	assert.True(t, m.Test("postgresql.connections"))
	assert.False(t, m.Test("postgresql.locks."), "the exception prefix itself is kept")
	assert.False(t, m.Test("postgresql.locks.waiting"), "everything under the exception prefix is kept")
}

func TestNewMatcherWithPrefixRulesBothExceptionKinds(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{
			Prefix:       "postgresql.",
			ExceptExact:  []string{"postgresql.connections"},
			ExceptPrefix: []string{"postgresql.locks."},
		},
	})
	assert.Empty(t, dropped)

	assert.True(t, m.Test("postgresql.queries"))
	assert.False(t, m.Test("postgresql.connections"))
	assert.False(t, m.Test("postgresql.locks.waiting"))
}

func TestNewMatcherWithPrefixRulesDoesNotRetainExceptionInputSlices(t *testing.T) {
	rules := []PrefixRule{{
		Prefix:       "postgresql.",
		ExceptExact:  []string{"postgresql.connections", "postgresql.connections", "postgresql.locks"},
		ExceptPrefix: []string{"postgresql.metrics.", "postgresql.metrics.waiting.", "postgresql.metrics."},
	}}

	first, dropped := NewMatcherWithPrefixRules(nil, false, rules)
	assert.Empty(t, dropped)
	assert.False(t, first.Test("postgresql.connections"))
	assert.False(t, first.Test("postgresql.locks"))
	assert.False(t, first.Test("postgresql.metrics.waiting"))

	second, dropped := NewMatcherWithPrefixRules(nil, false, rules)
	assert.Empty(t, dropped)
	assert.False(t, second.Test("postgresql.connections"))
	assert.False(t, second.Test("postgresql.locks"))
	assert.False(t, second.Test("postgresql.metrics.waiting"))

	assert.False(t, first.Test("postgresql.connections"))
	assert.False(t, first.Test("postgresql.locks"))
	assert.False(t, first.Test("postgresql.metrics.waiting"))
}

// Exceptions are scoped to the entry that declares them.
func TestNewMatcherWithPrefixRulesExceptionsAreScopedToTheirRule(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(
		[]string{"postgresql.connections"},
		false,
		[]PrefixRule{
			{Prefix: "postgresql.", ExceptExact: []string{"postgresql.connections"}},
		},
	)
	assert.Empty(t, dropped)

	assert.True(t, m.Test("postgresql.connections"))
}

func TestNewMatcherWithPrefixRulesExceptionsAreScopedToTheirRuleAmongRules(t *testing.T) {
	bothExcept, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "foo.", ExceptExact: []string{"foo.bar"}},
		{Prefix: "foo.b", ExceptExact: []string{"foo.bar"}},
	})
	assert.Empty(t, dropped)
	assert.False(t, bothExcept.Test("foo.bar"), "excepted by every covering rule")
	assert.True(t, bothExcept.Test("foo.other"))

	oneExcepts, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "foo.", ExceptExact: []string{"foo.bar"}},
		{Prefix: "foo.b", ExceptExact: []string{"foo.baz"}},
	})
	assert.Empty(t, dropped)
	assert.True(t, oneExcepts.Test("foo.bar"), "excepted by one rule, but still matched by the other")
}

// Rules shadowed by unconditional prefixes are dropped.
func TestNewMatcherWithPrefixRulesDeadRuleDetection(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "postgresql."},
		{Prefix: "postgresql.locks.", ExceptExact: []string{"postgresql.locks.waiting"}},
	})
	assert.Equal(t, []string{"postgresql.locks."}, dropped)
	assert.Empty(t, m.rules)

	assert.True(t, m.Test("postgresql.locks.waiting"))
}

func TestNewMatcherWithPrefixRulesDeadRuleFromMatchPrefix(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(
		[]string{"postgresql."},
		true,
		[]PrefixRule{
			{Prefix: "postgresql.locks.", ExceptExact: []string{"postgresql.locks.waiting"}},
		},
	)
	assert.Equal(t, []string{"postgresql.locks."}, dropped)
	assert.Empty(t, m.rules)
	assert.True(t, m.Test("postgresql.locks.waiting"))
}

func TestNewMatcherWithPrefixRulesNoSegmentBoundary(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{{Prefix: "sys"}})
	assert.Empty(t, dropped)

	assert.True(t, m.Test("system.cpu"))
	assert.True(t, m.Test("sys.cpu"))
	assert.False(t, m.Test("other.cpu"))
}

func TestNewMatcherWithPrefixRulesEmptyPrefix(t *testing.T) {
	bare, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{{Prefix: ""}})
	assert.Empty(t, dropped)
	assert.True(t, bare.MatchesAll())
	assert.True(t, bare.Test("anything"))

	withException, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "", ExceptExact: []string{"keep.me"}},
	})
	assert.Empty(t, dropped)
	assert.False(t, withException.MatchesAll())
	assert.True(t, withException.Test("anything.else"))
	assert.False(t, withException.Test("keep.me"))
}

func TestRestrictExact(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(
		[]string{"foo.count", "foo.max"},
		false,
		[]PrefixRule{
			{Prefix: "bar."},
			{Prefix: "baz.", ExceptExact: []string{"baz.keep"}},
		},
	)
	assert.Empty(t, dropped)

	restricted := m.RestrictExact(func(name string) bool {
		return strings.HasSuffix(name, ".count")
	})

	assert.Equal(t, []string{"foo.count"}, restricted.exact)
	// Prefix state is shared for histogram aggregate matchers.
	assert.Equal(t, m.prefixes, restricted.prefixes)
	assert.Equal(t, m.rules, restricted.rules)

	assert.True(t, restricted.Test("foo.count"))
	assert.False(t, restricted.Test("foo.max"))
	assert.True(t, restricted.Test("bar.anything"))
	assert.True(t, restricted.Test("baz.anything"))
	assert.False(t, restricted.Test("baz.keep"))
}

func TestMatchesAll(t *testing.T) {
	// However the empty prefix is spelled, and whatever else is in the list.
	prefixMode := NewMatcher([]string{""}, true)
	assert.True(t, prefixMode.MatchesAll())

	withRule, dropped := NewMatcherWithPrefixRules([]string{"foo"}, false, []PrefixRule{{Prefix: ""}})
	assert.Empty(t, dropped)
	assert.True(t, withRule.MatchesAll())

	// A prefix that merely matches a lot is not the empty prefix.
	for _, list := range [][]string{nil, {"foo"}} {
		m := NewMatcher(list, false)
		assert.False(t, m.MatchesAll(), "%v should not match every name", list)
	}
	prefix := NewMatcher([]string{"a"}, true)
	assert.False(t, prefix.MatchesAll())

	// Exceptions keep empty-prefix rules out of MatchesAll.
	withException, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "", ExceptExact: []string{"keep.me"}},
	})
	assert.Empty(t, dropped)
	assert.False(t, withException.MatchesAll())
}

func TestNilMatcher(t *testing.T) {
	var m *Matcher
	assert.False(t, m.Test("foo"))
	assert.Equal(t, 0, m.Len())
	assert.False(t, m.MatchesAll())

	empty := Matcher{}
	assert.False(t, empty.Test("foo"))
	assert.Equal(t, 0, empty.Len())
	assert.False(t, empty.MatchesAll())
}

func randomString(size uint) string {
	letterBytes := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	var builder strings.Builder
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

// Bare prefix rules should stay on the compacted-prefix fast path.
func BenchmarkStringsMatcherMixed(b *testing.B) {
	const size = 5000

	var values []string
	for range size {
		values = append(values, randomString(50))
	}

	var exact []string
	var rules []PrefixRule
	for i, v := range values {
		if i%2 == 0 {
			rules = append(rules, PrefixRule{Prefix: v})
			continue
		}
		exact = append(exact, v)
	}

	words := []string{
		"foo",
		"longer.name.but.still.small",
		"very.long.string.with.some.good.amount.of.chars.for.a.metric",
		"bar",
	}

	exactOnly := NewMatcher(values, false)
	mixed, _ := NewMatcherWithPrefixRules(exact, false, rules)

	for name, matcher := range map[string]Matcher{"exact-only": exactOnly, "mixed": mixed} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				matcher.Test(words[n%len(words)])
			}
		})
	}
}

// Exception-bearing prefix rules are the linear-scan path.
func BenchmarkStringsMatcherPrefixRulesExceptions(b *testing.B) {
	words := []string{
		"foo.bar",
		"not.matched.at.all",
	}

	for _, n := range []int{0, 1, 10, 100} {
		b.Run(fmt.Sprintf("exception-rules-%d", n), func(b *testing.B) {
			var rules []PrefixRule
			for i := 0; i < n; i++ {
				rules = append(rules, PrefixRule{
					Prefix:      fmt.Sprintf("unrelated%d.", i),
					ExceptExact: []string{fmt.Sprintf("unrelated%d.keep", i)},
				})
			}
			// Include the bare-prefix fast path in every run.
			rules = append(rules, PrefixRule{Prefix: "foo."})

			matcher, _ := NewMatcherWithPrefixRules(nil, false, rules)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				matcher.Test(words[i%len(words)])
			}
		})
	}
}
