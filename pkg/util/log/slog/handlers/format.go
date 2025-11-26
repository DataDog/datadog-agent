// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"io"
	"log/slog"
)

var _ slog.Handler = (*format)(nil)

// format is a slog handler that formats records and writes them to an io.Writer.
type format struct {
	formatter func(ctx context.Context, r slog.Record) string
	writer    io.Writer
}

// NewFormat returns a handler that formats records and writes them to an io.Writer.
func NewFormat(formatter func(ctx context.Context, r slog.Record) string, writer io.Writer) slog.Handler {
	return &format{formatter: formatter, writer: writer}
}

// Handle writes a record to the writer.
func (h *format) Handle(ctx context.Context, r slog.Record) error {
	msg := h.formatter(ctx, r)
	_, err := h.writer.Write([]byte(msg))
	return err
}

// Enabled returns true if the handler is enabled for the given level.
func (h *format) Enabled(context.Context, slog.Level) bool {
	return true
}

// WithAttrs returns a new handler with the given attributes.
func (h *format) WithAttrs(_attrs []slog.Attr) slog.Handler {
	panic("not implemented")
}

// WithGroup returns a new handler with the given group name.
func (h *format) WithGroup(_name string) slog.Handler {
	panic("not implemented")
}
