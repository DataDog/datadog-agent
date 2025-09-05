// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package utils

import (
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
