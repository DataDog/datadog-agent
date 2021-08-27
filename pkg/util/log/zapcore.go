// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package log

import (
	"fmt"

	"github.com/cihub/seelog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ zapcore.Core = (*core)(nil)

type core struct {
	baseEncoder *encoder
}

func (c *core) Enabled(level zapcore.Level) bool {
	var seelogLevel seelog.LogLevel
	switch level {
	case zapcore.DebugLevel:
		seelogLevel = seelog.DebugLvl
	case zapcore.InfoLevel:
		seelogLevel = seelog.InfoLvl
	case zapcore.WarnLevel:
		seelogLevel = seelog.WarnLvl
	case zapcore.ErrorLevel:
		seelogLevel = seelog.ErrorLvl
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		seelogLevel = seelog.CriticalLvl
	}
	return logger.shouldLog(seelogLevel)
}

func (c *core) With(fields []zapcore.Field) zapcore.Core {
	enc := c.baseEncoder.Clone()
	for _, f := range fields {
		f.AddTo(enc)
	}

	return &core{
		baseEncoder: enc,
	}
}

func (c *core) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c *core) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	var context []interface{}
	if len(fields) == 0 {
		// avoid copy when there are no fields
		context = c.baseEncoder.ctx
	} else {
		enc := c.baseEncoder.Clone()
		for _, f := range fields {
			f.AddTo(enc)
		}
		context = enc.ctx
	}

	// use similar format to Python checks: (file:no) | message
	message := fmt.Sprintf("(%s) | %s", entry.Caller.TrimmedPath(), entry.Message)

	switch entry.Level {
	case zapcore.DebugLevel:
		Debugc(message, context...)
	case zapcore.InfoLevel:
		Infoc(message, context...)
	// we ignore errors since these are not related to writing
	case zapcore.WarnLevel:
		_ = Warnc(message, context...)
	case zapcore.ErrorLevel:
		_ = Errorc(message, context...)
	// zap's default core panics or exits at these levels;
	// we just log them at critical level
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		_ = Criticalc(message, context...)
	}
	return nil
}

func (c *core) Sync() error { return nil }

// NewZapCore creates a new zap core that wraps the default agent log instance.
func NewZapCore() zapcore.Core {
	return &core{baseEncoder: &encoder{}}
}

// NewZapLogger creates a new zap Logger that wraps the default agent log instance.
func NewZapLogger(options ...zap.Option) *zap.Logger {
	// caller MUST be added for the core to work properly
	options = append(options, zap.AddCaller())
	return zap.New(NewZapCore(), options...)
}
