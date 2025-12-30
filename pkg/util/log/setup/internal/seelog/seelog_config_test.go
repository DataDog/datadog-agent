// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seelog provides the configuration for the logger
package seelog

import (
	"context"
	"fmt"
	stdslog "log/slog"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func callerPC() uintptr {
	pcs := make([]uintptr, 1)
	runtime.Callers(1, pcs)
	return pcs[0]
}

func TestCommonSyslogFormatter(t *testing.T) {
	pid := os.Getpid()
	procName := "seelog.test"
	if runtime.GOOS == "windows" {
		procName = "seelog.test.exe"
	}
	expected := fmt.Sprintf(`<166>%s[%d]: CORE | INFO | (pkg/util/log/setup/internal/seelog/seelog_config_test.go:23 in callerPC) | check:cpu | Done running check
`, procName, pid)

	cfg := Config{loggerName: "CORE"}
	logTime := time.Now()
	record := stdslog.Record{
		PC:      callerPC(),
		Level:   stdslog.LevelInfo,
		Message: "Done running check",
		Time:    logTime,
	}
	record.AddAttrs(stdslog.String("check", "cpu"))
	msg := cfg.commonSyslogFormatter(context.Background(), record)

	require.Equal(t, expected, msg)
}

func TestJsonSyslogFormatter(t *testing.T) {
	pid := os.Getpid()
	procName := "seelog.test"
	if runtime.GOOS == "windows" {
		procName = "seelog.test.exe"
	}
	expected := fmt.Sprintf(`<166>1 %s %d - - {"agent":"core","level":"INFO","relfile":"pkg/util/log/setup/internal/seelog/seelog_config_test.go","line":"23","msg":"Done running check","check":"cpu"}
`, procName, pid)

	cfg := Config{loggerName: "CORE", syslogRFC: true}
	logTime := time.Now()
	record := stdslog.Record{
		PC:      callerPC(),
		Level:   stdslog.LevelInfo,
		Message: "Done running check",
		Time:    logTime,
	}
	record.AddAttrs(stdslog.String("check", "cpu"))
	msg := cfg.jsonSyslogFormatter(context.Background(), record)

	require.Equal(t, expected, msg)
}
