// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanonicalizeGenerics(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no brackets", "encoding/json.Marshal", "encoding/json.Marshal"},
		{"simple generic", "main.typeWithGenerics[go.shape.int].Guess", "main.typeWithGenerics[...].Guess"},
		{"ptr receiver generic", "main.(*typeWithGenerics[go.shape.string]).Guess", "main.(*typeWithGenerics[...]).Guess"},
		{"nested brackets", "pkg.(*bucket[go.shape.[256]crypto/elliptic.p256Element]).add", "pkg.(*bucket[...]).add"},
		{"multiple brackets", "pkg.Foo[int].Bar[string].Baz", "pkg.Foo[...].Bar[...].Baz"},
		{
			"complex struct type param",
			`pkg.(*TimestampSpansMap).Keys[go.shape.struct { WallTime int64 "protobuf:\"varint,1\"" }].func2`,
			"pkg.(*TimestampSpansMap).Keys[...].func2",
		},
		{"generic with cross-package", "pkg.(*Set[go.shape.*github.com/foo/bar.Type]).Range", "pkg.(*Set[...]).Range"},
		// The cases below use type-like strings to show how every bracket pair
		// is rewritten. The results can look like nonsense compared to real Go
		// types; that is expected because CanonicalizeGenerics targets symbol
		// names, not type syntax (see doc comment on CanonicalizeGenerics).
		{"slice type name", "[]int", "[...]int"},
		{"map type name", "map[string]int", "map[...]int"},
		{"array type name", "[4]int", "[...]int"},
		{"slice of map", "[]map[string]int", "[...]map[...]int"},
		{"map value is slice", "map[string][]int", "map[...][...]int"},
		// [][]T is two [] pairs, each rewritten, not one nested [[...]] span.
		{"slice of slice", "[][]int", "[...][...]int"},
		{"slice of map with slice value", "[]map[string][]int", "[...]map[...][...]int"},
		{"bare name", "indexbytebody", "indexbytebody"},
		{"unmatched bracket", "broken[stuff", "broken[stuff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalizeGenerics(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchBracket(t *testing.T) {
	tests := []struct {
		s     string
		start int
		want  int
	}{
		{"[int]", 0, 4},
		{"[nested[inner]]", 0, 14},
		{"x[a]y", 1, 3},
		{"[unmatched", 0, -1},
		{"[]", 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := MatchBracket(tt.s, tt.start)
			assert.Equal(t, tt.want, got)
		})
	}
}
