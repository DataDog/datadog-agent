// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathPatternMatch(t *testing.T) {
	tests := []struct {
		Pattern        string
		Path           string
		Opts           PathPatternMatchOpts
		ExpectedResult bool
	}{
		{
			Pattern:        "/etc/passwd",
			Path:           "/etc/passwd",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/bin/baz",
			Path:           "/bin/baz2",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/abc/12312/sad",
			Path:           "/abc/51231",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/abc/12312/sad/",
			Path:           "/abc/51231",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/abc/12312/sad/",
			Path:           "/abc/51231/",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/abc/12312/sad",
			Path:           "/abc/51231/",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/abc/12312",
			Path:           "/abc/51231/sad",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/abc/12312",
			Path:           "/abc/51231/sad/",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/abc/12312/",
			Path:           "/abc/51231/sad/",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/abc/12312/",
			Path:           "/abc/51231/sad",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/12312",
			Path:           "/51231",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "12312",
			Path:           "51231",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "",
			Path:           "",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/bin/baz2",
			Path:           "/bin/baz",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/etc/http",
			Path:           "/etc/passwd",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/1234/runc.pid",
			Path:           "/var/run/54321/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/12345/runc.pid",
			Path:           "/var/run/5432/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/12345/12345/runc.pid",
			Path:           "/var/run/54321/54321/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/var/run/12345/12345/runc.pid",
			Path:           "/var/run/54321/54321/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 2},
			ExpectedResult: true,
		},
		{
			Pattern:        "/12345/12345/runc.pid",
			Path:           "/54321/12345/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/runc/12345",
			Path:           "/var/runc/54321",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/runc12345",
			Path:           "/var/runc54321",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/12345/runc.pid",
			Path:           "/var/run/12/45/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/var/run/66/45/runc.pid",
			Path:           "/var/run/12345/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: false,
		},
		{
			Pattern:        "/var/run/1234/runc.pid",
			Path:           "/var/run/12345/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/1234/runc.pid",
			Path:           "/var/run/4321/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, PrefixNodeRequired: 2},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/sdfgh/runc.pid",
			Path:           "/var/run/hgfds/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, PrefixNodeRequired: 3},
			ExpectedResult: false,
		},
		{
			Pattern:        "/var/run/1234/runc.pid",
			Path:           "/var/run/4321/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, PrefixNodeRequired: 2, SuffixNodeRequired: 1},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/1234/runc.pid",
			Path:           "/var/run/4321/runc.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, PrefixNodeRequired: 2, SuffixNodeRequired: 2},
			ExpectedResult: false,
		},
		{
			Pattern:        "/var",
			Path:           "/var",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, PrefixNodeRequired: 2},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var",
			Path:           "/var",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, SuffixNodeRequired: 2},
			ExpectedResult: true,
		},
		{
			Pattern:        "/var/run/1234/http.pid",
			Path:           "/var/run/4321/http.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, NodeSizeLimit: 10},
			ExpectedResult: false,
		},
		{
			Pattern:        "/var/run/1234/mysql.pid",
			Path:           "/var/run/4321/mysql.pid",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, NodeSizeLimit: 4},
			ExpectedResult: true,
		},
		{
			Pattern:        "/bin/baz2",
			Path:           "/bin/baz",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, NodeSizeLimit: 6},
			ExpectedResult: false,
		},
		{
			Pattern:        "/bin/baz2",
			Path:           "/bin/baz",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, PrefixNodeRequired: 2},
			ExpectedResult: false,
		},
		{
			Pattern:        "/bin/baz2",
			Path:           "/bin/baz",
			Opts:           PathPatternMatchOpts{WildcardLimit: 1, SuffixNodeRequired: 1},
			ExpectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run("test", func(t *testing.T) {
			r := PathPatternMatch(test.Pattern, test.Path, test.Opts)
			assert.Equal(t, test.ExpectedResult, r, "%s vs %s", test.Pattern, test.Path)
		})
	}
}

func BenchmarkPathPatternBuilder(b *testing.B) {
	b.Run("pattern", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			PathPatternMatch("/var/run/1234/runc.pid", "/var/run/54321/runc.pid", PathPatternMatchOpts{WildcardLimit: 1, PrefixNodeRequired: 2, SuffixNodeRequired: 2})
		}
	})

	b.Run("standard", func(b *testing.B) {
		equal := func(a, b string) bool {
			return a == b
		}

		for i := 0; i < b.N; i++ {
			equal("/var/run/1234/runc.pid", "/var/run/54321/runc.pid")
		}
	})
}
