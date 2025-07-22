// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package strings

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBlocklist(t *testing.T) {
	check := func(data []string) []string {
		b := NewBlocklist(data, true)
		return b.data
	}

	assert.Equal(t, []string{}, check([]string{}))
	assert.Equal(t, []string{"a"}, check([]string{"a"}))
	assert.Equal(t, []string{"a"}, check([]string{"a", "aa"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "aa", "b", "bb"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "b", "bb"}))
}

func TestIsStringBlocked(t *testing.T) {
	cases := []struct {
		result      bool
		name        string
		blocklist   []string
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
		t.Run(fmt.Sprintf("%v-%v-%v", c.name, c.blocklist, c.matchPrefix),
			func(t *testing.T) {
				b := NewBlocklist(c.blocklist, c.matchPrefix)
				assert.Equal(t, c.result, b.Test(c.name))
			})
	}
}

func randomString(size uint) string {
	letterBytes := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	var str string
	for range size {
		str += string(letterBytes[rand.Intn(len(letterBytes))])
	}

	return str
}

func BenchmarkBlocklist(b *testing.B) {
	words := []string{
		"foo",
		"longer.name.but.still.small",
		"very.long.string.with.some.good.amount.of.chars.for.a.metric",
		"bar",
	}
	for i := 1000; i <= 10000; i += 1000 {
		b.Run(fmt.Sprintf("blocklist-%d", i), func(b *testing.B) {
			var values []string
			for range i {
				values = append(values, randomString(50))
			}
			benchmarkBlocklist(b, words, values)
		})
	}
}

func benchmarkBlocklist(b *testing.B, words, values []string) {
	b.ReportAllocs()
	b.ResetTimer()

	// first and last will match
	words[0] = values[0]
	words[3] = values[100]

	blocklist := NewBlocklist(values, false)

	for n := 0; n < b.N; n++ {
		blocklist.Test(words[n%len(words)])
	}
}
