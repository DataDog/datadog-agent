// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock offers a mock for the logger.
package mock

import (
	"strings"
	"testing"

	"github.com/cihub/seelog"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	// we need this import for the seelog custom 'ShortFilePath' custom formater. We should migrate them to
	// pkg/util/log
	_ "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Mock for additional interecpting and analyzing the log messages
type Mock struct {
	w             *pkglog.Wrapper
	TraceCount    int
	DebugCount    int
	InfoCount     int
	WarnCount     int
	ErrorCount    int
	CriticalCount int
}

//nolint:revive
func (m *Mock) Trace(v ...interface{}) {
	m.TraceCount++
	m.w.Trace(v...)
}

//nolint:revive
func (m *Mock) Tracef(format string, params ...interface{}) {
	m.TraceCount++
	m.w.Tracef(format, params...)
}

//nolint:revive
func (m *Mock) Debug(v ...interface{}) {
	m.DebugCount++
	m.w.Debug(v...)
}

//nolint:revive
func (m *Mock) Debugf(format string, params ...interface{}) {
	m.DebugCount++
	m.w.Debugf(format, params...)
}

//nolint:revive
func (m *Mock) Info(v ...interface{}) {
	m.InfoCount++
	m.w.Info(v...)
}

//nolint:revive
func (m *Mock) Infof(format string, params ...interface{}) {
	m.InfoCount++
	m.w.Infof(format, params...)
}

//nolint:revive
func (m *Mock) Warn(v ...interface{}) error {
	m.WarnCount++
	m.w.Warn(v...)
	return nil
}

//nolint:revive
func (m *Mock) Warnf(format string, params ...interface{}) error {
	m.WarnCount++
	m.w.Warnf(format, params...)
	return nil
}

//nolint:revive
func (m *Mock) Error(v ...interface{}) error {
	m.ErrorCount++
	m.w.Error(v...)
	return nil
}

//nolint:revive
func (m *Mock) Errorf(format string, params ...interface{}) error {
	m.ErrorCount++
	m.w.Errorf(format, params...)
	return nil
}

//nolint:revive
func (m *Mock) Critical(v ...interface{}) error {
	m.CriticalCount++
	m.w.Critical(v...)
	return nil
}

//nolint:revive
func (m *Mock) Criticalf(format string, params ...interface{}) error {
	m.CriticalCount++
	m.w.Criticalf(format, params...)
	return nil
}

//nolint:revive
func (m *Mock) Flush() {
	m.w.Flush()
}

// tbWriter is an implementation of io.Writer that sends lines to
// testing.TB#Log.
type tbWriter struct {
	t testing.TB
}

// Write implements Writer#Write.
func (tbw *tbWriter) Write(p []byte) (n int, err error) {
	// this assumes that seelog always writes one log entry in one Write call
	msg := strings.TrimSuffix(string(p), "\n")
	tbw.t.Log(msg)
	return len(p), nil
}

// New returns a new mock for the log Component
func New(t testing.TB) log.Component {
	// Build a logger that only logs to t.Log(..)
	iface, err := seelog.LoggerFromWriterWithMinLevelAndFormat(&tbWriter{t}, seelog.TraceLvl,
		"%Date(2006-01-02 15:04:05 MST) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n")
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Cleanup(func() {
		// stop using the logger to avoid a race condition
		pkglog.ChangeLogLevel(seelog.Default, "debug")
		iface.Flush()
	})

	// install the logger into pkg/util/log
	pkglog.ChangeLogLevel(iface, "debug")

	return newMock()
}

func newMock() log.Component {
	return &Mock{
		w: pkglog.NewWrapper(2),
	}
}
