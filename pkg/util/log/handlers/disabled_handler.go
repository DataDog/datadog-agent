// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"log/slog"
)

var _ slog.Handler = disabled{}

// disabled is a slog handler which never writes anything.
//
// This can be replaced by slog.Disabled once we update to Go 1.25
type disabled struct{}

// NewDisabledHandler returns a handler which never writes anything.
func NewDisabledHandler() slog.Handler {
	return disabled{}
}

// Enabled returns false
func (disabled) Enabled(context.Context, slog.Level) bool {
	return false
}

// Handle does nothing
func (disabled) Handle(context.Context, slog.Record) error {
	return nil
}

// WithAttrs does nothing
func (d disabled) WithAttrs([]slog.Attr) slog.Handler {
	return d
}

// WithGroup does nothing
func (d disabled) WithGroup(string) slog.Handler {
	return d
}
