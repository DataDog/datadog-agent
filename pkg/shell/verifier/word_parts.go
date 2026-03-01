// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import (
	"mvdan.cc/sh/v3/syntax"
)

// verifyWord recursively checks all parts of a Word for disallowed constructs.
func (v *verifier) verifyWord(w *syntax.Word) {
	if w == nil {
		return
	}
	for _, part := range w.Parts {
		v.verifyWordPart(part)
	}
}

// verifyWordPart checks a single WordPart for safety.
func (v *verifier) verifyWordPart(wp syntax.WordPart) {
	switch p := wp.(type) {
	case *syntax.Lit:
		// Literal strings are always safe.

	case *syntax.SglQuoted:
		// Single-quoted strings are always safe (no expansion).

	case *syntax.DblQuoted:
		// Double-quoted strings: recurse into their parts.
		for _, part := range p.Parts {
			v.verifyWordPart(part)
		}

	case *syntax.ParamExp:
		// Parameter expansion ($VAR, ${VAR:-default}, etc.) is allowed.
		// Recurse into all embedded words that could contain command substitutions.
		if p.Exp != nil {
			v.verifyWord(p.Exp.Word)
		}
		if p.Repl != nil {
			v.verifyWord(p.Repl.Orig)
			v.verifyWord(p.Repl.With)
		}
		if p.Slice != nil {
			v.verifyArithmExpr(p.Slice.Offset)
			v.verifyArithmExpr(p.Slice.Length)
		}

	case *syntax.ArithmExp:
		// Arithmetic expansion $((expr)) — recurse to catch embedded command substitutions.
		v.verifyArithmExpr(p.X)

	case *syntax.CmdSubst:
		// Command substitution $(cmd) and backtick `cmd` are forbidden.
		v.addViolation(p.Pos(), "shell_feature", "command substitution ($(...) or backticks) is not allowed")

	case *syntax.ProcSubst:
		// Process substitution <(cmd) and >(cmd) are forbidden.
		v.addViolation(p.Pos(), "shell_feature", "process substitution (<(...) or >(...)) is not allowed")

	case *syntax.ExtGlob:
		// Extended glob patterns are safe.

	case *syntax.BraceExp:
		// Brace expansion {a,b,c} is safe.

	default:
		// Unknown word part type — reject for safety.
		v.addViolation(wp.Pos(), "shell_feature", "unsupported word part type")
	}
}

// verifyArithmExpr recursively walks arithmetic expressions to catch embedded
// command substitutions (e.g., $(($(whoami)))).
func (v *verifier) verifyArithmExpr(expr syntax.ArithmExpr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *syntax.BinaryArithm:
		v.verifyArithmExpr(e.X)
		v.verifyArithmExpr(e.Y)
	case *syntax.UnaryArithm:
		v.verifyArithmExpr(e.X)
	case *syntax.ParenArithm:
		v.verifyArithmExpr(e.X)
	case *syntax.Word:
		v.verifyWord(e)
	}
}

// literalWordValue extracts the string value from a literal word.
// Returns ("", false) if the word contains non-literal parts.
func literalWordValue(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	var val string
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			val += p.Value
		case *syntax.SglQuoted:
			val += p.Value
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				val += lit.Value
			}
		default:
			return "", false
		}
	}
	return val, true
}
