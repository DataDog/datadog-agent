// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"strings"
	"testing"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogLevel sets the logger level for this test only
func LogLevel(t testing.TB, level string) {
	t.Cleanup(func() {
		log.SetupLogger(seelog.Default, "off")
	})
	logger, err := seelog.LoggerFromCustomReceiver(testLogger{t})
	if err != nil {
		return
	}
	log.SetupLogger(logger, level)
}

type testLogger struct {
	testing.TB
}

func (t testLogger) ReceiveMessage(message string, level seelog.LogLevel, context seelog.LogContextInterface) error {
	t.Logf("%s:%d: %s | %s | %s", context.FileName(), context.Line(), context.CallTime().Format("2006-01-02 15:04:05.000 MST"), strings.ToUpper(level.String()), message)
	return nil
}

func (t testLogger) AfterParse(initArgs seelog.CustomReceiverInitArgs) error { //nolint:revive // TODO fix revive unused-parameter
	return nil
}

func (t testLogger) Flush() {
}

func (t testLogger) Close() error {
	return nil
}
