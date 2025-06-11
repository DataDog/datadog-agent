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

func TestNewWrapper(t *testing.T) {
	wrapper := NewWrapper(5)
	assert.Equal(t, 5, wrapper.stackDepth, "expected wrapper stack depth to match the provided value")
}

func TestWrapperMethods(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, DebugLvl, "[%LEVEL] %Msg\n")
	SetupLogger(l, DebugStr)

	wrapper := NewWrapper(3)

	wrapper.Trace("trace message")
	wrapper.Tracef("tracef message %d", 123)
	
	wrapper.Debug("debug message")
	wrapper.Debugf("debugf message %d", 123)

	wrapper.Info("info message")
	wrapper.Infof("infof message %d", 123)

	wrapper.Warn("warn message")
	wrapper.Warnf("warnf message %d", 123)

	wrapper.Error("error message")
	wrapper.Errorf("errorf message %d", 123)

	wrapper.Critical("critical message")
	wrapper.Criticalf("criticalf message %d", 123)

	w.Flush()

	assert.Subset(t, strings.Split(b.String(), "\n"), []string{
		"[DEBUG] debug message",
		"[DEBUG] debugf message 123",
		"[INFO] info message",
		"[INFO] infof message 123",
		"[WARN] warn message",
		"[WARN] warnf message 123",
		"[ERROR] error message",
		"[ERROR] errorf message 123",
		"[CRITICAL] critical message",
		"[CRITICAL] criticalf message 123",
	})

	// reset buffer state
	logsBuffer = []func(){}
	logger.Store(nil)
}

func TestWrapperFlush(t *testing.T) {
	wrapper := NewWrapper(3)

	wrapper.Debug("debug message")

	wrapper.Flush() // should print debug log
}