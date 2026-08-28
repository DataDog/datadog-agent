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
		return b.data
	}

	assert.Equal(t, []string{}, check([]string{}))
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

	assert.Equal(t, []string{"redis.checkpoint_"}, m.data, "prefix entry must be kept verbatim")

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
