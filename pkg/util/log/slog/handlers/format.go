// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"io"
	"log/slog"
	"slices"
)

var _ slog.Handler = (*format)(nil)

// format is a slog handler that formats records and writes them to an io.Writer.
type format struct {
	formatter func(ctx context.Context, r slog.Record) string
	writer    io.Writer
	attrs     []slog.Attr // precomputed attributes (already wrapped in groups if needed)
	groups    []string    // currently open groups for future WithAttrs calls
}

// NewFormat returns a handler that formats records and writes them to an io.Writer.
func NewFormat(formatter func(ctx context.Context, r slog.Record) string, writer io.Writer) slog.Handler {
	return &format{formatter: formatter, writer: writer}
}

// Handle writes a record to the writer.
func (h *format) Handle(ctx context.Context, r slog.Record) error {
	// If there are open groups, record's attrs must be wrapped in those groups
	if len(h.groups) > 0 {
		// Collect record's original attrs
		var recordAttrs []slog.Attr
		r.Attrs(func(a slog.Attr) bool {
			recordAttrs = append(recordAttrs, a)
			return true
		})

		// Build final attrs: handler's precomputed attrs + wrapped record attrs
		finalAttrs := slices.Clone(h.attrs)
		if len(recordAttrs) > 0 {
			wrapped := wrapInGroups(h.groups, recordAttrs)
			finalAttrs = mergeGroupedAttr(finalAttrs, wrapped)
		}

		// Create a new record with the properly grouped attrs
		newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
		newRecord.AddAttrs(finalAttrs...)
		r = newRecord
	} else if len(h.attrs) > 0 {
		// No groups, just add handler's attrs to the record
		r.AddAttrs(h.attrs...)
	}

	msg := h.formatter(ctx, r)
	_, err := h.writer.Write([]byte(msg))
	return err
}

// Enabled returns true if the handler is enabled for the given level.
func (h *format) Enabled(context.Context, slog.Level) bool {
	return true
}

// WithAttrs returns a new handler with the given attributes.
// Per slog.Handler contract, it does not modify the receiver.
func (h *format) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	// Create a new handler
	newH := &format{
		formatter: h.formatter,
		writer:    h.writer,
		groups:    h.groups, // safe to share since groups are only appended via WithGroup
	}

	// Copy existing attrs
	newH.attrs = make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newH.attrs, h.attrs)

	// Add new attrs, wrapped in any open groups
	if len(h.groups) > 0 {
		wrapped := wrapInGroups(h.groups, attrs)
		newH.attrs = mergeGroupedAttr(newH.attrs, wrapped)
	} else {
		newH.attrs = append(newH.attrs, attrs...)
	}

	return newH
}

// WithGroup returns a new handler with the given group name.
// Per slog.Handler contract, if name is empty it returns the receiver unchanged.
func (h *format) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	// Create a new handler with the group added
	newH := &format{
		formatter: h.formatter,
		writer:    h.writer,
		attrs:     h.attrs, // safe to share since attrs are only appended via WithAttrs
	}

	// Copy groups and add new one
	newH.groups = make([]string, len(h.groups)+1)
	copy(newH.groups, h.groups)
	newH.groups[len(h.groups)] = name

	return newH
}

// wrapInGroups wraps attributes in nested groups (outermost first in groups slice).
func wrapInGroups(groups []string, attrs []slog.Attr) slog.Attr {
	// Build from innermost to outermost
	// groups[0] is outermost, groups[len-1] is innermost
	result := attrsToAny(attrs)
	for i := len(groups) - 1; i >= 0; i-- {
		result = []any{slog.Group(groups[i], result...)}
	}
	return result[0].(slog.Attr)
}

// mergeGroupedAttr merges a new grouped attr into existing attrs.
// If the new attr is a group and there's an existing group with the same key,
// the contents are merged recursively. Otherwise, the attr is appended.
func mergeGroupedAttr(existing []slog.Attr, newAttr slog.Attr) []slog.Attr {
	if newAttr.Value.Kind() != slog.KindGroup {
		return append(existing, newAttr)
	}

	for i, e := range existing {
		if e.Key == newAttr.Key && e.Value.Kind() == slog.KindGroup {
			// Found an existing group with the same key - merge contents
			existingGroup := e.Value.Group()
			newGroup := newAttr.Value.Group()

			// Merge each attr from newGroup into existingGroup
			merged := existingGroup
			for _, na := range newGroup {
				merged = mergeGroupedAttr(merged, na)
			}

			// Create a new slice with the merged group
			result := slices.Clone(existing)
			result[i] = slog.Group(e.Key, attrsToAny(merged)...)
			return result
		}
	}

	// No existing group with this key, append
	return append(existing, newAttr)
}

// attrsToAny converts a slice of slog.Attr to a slice of any for use with slog.Group.
func attrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, a := range attrs {
		result[i] = a
	}
	return result
}
