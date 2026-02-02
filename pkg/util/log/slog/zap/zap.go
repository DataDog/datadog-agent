// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package zap

import (
	"context"
	stdslog "log/slog"

	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

type zapHandler struct {
	logger *zap.Logger
}

// NewZapLogger returns a logger that writes to a zap logger.
func NewZapLogger(logger *zap.Logger) types.LoggerInterface {
	h := &zapHandler{logger: logger}
	return slog.NewWrapperWithCloseAndFlush(h, func() { logger.Sync() }, nil)
}

func (h *zapHandler) Handle(ctx context.Context, r stdslog.Record) error {
	switch types.FromSlogLevel(r.Level) {
	case types.DebugLvl:
		h.logger.Debug(r.Message)
	case types.InfoLvl:
		h.logger.Info(r.Message)
	case types.WarnLvl:
		h.logger.Warn(r.Message)
	case types.ErrorLvl:
		h.logger.Error(r.Message)
	case types.CriticalLvl:
		h.logger.Error(r.Message)
	}
	return nil
}

func (h *zapHandler) Enabled(ctx context.Context, level stdslog.Level) bool {
	return true
}

func (h *zapHandler) WithAttrs(attrs []stdslog.Attr) stdslog.Handler {
	return h
}

func (h *zapHandler) WithGroup(name string) stdslog.Handler {
	return h
}
