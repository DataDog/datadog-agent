// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package errortracking

import (
	"hash/fnv"
	"log/slog"
)

// ExternalStack describes an error whose stack was captured outside the Go
// runtime, for example by an embedded interpreter. A producer passes it as a
// slog attribute value so the Handler submits the pre-formatted frames
// instead of walking the Go stack, which would only show the bridge code.
//
// Every field must already be free of user data: the producer is
// responsible for redacting frames before building the value.
type ExternalStack struct {
	// Frames is the stack in the wire shape "function\n\tfile:line +0xoffset"
	// joined with newlines, innermost frame first.
	Frames string

	// ErrorKind is the type name of the error that triggered the log, such
	// as an exception class. Empty when there is none.
	ErrorKind string

	// Source names the component the frames came from, for example an
	// integration. It is shipped as a tag and shown in the local log line,
	// so it must be a short code-determined identifier.
	Source string
}

// String returns the short source name. It is what the log formatters render
// for the attribute, so the frames never reach the local log file.
func (s ExternalStack) String() string {
	return s.Source
}

// StackProvider is implemented by errors that carry an ExternalStack. The
// logging call site extracts it with errors.As and forwards it as an
// attribute, since the error text itself is never shipped.
type StackProvider interface {
	ExternalStack() ExternalStack
}

// externalStackFromRecord returns the first ExternalStack attribute of r.
func externalStackFromRecord(r slog.Record) (ExternalStack, bool) {
	var found ExternalStack
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		v := a.Value.Resolve()
		if v.Kind() != slog.KindAny {
			return true
		}
		found, ok = v.Any().(ExternalStack)
		return !ok
	})
	return found, ok
}

// hashKey returns the Bouncer key: a FNV-1a hash of the error kind and the
// formatted frames, so distinct stacks stay distinct and the same stack is
// rate limited.
func (s ExternalStack) hashKey() uint64 {
	h := fnv.New64a()
	h.Write([]byte(s.ErrorKind))
	h.Write([]byte{0})
	h.Write([]byte(s.Frames))
	return h.Sum64()
}

// handleExternal submits r with the externally captured stack, applying the
// same Bouncer gating as Go-origin records. It is called only after the
// Submitter has been loaded and found non-nil.
func (h *Handler) handleExternal(r slog.Record, ext ExternalStack, submit Submitter) {
	var b *Bouncer
	if h.loadBouncer != nil {
		b = h.loadBouncer()
	}
	if b == nil {
		return
	}
	suppressed, count, _ := b.Observe(ext.hashKey(), r.Time)
	if suppressed {
		return
	}
	submit(ErrorLog{
		Time:        r.Time,
		Count:       count,
		ErrorKind:   ext.ErrorKind,
		Stack:       ext.Frames,
		Integration: ext.Source,
	})
}
