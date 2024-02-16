// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FailLogLevel sets the logger level for this test only and only outputs if the test fails
func FailLogLevel(t testing.TB, level string) {
	t.Helper()
	inner := &failureTestLogger{TB: t}
	t.Cleanup(func() {
		t.Helper()
		log.SetupLogger(seelog.Default, "off")
		inner.outputIfFailed()
	})
	logger, err := seelog.LoggerFromCustomReceiver(inner)
	if err != nil {
		return
	}
	log.SetupLogger(logger, level)
}

type failureTestLogger struct {
	testing.TB
	logData []byte
}

// ReceiveMessage implements logger.CustomReceiver
func (l *failureTestLogger) ReceiveMessage(message string, level seelog.LogLevel, context seelog.LogContextInterface) error {
	l.logData = append(l.logData, fmt.Sprintf("%s:%d: %s | %s | %s\n", context.FileName(), context.Line(), context.CallTime().Format("2006-01-02 15:04:05.000 MST"), strings.ToUpper(level.String()), message)...)
	return nil
}

// AfterParse implements logger.CustomReceiver
func (l *failureTestLogger) AfterParse(_ seelog.CustomReceiverInitArgs) error {
	return nil
}

// Flush implements logger.CustomReceiver
func (l *failureTestLogger) Flush() {
}

func (l *failureTestLogger) outputIfFailed() {
	l.Helper()
	if l.Failed() {
		l.TB.Logf("\n%s", l.logData)
	}
	l.logData = nil
}

// Close implements logger.CustomReceiver
func (l *failureTestLogger) Close() error {
	return nil
}
