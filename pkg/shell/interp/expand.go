// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"fmt"
	"path/filepath"

	"mvdan.cc/sh/v3/syntax"
)

// expandWord expands a single Word to a string.
// Only literals, single-quoted, double-quoted (treated as literal), and
// for-loop variable references are supported.
func (r *Runner) expandWord(w *syntax.Word) (string, error) {
	if w == nil {
		return "", nil
	}
	var result string
	for _, part := range w.Parts {
		s, err := r.expandWordPart(part)
		if err != nil {
			return "", err
		}
		result += s
	}
	return result, nil
}

// expandWordPart expands an individual word part.
func (r *Runner) expandWordPart(wp syntax.WordPart) (string, error) {
	switch p := wp.(type) {
	case *syntax.Lit:
		return p.Value, nil

	case *syntax.SglQuoted:
		return p.Value, nil

	case *syntax.DblQuoted:
		// Treat double-quoted strings the same as single-quoted: no expansion.
		var result string
		for _, inner := range p.Parts {
			s, err := r.expandDblQuotedPart(inner)
			if err != nil {
				return "", err
			}
			result += s
		}
		return result, nil

	case *syntax.ParamExp:
		return r.expandParamExp(p)

	case *syntax.CmdSubst:
		return "", fmt.Errorf("command substitution ($(...) or backticks) is not supported")

	case *syntax.ProcSubst:
		return "", fmt.Errorf("process substitution (<(...) or >(...)) is not supported")

	case *syntax.ArithmExp:
		return "", fmt.Errorf("arithmetic expansion ($((expr))) is not supported")

	case *syntax.BraceExp:
		return "", fmt.Errorf("brace expansion is not supported")

	case *syntax.ExtGlob:
		return "", fmt.Errorf("extended glob patterns are not supported")

	default:
		return "", fmt.Errorf("unsupported word part type: %T", wp)
	}
}

// expandDblQuotedPart expands a part inside double quotes.
func (r *Runner) expandDblQuotedPart(wp syntax.WordPart) (string, error) {
	switch p := wp.(type) {
	case *syntax.Lit:
		return p.Value, nil

	case *syntax.ParamExp:
		return r.expandParamExp(p)

	case *syntax.CmdSubst:
		return "", fmt.Errorf("command substitution ($(...) or backticks) is not supported")

	case *syntax.ArithmExp:
		return "", fmt.Errorf("arithmetic expansion ($((expr))) is not supported")

	default:
		return "", fmt.Errorf("unsupported word part in double quotes: %T", wp)
	}
}

// expandParamExp handles parameter expansion. Only for-loop variables are expanded.
func (r *Runner) expandParamExp(p *syntax.ParamExp) (string, error) {
	if p == nil {
		return "", nil
	}

	// Reject complex parameter expansions (${var:-default}, ${var/pat/rep}, etc.)
	if p.Exp != nil || p.Repl != nil || p.Slice != nil || p.Excl || p.Length || p.Width {
		return "", fmt.Errorf("complex parameter expansion (${...}) is not supported")
	}

	name := p.Param.Value

	// Only expand for-loop variables.
	if val, ok := r.vars[name]; ok {
		return val, nil
	}

	return "", fmt.Errorf("variable expansion ($%s) is not supported; only for-loop variables may be expanded", name)
}

// expandFields expands a list of words, performing glob expansion on each.
// Each word may produce multiple fields if the expanded string matches a glob pattern.
func (r *Runner) expandFields(words []*syntax.Word) ([]string, error) {
	var fields []string
	for _, w := range words {
		s, err := r.expandWord(w)
		if err != nil {
			return nil, err
		}
		// Apply glob expansion.
		globbed := r.expandGlob(s)
		fields = append(fields, globbed...)
	}
	return fields, nil
}

// expandGlob performs filepath glob expansion. If the pattern contains glob
// characters and matches files, the matches are returned. Otherwise the
// original pattern is returned unchanged.
func (r *Runner) expandGlob(pattern string) []string {
	if !hasGlobMeta(pattern) {
		return []string{pattern}
	}

	// Use the runner's directory as a base for relative patterns.
	absPattern := pattern
	if !filepath.IsAbs(pattern) {
		absPattern = filepath.Join(r.dir, pattern)
	}

	matches, err := filepath.Glob(absPattern)
	if err != nil || len(matches) == 0 {
		return []string{pattern}
	}

	// If we joined with dir, strip the prefix back off for relative patterns.
	if !filepath.IsAbs(pattern) {
		for i, m := range matches {
			rel, err := filepath.Rel(r.dir, m)
			if err == nil {
				matches[i] = rel
			}
		}
	}

	return matches
}

// hasGlobMeta returns true if the string contains unescaped glob metacharacters.
func hasGlobMeta(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '*', '?', '[':
			return true
		case '\\':
			i++ // skip escaped char
		}
	}
	return false
}

// literalWordValue extracts the string value from a word that contains only
// literal, single-quoted, or double-quoted-literal parts.
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
