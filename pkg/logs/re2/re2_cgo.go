// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build re2_cgo

// Package re2 provides a CGo wrapper for Google's RE2 regular expression library.
package re2

import gre2 "github.com/DataDog/datadog-agent/pkg/logs/re2/internal/go-re2"

// Regexp is the go-re2 compiled regular expression. Under the re2_cgo
// build tag it is a type alias for the vendored go-re2 Regexp, which
// uses Google's RE2 via CGo.
type Regexp = gre2.Regexp

// Compile compiles a regular expression using the go-re2 engine.
func Compile(expr string) (*Regexp, error) {
	return gre2.Compile(expr)
}

// Match reports whether the compiled RE2 regex matches anywhere in content.
func Match(compiled *Regexp, content []byte) bool {
	return compiled.FindAllIndex(content, 1) != nil
}

// ReplaceLiteral replaces all matches of compiled in src with the literal
// replacement repl. Returns (src, false) with zero allocation when there
// is no match.
func ReplaceLiteral(compiled *Regexp, src, repl []byte) ([]byte, bool) {
	matches := compiled.FindAllIndex(src, -1)
	if matches == nil {
		return src, false
	}
	estimated := len(src) + (len(repl)-averageMatchLen(matches))*len(matches)
	if estimated < len(src) {
		estimated = len(src)
	}
	buf := make([]byte, 0, estimated)
	lastEnd := 0
	for _, m := range matches {
		buf = append(buf, src[lastEnd:m[0]]...)
		buf = append(buf, repl...)
		lastEnd = m[1]
	}
	buf = append(buf, src[lastEnd:]...)
	return buf, true
}

// ReplaceExpand replaces all matches of compiled in src using submatch
// expansion ($1, etc.) in repl. Returns (src, false) with zero allocation
// when there is no match.
func ReplaceExpand(compiled *Regexp, src, repl []byte) ([]byte, bool) {
	matches := compiled.FindAllSubmatchIndex(src, -1)
	if matches == nil {
		return src, false
	}
	buf := make([]byte, 0, len(src))
	lastEnd := 0
	for _, m := range matches {
		buf = append(buf, src[lastEnd:m[0]]...)
		buf = compiled.Expand(buf, repl, src, m)
		lastEnd = m[1]
	}
	buf = append(buf, src[lastEnd:]...)
	return buf, true
}

func averageMatchLen(matches [][]int) int {
	if len(matches) == 0 {
		return 0
	}
	total := 0
	for _, m := range matches {
		total += m[1] - m[0]
	}
	return total / len(matches)
}
