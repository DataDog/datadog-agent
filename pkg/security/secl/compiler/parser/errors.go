// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser holds a hand written parser of the SECL language.
package parser

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// Error is a syntax error.
type Error struct {
	// Pos is the position of the first character of the faulty source.
	Pos ast.Position
	// End is the offset right after the last character of the faulty source.
	// It is equal to Pos.Offset for an error at the end of the expression.
	End int
	// Msg is the description of the error: `expected operand after "==",
	// found end of expression`.
	Msg string
	// Hint is an optional suggestion to fix the error: "did you mean `==`?".
	Hint string
}

// Error renders the error on a single line: `1:19: expected ...`.
func (e *Error) Error() string {
	if e.Hint == "" {
		return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
	}
	return fmt.Sprintf("%s: %s, %s", e.Pos, e.Msg, e.Hint)
}

// Format renders the error under the line of the source it points to:
//
//	1:19: expected operand after `==`, found end of expression
//	open.file.path ==
//	                  ^
func (e *Error) Format(src string) string {
	lineStart := strings.LastIndexByte(src[:e.Pos.Offset], '\n') + 1
	lineEnd := strings.IndexByte(src[e.Pos.Offset:], '\n')
	if lineEnd < 0 {
		lineEnd = len(src)
	} else {
		lineEnd += e.Pos.Offset
	}

	width := 1
	if end := min(e.End, lineEnd); end > e.Pos.Offset {
		width = utf8.RuneCountInString(src[e.Pos.Offset:end])
	}

	var sb strings.Builder
	sb.WriteString(e.Error())
	sb.WriteByte('\n')
	sb.WriteString(src[lineStart:lineEnd])
	sb.WriteByte('\n')
	sb.WriteString(strings.Repeat(" ", e.Pos.Column-1))
	sb.WriteString(strings.Repeat("^", width))
	return sb.String()
}

// ErrorList is the list of the syntax errors of an expression, in source
// order.
type ErrorList []*Error

// Error renders the first error, and how many follow.
func (l ErrorList) Error() string {
	switch len(l) {
	case 0:
		return "no error"
	case 1:
		return l[0].Error()
	}
	return fmt.Sprintf("%s (and %d more errors)", l[0], len(l)-1)
}

// Unwrap gives `errors.As` access to each error.
func (l ErrorList) Unwrap() []error {
	errs := make([]error, len(l))
	for i, err := range l {
		errs[i] = err
	}
	return errs
}

// Format renders each error under the line of the source it points to.
func (l ErrorList) Format(src string) string {
	formatted := make([]string, len(l))
	for i, err := range l {
		formatted[i] = err.Format(src)
	}
	return strings.Join(formatted, "\n")
}

// maxErrors is the number of errors past which parsing is abandoned.
const maxErrors = 10

// bailout is the panic value abandoning parsing, recovered by the entry points
// of the package.
type bailout struct{}

// add appends an error to the list.
//
// An error is dropped when it doesn't start past the previous one: it is the
// fallout of the previous error, the parser complaining about the Illegal token
// the scanner already reported, or about a token it already complained about.
func (l *ErrorList) add(err *Error) {
	if n := len(*l); n > 0 && err.Pos.Offset <= (*l)[n-1].Pos.Offset {
		return
	}
	*l = append(*l, err)
	if len(*l) >= maxErrors {
		panic(bailout{})
	}
}

// err returns the list as an error, nil when it is empty.
func (l ErrorList) err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}
