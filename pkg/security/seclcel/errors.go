// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"

	"github.com/google/cel-go/common"
)

// ParseError reports a SECL expression that could not be translated to CEL,
// either because it is not valid SECL or because it uses a construct the CEL
// branch does not support yet.
type ParseError struct {
	// Offset is the byte offset in the expression the error refers to.
	Offset int
	// Message describes the problem.
	Message string

	// line and column are resolved from Offset once the source is known.
	line, column int
}

func (e *ParseError) Error() string {
	if e.line > 0 {
		return fmt.Sprintf("%d:%d: %s", e.line, e.column, e.Message)
	}
	return fmt.Sprintf("offset %d: %s", e.Offset, e.Message)
}

// withSource resolves the error position against src so that Error() reports a
// line and column instead of a raw offset.
func (e *ParseError) withSource(src common.Source) *ParseError {
	if loc, ok := src.OffsetLocation(int32(e.Offset)); ok {
		e.line, e.column = loc.Line(), loc.Column()+1
	}
	return e
}

func errorf(offset int, format string, args ...any) *ParseError {
	return &ParseError{Offset: offset, Message: fmt.Sprintf(format, args...)}
}
