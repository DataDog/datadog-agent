// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package scrub provides a slog handler that scrubs log messages.
package scrub

import (
	"context"
	"log/slog"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// NewHandler returns a slog handler that scrubs the message of every record
// before forwarding it to innerHandler. If scrubbing fails, the message is redacted.
func NewHandler(innerHandler slog.Handler) slog.Handler {
	return &scrubHandler{
		innerHandler: innerHandler,
		scrubString:  scrubber.ScrubString,
	}
}

type scrubHandler struct {
	innerHandler slog.Handler
	scrubString  func(string) (string, error)
}

func (h *scrubHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.innerHandler.Enabled(ctx, level)
}

func (h *scrubHandler) Handle(ctx context.Context, r slog.Record) error {
	if scrubbed, err := h.scrubString(r.Message); err == nil {
		r.Message = string(scrubbed)
	} else {
		r.Message = "[REDACTED] - failure to clean the message"
	}
	return h.innerHandler.Handle(ctx, r)
}

func (h *scrubHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &scrubHandler{
		innerHandler: h.innerHandler.WithAttrs(attrs),
		scrubString:  h.scrubString,
	}
}

func (h *scrubHandler) WithGroup(name string) slog.Handler {
	return &scrubHandler{
		innerHandler: h.innerHandler.WithGroup(name),
		scrubString:  h.scrubString,
	}
}
