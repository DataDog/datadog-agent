// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
)

func TestExpressionReferencesRedacted(t *testing.T) {
	cfg := redaction.NewConfig(nil, nil, nil)
	ref := func(name string) *exprlang.RefExpr { return &exprlang.RefExpr{Ref: name} }
	member := func(base exprlang.Expr, m string) *exprlang.GetMemberExpr {
		return &exprlang.GetMemberExpr{Base: base, Member: m}
	}
	lit := func(v any) *exprlang.LiteralExpr { return &exprlang.LiteralExpr{Value: v} }
	index := func(base exprlang.Expr, key exprlang.Expr) *exprlang.IndexExpr {
		return &exprlang.IndexExpr{Base: base, Index: key}
	}

	redacted := map[string]exprlang.Expr{
		"bare ref":         ref("password"),
		"member access":    member(ref("user"), "password"),
		"deep member":      member(member(ref("obj"), "creds"), "token"),
		"inside eq":        &exprlang.EqExpr{Left: member(ref("u"), "secret"), Right: lit("x")},
		"string index key": index(ref("m"), lit("password")),
		"contains key":     &exprlang.ContainsExpr{Base: ref("m"), Key: lit("token")},
		"inside and": &exprlang.AndExpr{
			Left:  &exprlang.EqExpr{Left: ref("count"), Right: lit(int64(1))},
			Right: &exprlang.EqExpr{Left: index(ref("m"), lit("apikey")), Right: lit("y")},
		},
	}
	for name, expr := range redacted {
		_, ok := expressionReferencesRedacted(expr, cfg)
		require.Truef(t, ok, "%s should be flagged", name)
	}

	clean := map[string]exprlang.Expr{
		"bare ref":         ref("user"),
		"member access":    member(ref("user"), "name"),
		"eq on count":      &exprlang.EqExpr{Left: ref("count"), Right: lit(int64(1))},
		"non-secret index": index(ref("m"), lit("name")),
		// A redacted word as a compared value, not a key, is not a reference.
		"redacted literal value": &exprlang.EqExpr{Left: ref("user"), Right: lit("password")},
	}
	for name, expr := range clean {
		_, ok := expressionReferencesRedacted(expr, cfg)
		require.Falsef(t, ok, "%s should not be flagged", name)
	}

	name, ok := expressionReferencesRedacted(index(ref("m"), lit("password")), cfg)
	require.True(t, ok)
	require.Equal(t, `m["password"]`, name)

	_, ok = expressionReferencesRedacted(ref("password"), nil)
	require.False(t, ok, "nil policy redacts nothing")
	_, ok = expressionReferencesRedacted(nil, cfg)
	require.False(t, ok, "nil expression is safe")
}
