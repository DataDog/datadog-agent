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

// LogLevel sets the logger level for this test only
func LogLevel(t testing.TB, level string) {
	t.Cleanup(func() {
		log.SetupLogger(log.Default(), "off")
	})
	lvl, err := log.ValidateLogLevel(level)
	if err != nil {
		return
	}
	logger, err := slog.LoggerFromWriterWithMinLevelAndFormat(
		testLogWriter{t},
		lvl,
		"{{ShortFilePath}}:{{.line}}: {{DateTime}} | {{LEVEL}} | {{.msg}}",
	)
	if err != nil {
		return
	}
	log.SetupLogger(logger, level)
}

// testLogWriter wraps testing.TB to implement io.Writer
type testLogWriter struct {
	testing.TB
}

func (t testLogWriter) Write(p []byte) (n int, err error) {
	t.Log(string(p))
	return len(p), nil
}
