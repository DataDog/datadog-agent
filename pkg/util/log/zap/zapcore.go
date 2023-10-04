// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package log wraps the zap logger
package log

import (
	"github.com/cihub/seelog"
	"go.uber.org/zap/zapcore"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	return log.ShouldLog(seelogLevel)
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

	const depth = 3
	switch entry.Level {
	case zapcore.DebugLevel:
		log.DebugcStackDepth(entry.Message, depth, context...)
	case zapcore.InfoLevel:
		log.InfocStackDepth(entry.Message, depth, context...)
	// we ignore errors since these are not related to writing
	case zapcore.WarnLevel:
		_ = log.WarncStackDepth(entry.Message, depth, context...)
	case zapcore.ErrorLevel:
		_ = log.ErrorcStackDepth(entry.Message, depth, context...)
	// zap's default core panics or exits at these levels;
	// we just log them at critical level
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		_ = log.CriticalcStackDepth(entry.Message, depth, context...)
	}
	return nil
}

func (c *core) Sync() error {
	log.Flush()
	return nil
}

// NewZapCore creates a new zap core that wraps the default agent log instance.
func NewZapCore() zapcore.Core {
	return &core{baseEncoder: &encoder{}}
}
