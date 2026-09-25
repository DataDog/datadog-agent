// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			// of a given name.
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

func TestIsNameMatchingExceptions(t *testing.T) {
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
			// A bare `*` with exceptions is an allow-list. `""` is not tested
			// here: it is not a metric name the intake would ever store, so
			// Matcher.Test never matches it, whatever the rules say.
			name:   "match all except",
			rules:  []Rule{{Pattern: "*", Except: []string{"keep.*", "exactly.this"}}},
			match:  []string{"anything", "kee", "exactly.that"},
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

// TestNewRuleMatcherEquivalentToNewMatcher pins that rules without exceptions
// compile exactly like the plain string list they came from, so adding the
// feature cannot change the behaviour of a list that does not use it.
func TestNewRuleMatcherEquivalentToNewMatcher(t *testing.T) {
	list := []string{"foo.bar", "foo.baz.*", "zzz", "app.*", "app.metrics.*", "foo.baz.qux", "z.literal"}

	rules := make([]Rule, 0, len(list))
	for _, entry := range list {
		rules = append(rules, Rule{Pattern: entry})
	}

	for _, matchPrefix := range []bool{false, true} {
		assert.Equal(t, NewMatcher(list, matchPrefix), NewRuleMatcher(rules, matchPrefix),
			"matchPrefix=%v", matchPrefix)
	}
}

// TestIsEmptyMatchesLenZero pins that the O(1) isEmpty check Test relies on
// agrees with Len() == 0 in every shape a Matcher can take, including guarded
// rules and the nested nested matchers a longer guarded prefix produces:
// isEmpty must never diverge from Len for Test to stay correct on an empty
// matcher while skipping Len's O(rule count) walk.
func TestIsEmptyMatchesLenZero(t *testing.T) {
	cases := []struct {
		name     string
		rules    []Rule
		nonEmpty bool
	}{
		{"nil", nil, false},
		{"exact only", []Rule{{Pattern: "foo"}}, true},
		{"prefix only", []Rule{{Pattern: "foo.*"}}, true},
		{"guarded, no nesting", []Rule{{Pattern: "foo.*", Except: []string{"foo.keep"}}}, true},
		{
			"guarded, nested",
			[]Rule{
				{Pattern: "foo.*", Except: []string{"foo.a"}},
				{Pattern: "foo.bar.*", Except: []string{"foo.bar.b"}},
			},
			true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewRuleMatcher(c.rules, false)
			assert.Equal(t, m.Len() == 0, m.isEmpty())
			assert.Equal(t, c.nonEmpty, !m.isEmpty())
		})
	}

	var nilMatcher *Matcher
	assert.True(t, nilMatcher.isEmpty())
}

func TestRestrictExactSharesGuarded(t *testing.T) {
	m := NewRuleMatcher([]Rule{
		{Pattern: "foo.avg"},
		{Pattern: "bar.avg"},
		{Pattern: "redis.*", Except: []string{"redis.keep"}},
	}, false)

	restricted := m.RestrictExact(func(name string) bool {
		return name == "foo.avg"
	})

	assert.True(t, restricted.Test("foo.avg"), "kept exact entry should still match")
	assert.False(t, restricted.Test("bar.avg"), "filtered-out exact entry should no longer match")
	// The guarded prefix is shared unchanged: it matches (and still respects
	// its own exceptions) exactly like it did before restricting.
	assert.True(t, restricted.Test("redis.mem.used"))
	assert.False(t, restricted.Test("redis.keep"))
}

// BenchmarkGuardedRulesAtScale exercises Test against a matcher holding many
// guarded (prefix + exceptions) rules alongside a large exact list, the shape
// that let Test's per-lookup cost regress to O(guarded rule count) unnoticed:
// the existing benchmarks only ever build plain exact/prefix matchers with
// NewMatcher, which have no guarded entries to walk.
func BenchmarkGuardedRulesAtScale(b *testing.B) {
	const guardedCount = 1250
	const exactCount = 5000

	rules := make([]Rule, 0, guardedCount+exactCount)
	for i := range guardedCount {
		prefix := fmt.Sprintf("smp.filterlist.bench.prefix.%06d.", i)
		rules = append(rules, Rule{
			Pattern: prefix + "*",
			Except:  []string{prefix + "keep1", prefix + "keep2*"},
		})
	}
	for i := range exactCount {
		rules = append(rules, Rule{Pattern: fmt.Sprintf("smp.filterlist.bench.exact.%06d", i)})
	}

	m := NewRuleMatcher(rules, false)

	cases := map[string]string{
		"guarded-hit":      "smp.filterlist.bench.prefix.000042.somethingnotexcepted",
		"guarded-excepted": "smp.filterlist.bench.prefix.000042.keep1",
		"exact-hit":        "smp.filterlist.bench.exact.000042",
		"miss":             "smp.filterlist.bench.nothing.here.at.all",
	}

	for name, key := range cases {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				m.Test(key)
			}
		})
	}
}
