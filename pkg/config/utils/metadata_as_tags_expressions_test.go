// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package utils

import (
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

type scenario[T any] struct {
	name string
	in   string
	meta KubernetesMetadata
	out  T
	err  bool
}

func (s scenario[T]) evaluate(t *testing.T) {
	t.Helper()
	t.Run(s.name, func(t *testing.T) {
		program, err := newExpressionProgram[T](s.in)
		if s.err {
			require.Error(t, err)
			return
		}

		require.NoError(t, err)
		out, err := program.Eval(s.meta)
		require.NoError(t, err)
		require.Equalf(t, s.out, out, "mismatch given meta %+v", s.meta)
	})
}

func TestNewExpressionProgram(t *testing.T) {
	meta := KubernetesMetadata{
		Namespace: "application",
		Labels: map[string]string{
			"prefix": "before",
			"suffix": "after",
		},
	}
	t.Run("strings", func(t *testing.T) {
		datas := []scenario[string]{
			{
				name: "invalid expression",
				in:   `foo`,
				err:  true,
			},
			{
				name: "produces bool fails we only produce strings",
				in:   `meta != nil`,
				err:  true,
			},
			{
				name: "produces string is ok",
				in:   `"foo"`,
				out:  "foo",
			},
			{
				name: "uses labels",
				in:   `has(meta.labels.someKey) ? meta.labels.someKey : ""`,
				out:  "",
			},
			{
				name: "eval a join",
				in:   `has(meta.labels.prefix) && has(meta.labels.suffix) ? meta.labels.prefix + "_and_" + meta.labels.suffix : "DEFAULT"`,
				out:  "before_and_after",
				meta: meta,
			},
		}

		for _, s := range datas {
			s.evaluate(t)
		}
	})

	t.Run("bools", func(t *testing.T) {
		datas := []scenario[bool]{
			{
				name: "produces strings but failes we only produce bools",
				in:   `meta.namespace`,
				err:  true,
			},
			{
				name: "raw bool is fine",
				in:   `true`,
				out:  true,
			},
			{
				name: "eval has false",
				in:   `has(meta.labels.prefix) && has(meta.labels.suffix)`,
				out:  false,
			},
			{
				name: "eval has true",
				in:   `has(meta.labels.prefix) && has(meta.labels.suffix)`,
				out:  true,
				meta: meta,
			},
			{
				name: "namespace matches",
				in:   `meta.namespace in ["application", "banana"]`,
				out:  true,
				meta: meta,
			},
			{
				name: "namespace mismtatch",
				in:   `meta.namespace in ["banana"]`,
				out:  false,
				meta: meta,
			},
		}

		for _, s := range datas {
			s.evaluate(t)
		}
	})
}

func TestResourceTagExpressions(t *testing.T) {
	testData := []struct {
		name   string
		in     []tagSelectionExpressionConfig
		meta   KubernetesMetadata
		expect func(t *testing.T, out iter.Seq2[TagValue, error])
	}{
		{
			name: "no match-static tag",
			in: []tagSelectionExpressionConfig{
				{
					Tags: map[string]string{
						"service": `"foo"`,
					},
				},
			},
			expect: func(t *testing.T, out iter.Seq2[TagValue, error]) {
				i := 0
				for kv, err := range out {
					require.NoError(t, err)
					require.Equal(t, TagValue{Key: "service", Value: "foo"}, kv)
					i++
				}
				require.Equal(t, 1, i, "only one value produced")
			},
		},
		{
			name: "matches nothing",
			in: []tagSelectionExpressionConfig{
				{
					Match: `meta.namespace == "foo"`,
					Tags: map[string]string{
						"service": `"foo"`,
					},
				},
			},
			expect: func(t *testing.T, out iter.Seq2[TagValue, error]) {
				for range out {
					t.Error("should have no results")
				}
			},
		},
	}

	for _, dd := range testData {
		t.Run(dd.name, func(t *testing.T) {
			list := parseTagExpressionsList(dd.in)
			require.Equal(t, len(dd.in), len(list), "expressions parsed")

			r := ResourceTagExpressions(list)
			dd.expect(t, r.Eval(dd.meta))
		})
	}
}
