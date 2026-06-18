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

// expressionReferencesRedacted returns the first redacted identifier referenced
// by an expression, if any. It inspects variable references, member names, and
// string-literal map index keys, so both obj.password and m["password"] are
// detected.
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
	check := func(s string) {
		if found == "" && cfg.RedactIdentifier(s) {
			found = s
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
			check(n.Ref)
		case *exprlang.GetMemberExpr:
			check(n.Member)
		case *exprlang.IndexExpr:
			if s, ok := literalString(n.Index); ok {
				check(s)
			}
		case *exprlang.ContainsExpr:
			if s, ok := literalString(n.Key); ok {
				check(s)
			}
		}
		return nil
	})
	return found, found != ""
}
