// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	slogwrapper "github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// SetupLogger configures the global logger for installer commands.
// When not called from the daemon, warn+ logs are written to stdout so they are
// visible when running the installer manually.
// In all cases, warn+ messages are captured as span tags for APM observability.
func SetupLogger(e *env.Env, span *telemetry.Span) {
	var handlers []slog.Handler

	if !e.IsFromDaemon {
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
	}

	handlers = append(handlers, &spanTagHandler{span: span})

	pkglog.SetupLogger(slogwrapper.NewWrapper(newMultiHandler(handlers...)), "warn")
}

// spanTagHandler is a slog.Handler that records warn+ log messages as span tags.
type spanTagHandler struct {
	span  *telemetry.Span
	mu    sync.Mutex
	count int
}

func (h *spanTagHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (h *spanTagHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.span.SetTag(fmt.Sprintf("log.%d", h.count), fmt.Sprintf("[%s] %s", r.Level, r.Message))
	h.count++
	return nil
}

func (h *spanTagHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *spanTagHandler) WithGroup(_ string) slog.Handler      { return h }

// multiHandler fans out log records to multiple slog.Handler implementations.
type multiHandler []slog.Handler

func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	return multiHandler(handlers)
}

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithAttrs(attrs)
	}
	return handlers
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithGroup(name)
	}
	return handlers
}
