// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package wrapper

import (
	"github.com/go-logr/logr"
	v122Logr "github.com/go-logr/logr/funcr/internal/logr"
)

var _ logr.Logger = (*loggerImpl)(nil)

// loggerImpl wraps a v1.2.2 logr.Logger struct into a struct
// that implements the v0.4.0 logr.Logr interface.
type loggerImpl struct {
	l v122Logr.Logger
}

func (l *loggerImpl) Enabled() bool {
	return l.l.Enabled()
}

func (l *loggerImpl) Info(msg string, keysAndValues ...interface{}) {
	l.l.Info(msg, keysAndValues...)
}

func (l *loggerImpl) Error(err error, msg string, keysAndValues ...interface{}) {
	l.l.Error(err, msg, keysAndValues...)
}

func (l *loggerImpl) V(level int) logr.Logger {
	return &loggerImpl{l.l.V(level)}
}

func (l *loggerImpl) WithValues(keysAndValues ...interface{}) logr.Logger {
	return &loggerImpl{l.l.WithValues(keysAndValues...)}
}

func (l *loggerImpl) WithName(name string) logr.Logger {
	return &loggerImpl{l.l.WithName(name)}
}

// Fromv122Logger wraps a v1.2.2 logr.Logger into a struct that implements
// the v0.4.0 logr.Logger interface.
func Fromv122Logger(logger v122Logr.Logger) logr.Logger {
	return &loggerImpl{logger}
}
