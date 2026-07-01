// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEscapePkg(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{"plain", "main", "main"},
		{"single_dot_in_segment", "lib.v2", "lib%2ev2"},
		{"slash_then_dot", "gopkg.in/ini.v1", "gopkg.in/ini%2ev1"},
		{"multi_segment_no_dot_last", "github.com/DataDog/datadog-agent/pkg/foo", "github.com/DataDog/datadog-agent/pkg/foo"},
		{"multi_segment_dot_last", "github.com/DataDog/datadog-agent/pkg/foo.v2", "github.com/DataDog/datadog-agent/pkg/foo%2ev2"},
		{"multiple_dots_last", "a.b.c", "a%2eb%2ec"},
		{"percent", "weird%name", "weird%25name"},
		{"quote", `q"x`, `q%22x`},
		{"empty", "", ""},
		{"trailing_dot", "pkg.", "pkg%2e"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EscapePkg(c.in)
			require.Equal(t, c.out, got)
			// Round-trip through unescapePkg.
			back, err := unescapePkg(got)
			require.NoError(t, err)
			require.Equal(t, c.in, back)
		})
	}
}

// FuzzEscapePkg asserts that unescapePkg ∘ EscapePkg is the identity:
// every input round-trips through escape and unescape unchanged. Seeds
// cover the kinds of paths we observe in real Go binaries: plain import
// paths, paths with dotted last segments, the corner cases EscapePkg
// specifically handles (control bytes, quotes, percent, high bytes),
// and a few empty/edge inputs.
func FuzzEscapePkg(f *testing.F) {
	seeds := []string{
		"",
		"main",
		"lib.v2",
		"gopkg.in/ini.v1",
		"github.com/DataDog/datadog-agent/pkg/dyninst/symdb",
		"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib.v2",
		"a.b.c",
		"a/b.c",
		"a/b.c/d",
		"weird%name",
		`q"x`,
		"pkg.",
		"\x00\x01 \x7f",
		"\xc3\xa9",            // multi-byte UTF-8 (é)
		"go.opencensus.io",    // dotted top-level
		"k8s.io/api/core/v1",  // dotted top, plain last
		"sigs.k8s.io/yaml.v1", // dotted top + dotted last
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		got := EscapePkg(s)
		back, err := unescapePkg(got)
		require.NoError(t, err, "unescapePkg failed on EscapePkg(%q)=%q", s, got)
		require.Equal(t, s, back, "round-trip mismatch for %q (escaped=%q)", s, got)
	})
}
