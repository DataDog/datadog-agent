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
			// A prefix entry absorbs the entries it already matches, within
			// the same matchPrefix-enabled list.
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
			// `*` has no special meaning anywhere in this list anymore: it is
			// a literal character, like any other, that can never appear in
			// (and therefore never match) a normalized metric name. NewMatcher
			// does not normalize its input, so the literal `*` is simply kept
			// as part of the exact entry.
			name:  "star has no special meaning",
			list:  []string{"foo.*.bar", "foo.*.baz.*"},
			exact: []string{"foo.*.bar", "foo.*.baz.*"},
		},
		{
			// The empty entry matches everything once matchPrefix turns it
			// into a prefix, and absorbs every other entry.
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
	// A bare prefix rule must match the prefix itself: the prefix entry and
	// the tested name are equal, which is the case the binary search has to
	// handle separately.
	m, dropped := NewMatcherWithPrefixRules(nil, false, prefixRulesFrom([]string{"foo"}))
	assert.Empty(t, dropped)
	assert.True(t, m.Test("foo"))
	assert.True(t, m.Test("foobar"))
	assert.False(t, m.Test("fo"))

	// The empty prefix matches every storable name; names the intake rejects
	// still never match.
	matchAll, dropped := NewMatcherWithPrefixRules(nil, false, prefixRulesFrom([]string{""}))
	assert.Empty(t, dropped)
	assert.True(t, matchAll.Test("anything"))
	assert.False(t, matchAll.Test(""))
	assert.False(t, matchAll.Test("123"))
}

// prefixRulesFrom builds bare (exception-free) PrefixRule entries, one per
// prefix, for tests that only care about plain prefix matching through the
// PrefixRule/NewMatcherWithPrefixRules path.
func prefixRulesFrom(prefixes []string) []PrefixRule {
	rules := make([]PrefixRule, 0, len(prefixes))
	for _, p := range prefixes {
		rules = append(rules, PrefixRule{Prefix: p})
	}
	return rules
}

func TestNewMatcherWithPrefixRulesBareRulesFoldIntoPrefixes(t *testing.T) {
	// A PrefixRule with no exceptions is exactly a bare prefix: it must fold
	// into `prefixes`, the same fast path a whole-list matchPrefix entry
	// uses, rather than being kept as a `rules` entry.
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

// TestNewMatcherWithPrefixRulesExceptionsAreScopedToTheirRule asserts the RFC
// rule that an exception belongs to the entry that declares it: a metric
// excepted by one entry is still dropped if another entry -- of the prefix
// rules list, or of the plain exact list -- matches it.
func TestNewMatcherWithPrefixRulesExceptionsAreScopedToTheirRule(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(
		[]string{"postgresql.connections"},
		false,
		[]PrefixRule{
			{Prefix: "postgresql.", ExceptExact: []string{"postgresql.connections"}},
		},
	)
	assert.Empty(t, dropped)

	// The prefix rule excepts it, but the plain exact list still drops it.
	assert.True(t, m.Test("postgresql.connections"))
}

func TestNewMatcherWithPrefixRulesExceptionsAreScopedToTheirRuleAmongRules(t *testing.T) {
	// Two overlapping rules, both excepting the same name: the metric is kept
	// only because every rule that covers it also excepts it.
	bothExcept, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "foo.", ExceptExact: []string{"foo.bar"}},
		{Prefix: "foo.b", ExceptExact: []string{"foo.bar"}},
	})
	assert.Empty(t, dropped)
	assert.False(t, bothExcept.Test("foo.bar"), "excepted by every covering rule")
	assert.True(t, bothExcept.Test("foo.other"))

	// Same overlap, but only one of the two rules excepts the name: the other
	// rule still drops it, exactly as the RFC specifies -- an exception
	// belongs to the entry that declares it.
	oneExcepts, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "foo.", ExceptExact: []string{"foo.bar"}},
		{Prefix: "foo.b", ExceptExact: []string{"foo.baz"}},
	})
	assert.Empty(t, dropped)
	assert.True(t, oneExcepts.Test("foo.bar"), "excepted by one rule, but still matched by the other")
}

// TestNewMatcherWithPrefixRulesDeadRuleDetection asserts that a rule whose
// prefix is already covered, unconditionally, by a broader bare prefix is
// dropped: its exceptions could never take effect anyway, since the broader
// prefix matches every name it could ever match regardless of them.
func TestNewMatcherWithPrefixRulesDeadRuleDetection(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "postgresql."},
		{Prefix: "postgresql.locks.", ExceptExact: []string{"postgresql.locks.waiting"}},
	})
	assert.Equal(t, []string{"postgresql.locks."}, dropped)
	assert.Empty(t, m.rules)

	// The dead rule's exception has no effect: `postgresql.` alone drops it.
	assert.True(t, m.Test("postgresql.locks.waiting"))
}

// TestNewMatcherWithPrefixRulesDeadRuleFromMatchPrefix asserts the same dead
// rule detection when the broader, unconditional prefix comes from the plain
// list via matchPrefix rather than from another PrefixRule.
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

// TestNewMatcherWithPrefixRulesNoSegmentBoundary asserts that the Agent does
// not require a prefix to end on a segment boundary: `sys` legitimately
// matches both `system.cpu` and `sys.cpu`. Enforcing full-segment prefixes,
// if ever wanted, is left to the backend.
func TestNewMatcherWithPrefixRulesNoSegmentBoundary(t *testing.T) {
	m, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{{Prefix: "sys"}})
	assert.Empty(t, dropped)

	assert.True(t, m.Test("system.cpu"))
	assert.True(t, m.Test("sys.cpu"))
	assert.False(t, m.Test("other.cpu"))
}

// TestNewMatcherWithPrefixRulesEmptyPrefix asserts that an empty prefix is a
// valid, match-everything entry, not something dropped as invalid -- with or
// without exceptions.
func TestNewMatcherWithPrefixRulesEmptyPrefix(t *testing.T) {
	bare, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{{Prefix: ""}})
	assert.Empty(t, dropped)
	assert.True(t, bare.MatchesAll())
	assert.True(t, bare.Test("anything"))

	withException, dropped := NewMatcherWithPrefixRules(nil, false, []PrefixRule{
		{Prefix: "", ExceptExact: []string{"keep.me"}},
	})
	assert.Empty(t, dropped)
	// Not everything is dropped: `keep.me` survives, so this is not the
	// same as the bare empty-prefix case above.
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
	// Prefixes and prefix rules are shared: a derived name matched by either
	// one stays matched, exceptions included.
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

	// An exception-bearing empty-prefix rule does not match everything: at
	// least its exception is kept, so it must not fold into `prefixes` and
	// must not report MatchesAll.
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

// BenchmarkStringsMatcherMixed measures the cost of a list mixing exact
// entries and bare (exception-free) prefix rules, where `Test` has to probe
// both the exact set and the compacted `prefixes` set, against the exact-only
// list of the same size.
func BenchmarkStringsMatcherMixed(b *testing.B) {
	const size = 5000

	var values []string
	for range size {
		values = append(values, randomString(50))
	}

	// Turn one entry out of two into a bare prefix rule.
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

// BenchmarkStringsMatcherPrefixRulesExceptions measures the cost `Test` pays
// for exception-bearing prefix rules specifically: they are linear-scanned,
// unlike bare prefixes and exact entries which stay on the compacted,
// binary-searched fast paths. This is what backs the claim that a
// metric_filterlist_prefix list made only of bare prefixes costs the same as
// today, and that the extra cost of exceptions is proportional to how many
// rules actually declare them.
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
			// One matching bare prefix, so the benchmark also pays the
			// (unaffected) compacted-prefix lookup cost every real-world list
			// carrying `metric_filterlist_prefix` entries pays too.
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
