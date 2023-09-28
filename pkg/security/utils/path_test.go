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

func TestPathPatternBuilder(t *testing.T) {
	tests := []struct {
		Pattern         string
		Path            string
		Opts            PathPatternBuilderOpts
		ExpectedResult  bool
		ExpectedPattern string
	}{
		{
			Pattern:         "/etc/passwd",
			Path:            "/etc/passwd",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/etc/passwd",
		},
		{
			Pattern:         "/bin/baz",
			Path:            "/bin/baz2",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/bin/*",
		},
		{
			Pattern:         "/abc/12312/sad",
			Path:            "/abc/51231",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/abc/12312/sad/",
			Path:            "/abc/51231",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/abc/12312/sad/",
			Path:            "/abc/51231/",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/abc/12312/sad",
			Path:            "/abc/51231/",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/abc/12312",
			Path:            "/abc/51231/sad",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/abc/12312",
			Path:            "/abc/51231/sad/",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/abc/12312/",
			Path:            "/abc/51231/sad/",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/abc/12312/",
			Path:            "/abc/51231/sad",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/12312",
			Path:            "/51231",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/*",
		},
		{
			Pattern:         "12312",
			Path:            "51231",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "",
			Path:            "",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/bin/baz2",
			Path:            "/bin/baz",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/bin/*",
		},
		{
			Pattern:         "/etc/http",
			Path:            "/etc/passwd",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/etc/*",
		},
		{
			Pattern:         "/var/run/1234/runc.pid",
			Path:            "/var/run/54321/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/runc.pid",
		},
		{
			Pattern:         "/var/run/12345/runc.pid",
			Path:            "/var/run/5432/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/runc.pid",
		},
		{
			Pattern:         "/var/run/12345/12345/runc.pid",
			Path:            "/var/run/54321/54321/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var/run/12345/12345/runc.pid",
			Path:            "/var/run/54321/54321/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 2},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/*/runc.pid",
		},
		{
			Pattern:         "/12345/12345/runc.pid",
			Path:            "/54321/12345/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/*/12345/runc.pid",
		},
		{
			Pattern:         "/var/runc/12345",
			Path:            "/var/runc/54321",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/runc/*",
		},
		{
			Pattern:         "/var/runc12345",
			Path:            "/var/runc54321",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/*",
		},
		{
			Pattern:         "/var/run/12345/runc.pid",
			Path:            "/var/run/12/45/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var/run/66/45/runc.pid",
			Path:            "/var/run/12345/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var/run/*/runc.pid",
			Path:            "/var/run/12345/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/runc.pid",
		},
		{
			Pattern:         "/var/*/54321/runc.pid",
			Path:            "/var/run/12345/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var/*/54321/runc.pid",
			Path:            "/var/run/12345/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 2},
			ExpectedResult:  true,
			ExpectedPattern: "/var/*/*/runc.pid",
		},
		{
			Pattern:         "/*/run/runc.pid",
			Path:            "/var/run/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 2},
			ExpectedResult:  true,
			ExpectedPattern: "/*/run/runc.pid",
		},
		{
			Pattern:         "/var/run/*",
			Path:            "/var/run/56789",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*",
		},
		{
			Pattern:         "/var/run/1234/runc.pid",
			Path:            "/var/run/12345/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/runc.pid",
		},
		{
			Pattern:         "/var/run/1234/runc.pid",
			Path:            "/var/run/4321/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, PrefixNodeRequired: 2},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/runc.pid",
		},
		{
			Pattern:         "/var/run/sdfgh/runc.pid",
			Path:            "/var/run/hgfds/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, PrefixNodeRequired: 3},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var/run/1234/runc.pid",
			Path:            "/var/run/4321/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, PrefixNodeRequired: 2, SuffixNodeRequired: 1},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/runc.pid",
		},
		{
			Pattern:         "/var/run/1234/runc.pid",
			Path:            "/var/run/4321/runc.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, PrefixNodeRequired: 2, SuffixNodeRequired: 2},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var",
			Path:            "/var",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, PrefixNodeRequired: 2},
			ExpectedResult:  true,
			ExpectedPattern: "/var",
		},
		{
			Pattern:         "/var",
			Path:            "/var",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, SuffixNodeRequired: 2},
			ExpectedResult:  true,
			ExpectedPattern: "/var",
		},
		{
			Pattern:         "/var/run/1234/http.pid",
			Path:            "/var/run/4321/http.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, NodeSizeLimit: 10},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var/run/1234/mysql.pid",
			Path:            "/var/run/4321/mysql.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, NodeSizeLimit: 4},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/mysql.pid",
		},
		{
			Pattern:         "/var/run/*/nginx.pid",
			Path:            "/var/run/4321/nginx.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, NodeSizeLimit: 10},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/var/run/*/samba.pid",
			Path:            "/var/run/4321/samba.pid",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, NodeSizeLimit: 4},
			ExpectedResult:  true,
			ExpectedPattern: "/var/run/*/samba.pid",
		},
		{
			Pattern:         "/bin/baz2",
			Path:            "/bin/baz",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, NodeSizeLimit: 6},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/bin/*",
			Path:            "/bin/baz",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, NodeSizeLimit: 6},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/bin/*",
			Path:            "/bin/baz",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, NodeSizeLimit: 2},
			ExpectedResult:  true,
			ExpectedPattern: "/bin/*",
		},
		{
			Pattern:         "/bin/baz2",
			Path:            "/bin/baz",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, PrefixNodeRequired: 2},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
		{
			Pattern:         "/bin/baz2",
			Path:            "/bin/baz",
			Opts:            PathPatternBuilderOpts{WildcardLimit: 1, SuffixNodeRequired: 1},
			ExpectedResult:  false,
			ExpectedPattern: "",
		},
	}

	for _, test := range tests {
		t.Run("test", func(t *testing.T) {
			r, p := PathPatternBuilder(test.Pattern, test.Path, test.Opts)
			assert.Equal(t, test.ExpectedResult, r, "%s vs %s", test.Pattern, test.Path)
			assert.Equal(t, test.ExpectedPattern, p, "%s vs %s", test.Pattern, test.Path)
		})
	}
}

func BenchmarkPathPatternBuilder(b *testing.B) {
	b.Run("pattern", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			PathPatternBuilder("/var/run/1234/runc.pid", "/var/run/54321/runc.pid", PathPatternBuilderOpts{WildcardLimit: 1, PrefixNodeRequired: 2, SuffixNodeRequired: 2})
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
