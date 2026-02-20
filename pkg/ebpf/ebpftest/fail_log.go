// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// FailLogLevel sets the logger level for this test only and only outputs if the test fails
func FailLogLevel(t testing.TB, level string) {
	t.Helper()
	inner := &failureLogWriter{TB: t}
	t.Cleanup(func() {
		t.Helper()
		log.SetupLogger(log.Default(), "off")
		inner.outputIfFailed()
	})
	lvl, err := log.ValidateLogLevel(level)
	if err != nil {
		return
	}
	logger, err := slog.LoggerFromWriterWithMinLevelAndFormat(
		inner,
		lvl,
		"{{ShortFilePath}}:{{.line}}: {{DateTime}} | {{LEVEL}} | {{.msg}}",
	)
	if err != nil {
		return
	}
	log.SetupLogger(logger, level)
}

// failureLogWriter buffers log output and only writes it if the test fails
type failureLogWriter struct {
	testing.TB
	logData []byte
}

func (l *failureLogWriter) Write(p []byte) (n int, err error) {
	l.logData = append(l.logData, p...)
	return len(p), nil
}

func (l *failureLogWriter) outputIfFailed() {
	l.Helper()
	if l.Failed() {
		l.TB.Logf("\n%s", l.logData)
	}
	l.logData = nil
}
