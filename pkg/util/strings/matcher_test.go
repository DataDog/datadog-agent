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
	"github.com/stretchr/testify/require"
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

func TestNewRuleMatcherExceptions(t *testing.T) {
	cases := []struct {
		name        string
		rules       []Rule
		matchPrefix bool
		exact       []string
		prefixes    []string
		guarded     []string
		// nested maps a guarded entry to the guarded entries nested under it.
		nested map[string][]string
		// except maps a guarded entry to the number of rules written on it.
		// Defaults to one.
		except map[string]int
	}{
		{
			name:     "prefix without exception stays unguarded",
			rules:    []Rule{{Pattern: "foo.*"}, {Pattern: "bar.*", Except: []string{}}},
			prefixes: []string{"bar.", "foo."},
		},
		{
			name:    "prefix with exception is guarded",
			rules:   []Rule{{Pattern: "foo.*", Except: []string{"foo.keep"}}},
			guarded: []string{"foo."},
		},
		{
			// An unconditional prefix matches everything below it whatever
			// the longer rule's exceptions say, so the longer rule is dead.
			name:     "guarded prefix absorbed by shorter unguarded prefix",
			rules:    []Rule{{Pattern: "foo.*"}, {Pattern: "foo.bar.*", Except: []string{"foo.bar.keep"}}},
			prefixes: []string{"foo."},
		},
		{
			// The reverse does not hold: the shorter rule is cancelled for
			// its exceptions, so the longer one still has work to do.
			name:     "unguarded prefix survives under a shorter guarded prefix",
			rules:    []Rule{{Pattern: "foo.*", Except: []string{"foo.keep"}}, {Pattern: "foo.bar.*"}},
			prefixes: []string{"foo.bar."},
			guarded:  []string{"foo."},
		},
		{
			// Both are live, but only the shorter one is searchable: the longer
			// one is nested under it so that at most one entry can be a prefix
			// of a given string.
			name:    "nested guarded prefixes are both kept",
			rules:   []Rule{{Pattern: "foo.bar.*", Except: []string{"foo.bar.b"}}, {Pattern: "foo.*", Except: []string{"foo.a"}}},
			guarded: []string{"foo."},
			nested:  map[string][]string{"foo.": {"foo.bar."}},
			except:  map[string]int{"foo.": 1},
		},
		{
			// Two rules on the same prefix with different exceptions are not
			// interchangeable: they share one entry, which keeps both sets of
			// exceptions.
			name:    "same prefix with different exceptions",
			rules:   []Rule{{Pattern: "foo.*", Except: []string{"foo.a"}}, {Pattern: "foo.*", Except: []string{"foo.b"}}},
			guarded: []string{"foo."},
			except:  map[string]int{"foo.": 2},
		},
		{
			// An exception covering the whole prefix leaves nothing to match.
			name:  "exception covering the prefix kills the rule",
			rules: []Rule{{Pattern: "foo.bar.*", Except: []string{"foo.*"}}},
		},
		{
			name:  "exception equal to the prefix kills the rule",
			rules: []Rule{{Pattern: "foo.*", Except: []string{"foo.*"}}},
		},
		{
			// Exceptions on an exact entry can only cancel it entirely.
			name:  "exact entry cancelled by its own exception",
			rules: []Rule{{Pattern: "foo.bar", Except: []string{"foo.*"}}},
		},
		{
			name:  "exact entry with an exception that never matches",
			rules: []Rule{{Pattern: "foo.bar", Except: []string{"foo.baz"}}},
			exact: []string{"foo.bar"},
		},
		{
			// An exact entry is only absorbed by an unconditional prefix: a
			// guarded one may be cancelled for that very name.
			name:    "exact entry survives under a guarded prefix",
			rules:   []Rule{{Pattern: "foo.*", Except: []string{"foo.keep"}}, {Pattern: "foo.keep"}},
			exact:   []string{"foo.keep"},
			guarded: []string{"foo."},
		},
		{
			// matchPrefix turns the entry into a prefix, exceptions and all.
			name:        "match prefix mode",
			rules:       []Rule{{Pattern: "foo", Except: []string{"foo.keep"}}},
			matchPrefix: true,
			guarded:     []string{"foo"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewRuleMatcher(c.rules, c.matchPrefix)
			assert.Equal(t, c.exact, m.exact, "exact entries")
			assert.Equal(t, c.prefixes, m.prefixes, "prefix entries")
			assert.Equal(t, c.guarded, m.guarded, "guarded prefix entries")
			require.Len(t, m.guards, len(m.guarded), "one guard per guarded entry")

			nested := 0
			for i, entry := range m.guarded {
				assert.Equal(t, c.nested[entry], m.guards[i].nested.guarded, "entries nested under %q", entry)
				nested += len(c.nested[entry])

				except := c.except[entry]
				if except == 0 {
					except = 1
				}
				assert.Len(t, m.guards[i].except, except, "exceptions of %q", entry)
			}

			assert.Equal(t, len(c.exact)+len(c.prefixes)+len(c.guarded)+nested, m.Len())
		})
	}
}

func TestIsStringMatchingExceptions(t *testing.T) {
	cases := []struct {
		name   string
		rules  []Rule
		match  []string
		passes []string
	}{
		{
			name: "exact exceptions",
			rules: []Rule{{
				Pattern: "redis.*",
				Except:  []string{"redis.net.commands", "redis.mem.used"},
			}},
			match:  []string{"redis.", "redis.net", "redis.net.commands.rate", "redis.mem.use"},
			passes: []string{"redis.net.commands", "redis.mem.used", "redi", "other"},
		},
		{
			// An exception can itself be a prefix pattern.
			name: "prefix exception",
			rules: []Rule{{
				Pattern: "redis.*",
				Except:  []string{"redis.keys.*"},
			}},
			match:  []string{"redis.net", "redis.key", "redis.keys"},
			passes: []string{"redis.keys.", "redis.keys.count"},
		},
		{
			// Exceptions never cross rule boundaries: another rule matching
			// the same name still matches it.
			name: "exception overridden by another rule",
			rules: []Rule{
				{Pattern: "foo.*", Except: []string{"foo.keep", "foo.bar.keep"}},
				{Pattern: "foo.bar.*"},
				{Pattern: "foo.keep"},
			},
			match:  []string{"foo.other", "foo.keep", "foo.bar.keep", "foo.bar.x"},
			passes: []string{"foo", "other"},
		},
		{
			// Nested guarded prefixes: each applies its own exceptions, so a
			// name is only let through when every matching rule excepts it.
			name: "nested guarded prefixes",
			rules: []Rule{
				{Pattern: "a.*", Except: []string{"a.keep", "a.b.keep"}},
				{Pattern: "a.b.*", Except: []string{"a.b.keep", "a.b.other"}},
			},
			match:  []string{"a.x", "a.b.x", "a.b.other"},
			passes: []string{"a.keep", "a.b.keep"},
		},
		{
			// Two rules on the same prefix: a name has to be excepted by both
			// to be let through.
			name: "same prefix twice",
			rules: []Rule{
				{Pattern: "foo.*", Except: []string{"foo.a", "foo.b"}},
				{Pattern: "foo.*", Except: []string{"foo.b", "foo.c"}},
			},
			match:  []string{"foo.a", "foo.c", "foo.d"},
			passes: []string{"foo.b"},
		},
		{
			// A bare `*` with exceptions is an allow-list.
			name:   "match all except",
			rules:  []Rule{{Pattern: "*", Except: []string{"keep.*", "exactly.this"}}},
			match:  []string{"", "anything", "kee", "exactly.that"},
			passes: []string{"keep.", "keep.this", "exactly.this"},
		},
		{
			// An unconditional prefix wins over the exceptions of a longer
			// rule it covers.
			name: "unconditional prefix beats a longer exception",
			rules: []Rule{
				{Pattern: "foo.*"},
				{Pattern: "foo.bar.*", Except: []string{"foo.bar.keep"}},
			},
			match:  []string{"foo.bar.keep", "foo.bar.x", "foo.x"},
			passes: []string{"foo", "other"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewRuleMatcher(c.rules, false)
			for _, name := range c.match {
				assert.True(t, m.Test(name), "%q must match", name)
			}
			for _, name := range c.passes {
				assert.False(t, m.Test(name), "%q must not match", name)
			}
		})
	}
}

// naiveTest evaluates the rules the way they read, one after the other,
// without any of the compaction and searching `Matcher` relies on.
func naiveTest(rules []Rule, matchPrefix bool, name string) bool {
	matches := func(pattern, name string) bool {
		if entry, isPrefix := stdstrings.CutSuffix(pattern, PrefixSuffix); isPrefix {
			return stdstrings.HasPrefix(name, entry)
		}
		return pattern == name
	}

	for _, rule := range rules {
		// `matchPrefix` applies to the rule's own pattern only, never to its
		// exceptions.
		if matchPrefix {
			entry, _ := stdstrings.CutSuffix(rule.Pattern, PrefixSuffix)
			if !stdstrings.HasPrefix(name, entry) {
				continue
			}
		} else if !matches(rule.Pattern, name) {
			continue
		}

		if !slices.ContainsFunc(rule.Except, func(except string) bool {
			return matches(except, name)
		}) {
			return true
		}
	}

	return false
}

// TestNewRuleMatcherAgainstNaive cross-checks the compiled matcher against a
// direct evaluation of the same rules, on random lists drawn from a tiny
// alphabet so that prefixes, nesting and exceptions overlap constantly.
func TestNewRuleMatcherAgainstNaive(t *testing.T) {
	// Short names over {a, b, .} produce entries that are prefixes of each
	// other far more often than realistic metric names would.
	letters := []string{"a", "b", "."}
	randName := func(rnd *rand.Rand) string {
		var sb stdstrings.Builder
		for range rnd.Intn(5) {
			sb.WriteString(letters[rnd.Intn(len(letters))])
		}
		return sb.String()
	}
	randPattern := func(rnd *rand.Rand) string {
		name := randName(rnd)
		if rnd.Intn(2) == 0 {
			return name + PrefixSuffix
		}
		return name
	}

	// Counted so that the test cannot silently degenerate into a list of names
	// that nothing matches.
	matched := 0

	for seed := range 200 {
		rnd := rand.New(rand.NewSource(int64(seed)))

		rules := make([]Rule, rnd.Intn(8))
		for i := range rules {
			rules[i].Pattern = randPattern(rnd)
			rules[i].Except = make([]string, rnd.Intn(4))
			for j := range rules[i].Except {
				rules[i].Except[j] = randPattern(rnd)
			}
		}

		for _, matchPrefix := range []bool{false, true} {
			m := NewRuleMatcher(rules, matchPrefix)
			for range 40 {
				name := randName(rnd)
				expected := naiveTest(rules, matchPrefix, name)
				if expected {
					matched++
				}
				assert.Equal(t, expected, m.Test(name),
					"seed %d, matchPrefix %v, name %q, rules %+v", seed, matchPrefix, name, rules)
			}
		}
	}

	assert.Greater(t, matched, 1000, "the generated names barely match anything")
}

// TestNewRuleMatcherEquivalentToNewMatcher pins that rules without exceptions
// compile exactly like the plain string list they came from, so adding the
// feature cannot change the behaviour of a list that does not use it.
func TestNewRuleMatcherEquivalentToNewMatcher(t *testing.T) {
	list := []string{"foo.bar", "foo.baz.*", "zzz", "app.*", "app.metrics.*", "foo.baz.qux", "*.literal"}

	rules := make([]Rule, 0, len(list))
	for _, entry := range list {
		rules = append(rules, Rule{Pattern: entry})
	}

	for _, matchPrefix := range []bool{false, true} {
		assert.Equal(t, NewMatcher(list, matchPrefix), NewRuleMatcher(rules, matchPrefix),
			"matchPrefix=%v", matchPrefix)
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
