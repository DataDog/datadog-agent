// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKlogRedirectLoggerStackDepth(t *testing.T) {
	klogRedirectLogger := NewKlogRedirectLogger(2)
	assert.Equal(t, 2, klogRedirectLogger.stackDepth, "KlogRedirectLogger stack depth should match the provided value")
}

func TestKlogRedirectLoggerWrite(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, DebugLvl, "[%LEVEL] %Msg\n")
	SetupLogger(l, DebugStr)

	klogRedirectLogger := NewKlogRedirectLogger(3)

	klogRedirectLogger.Write([]byte("I0105 12:34:56.789012 threadid file:line] info log message"))
	klogRedirectLogger.Write([]byte("W0206 12:34:56.789012 threadid file:line] warning log message"))
	klogRedirectLogger.Write([]byte("E0307 12:34:56.789012 threadid file:line] error log message"))
	klogRedirectLogger.Write([]byte("F0408 12:34:56.789012 threadid file:line] critical log message"))
	klogRedirectLogger.Write([]byte("X0509 12:34:56.789012 threadid file:line] unknown level log message"))

	w.Flush()

	assert.Subset(t, strings.Split(b.String(), "\n"), []string{
		"[INFO] info log message",
		"[WARN] warning log message",
		"[ERROR] error log message",
		"[CRITICAL] critical log message",
		"[INFO] unknown level log message",
	})

	// reset buffer state
	logsBuffer = []func(){}
	logger.Store(nil)
}