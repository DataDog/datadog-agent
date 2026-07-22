// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
)

// expressionReferencesRedacted returns the full expression path leading to the
// first redacted identifier referenced by the expression, if any. For example,
// obj.password returns "obj.password" and m["password"] returns m["password"].
// It inspects variable references, member names, and string-literal map index
// keys.
//
// It guards two cases the decoder cannot scrub from output. A condition is
// compiled to eBPF and gates whether a snapshot fires, so reading a redacted
// value leaks it through the fire/no-fire signal; probes with such conditions
// are rejected. A capture or template expression is named in the snapshot by
// its display name rather than its source, so the decoder cannot tell it reads
// a redacted value; such expressions are marked here and dropped by the
// decoder. This mirrors the Java and Python tracers, which refuse to evaluate
// a redacted identifier in the expression language.
func expressionReferencesRedacted(expr exprlang.Expr, cfg *redaction.Config) (string, bool) {
	if cfg == nil || expr == nil {
		return "", false
	}
	var found string
	checkPath := func(name, path string) {
		if found == "" && cfg.RedactIdentifier(name) {
			found = path
		}
	}
	literalString := func(e exprlang.Expr) (string, bool) {
		lit, ok := e.(*exprlang.LiteralExpr)
		if !ok {
			return "", false
		}
		s, ok := lit.Value.(string)
		return s, ok
	}
	exprlang.Rewrite(expr, func(e exprlang.Expr) exprlang.Expr {
		switch n := e.(type) {
		case *exprlang.RefExpr:
			checkPath(n.Ref, n.Ref)
		case *exprlang.GetMemberExpr:
			checkPath(n.Member, exprString(n))
		case *exprlang.IndexExpr:
			if s, ok := literalString(n.Index); ok {
				checkPath(s, exprString(n))
			}
		case *exprlang.ContainsExpr:
			if s, ok := literalString(n.Key); ok {
				checkPath(s, s)
			}
		}
		return nil
	})
	return found, found != ""
}

// exprString returns a human-readable string for simple path expressions.
// Non-path nodes (e.g. comparisons) are rendered as "...".
func exprString(e exprlang.Expr) string {
	switch n := e.(type) {
	case *exprlang.RefExpr:
		return n.Ref
	case *exprlang.GetMemberExpr:
		return exprString(n.Base) + "." + n.Member
	case *exprlang.IndexExpr:
		if lit, ok := n.Index.(*exprlang.LiteralExpr); ok {
			if s, ok := lit.Value.(string); ok {
				return exprString(n.Base) + `["` + s + `"]`
			}
		}
		return exprString(n.Base) + "[...]"
	default:
		return "..."
	}
}
