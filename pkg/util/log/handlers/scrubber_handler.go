// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"log/slog"

	pkgscrubber "github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var _ slog.Handler = (*scrubber)(nil)

// scrubber is a slog handler that scrubs the message of a log record.
type scrubber struct {
	scrubber     *pkgscrubber.Scrubber
	innerHandler slog.Handler
}

// NewScrubberHandler returns a handler that scrubs the message of a log record.
func NewScrubberHandler(scrub *pkgscrubber.Scrubber, innerHandler slog.Handler) slog.Handler {
	return &scrubber{scrubber: scrub, innerHandler: innerHandler}
}

// Enabled returns true if the handler is enabled for the given level.
func (h *scrubber) Enabled(ctx context.Context, level slog.Level) bool {
	return h.innerHandler.Enabled(ctx, level)
}

// Handle writes a record to the innerHandler.
func (h *scrubber) Handle(ctx context.Context, r slog.Record) error {
	msg, err := h.scrubber.ScrubBytes([]byte(r.Message))
	if err == nil {
		r.Message = string(msg)
	}
	return h.innerHandler.Handle(ctx, r)
}

// WithAttrs returns a new handler with the given attributes.
func (h *scrubber) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &scrubber{
		scrubber:     h.scrubber,
		innerHandler: h.innerHandler.WithAttrs(attrs),
	}
}

// WithGroup returns a new handler with the given group name.
func (h *scrubber) WithGroup(name string) slog.Handler {
	return &scrubber{
		scrubber:     h.scrubber,
		innerHandler: h.innerHandler.WithGroup(name),
	}
}
