// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"log/slog"
	"time"
)

// ErrorLog is a value-typed snapshot of an error log record. It is the
// DTO that crosses the pkg/util/log -> comp/core boundary, keeping the
// foundational logger subtree free of comp/core dependencies and the
// agenttelemetry component free of log/slog leakage on its public
// interface.
//
// Producers (slog handlers under pkg/util/log) build an ErrorLog and pass
// it to a Submitter. Consumers (the agenttelemetry component) accept
// ErrorLog on their public method and translate it internally into the
// dd-go wire schema before sending.
type ErrorLog struct {
	// Time is the wall-clock instant the record was emitted.
	Time time.Time

	// Level is the slog level of the record. Implementations decide which
	// levels they forward; in practice only slog.LevelError is forwarded
	// by the current handler.
	Level slog.Level

	// Message is the formatted message string (post-Sprintf). The Agent's
	// current handler chain does not have access to the unexpanded
	// template; if template capture lands later it goes in a new field.
	Message string

	// PC is the program counter of the call site that emitted the record.
	// Resolved into "file:line" for stack_trace on the wire; also used by
	// the consumer's recursion guard to drop records that originated from
	// inside the consumer's own package.
	PC uintptr

	// Attrs are the structured attributes attached to the record (after
	// any WithAttrs/WithGroup chain on the handler has been replayed).
	Attrs []slog.Attr
}

// Submitter is the registration target for sending an ErrorLog to a
// consumer. The slog handler under pkg/util/log calls the currently
// installed Submitter (atomically loaded) on each error record.
//
// Implementations MUST be non-blocking on the hot path — the consumer is
// expected to enqueue into a bounded buffer and flush asynchronously.
// Implementations MUST be safe for concurrent calls.
//
// A nil Submitter means "errortracking is not yet configured"; callers
// must guard against this and drop the record silently.
type Submitter func(ErrorLog)
