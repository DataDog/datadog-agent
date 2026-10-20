// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jq

import (
	"context"
	"strings"
	"testing"
)

func FuzzJQFilterParser(f *testing.F) {
	for _, seed := range []string{
		`.`, `.items[]?`, `{a: .x | . + 1}`, `(1,2) + (10,20)`,
		`[.[] | select(. >= 2)]`, `$x // empty`, `1 < 2 < 3`, `"\ud800"`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, filter string) {
		if len(filter) > MaxFilterBytes {
			return
		}
		_, _ = parseFilter(filter)
	})
}

func FuzzJQEvaluator(f *testing.F) {
	for _, seed := range []struct{ filter, input string }{
		{`.`, `null`},
		{`{(.a): empty}`, `{"a":1}`},
		{`{a:(1,2),b:(3,4)}`, `null`},
		{`[.[] | select(. >= 2)]`, `[1,2,3]`},
		{`.[] | .a // "d"`, `[{"a":1},{}]`},
		{`map(.*2)`, `[1,2,3]`},
		{`.[[1]]`, `[0,1]`},
		{`(1,2) + (10,20)`, `null`},
		{`{(.k):(1,2)}`, `{"k":"x"}`},
	} {
		f.Add(seed.filter, seed.input)
	}
	f.Fuzz(func(t *testing.T, filter, input string) {
		if len(filter) > MaxFilterBytes || len(input) > 1<<16 {
			return
		}
		root, err := parseFilter(filter)
		if err != nil {
			return
		}
		v, err := parseSingleJSON(context.Background(), input)
		if err != nil {
			return
		}
		eval := newEvaluator(context.Background(), nil)
		results, err := eval.evaluate(v, root)
		// A retention imbalance is an internal invariant violation.
		if err != nil && strings.Contains(err.Error(), "retention accounting imbalance") {
			t.Fatalf("retention imbalance: filter=%q input=%q", filter, input)
		}
		for _, result := range results {
			if _, err := encodeValue(result, false, MaxOutputBytes); err != nil {
				return
			}
		}
	})
}

func FuzzJQJSONDecoder(f *testing.F) {
	for _, seed := range []string{
		`null`, `123456789012345678901234567890`, `1e-400`, `{"b":1,"a":2,"b":3}`,
		`[true,false,null,"text"]`, `"\ud83d\ude00"`, `"\ud800"`, `{`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1<<20 {
			return
		}
		_, _ = parseSingleJSON(context.Background(), input)
	})
}
