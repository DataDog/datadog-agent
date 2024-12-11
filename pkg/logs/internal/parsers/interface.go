// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parsers supports transforming raw log "lines" into messages with some
// associated metadata (timestamp, severity, etc.).
//
// This parsing comes after "line parsing" (breaking input into multiple lines) and
// before further processing and aggregation of log messages.
package parsers

import "github.com/DataDog/datadog-agent/pkg/logs/message"

// Parser parses messages, given as a raw byte sequence, into content and metadata.
type Parser interface {
	// Parse parses a line of log input.
	Parse(message *message.Message) (*message.Message, error)

	// SupportsPartialLine returns true for sources that can have partial
	// lines. If SupportsPartialLine is true, Parse can return messages with
	// IsPartial: true
	SupportsPartialLine() bool
}
