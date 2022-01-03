// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser supports transforming raw log "lines" into messages with some
// associated metadata (timestamp, severity, etc.).
//
// This parsing comes after "line parsing" (breaking input into multiple lines) and
// before further processing and aggregation of log messages.
package parser

// Parser parses messages, given as a raw byte sequence, into content and metadata.
type Parser interface {
	// Parse parses a line of log input.  It returns 1. message content, 2.
	// severity, 3. timestamp, 4. partial, 5. error.
	Parse([]byte) ([]byte, string, string, bool, error)

	// SupportsPartialLine returns true for sources that can have partial
	// lines. If SupportsPartialLine is true, Parse can return true for the
	// partial return value
	SupportsPartialLine() bool
}
