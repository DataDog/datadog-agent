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
	// create buffer to capture logs from the wrapper
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, DebugLvl, "[%LEVEL] %FuncShort: %Msg\n")
	SetupLogger(l, DebugStr)

	// log messages using the wrapper
	wrapper := NewWrapper(3)

	wrapper.Trace("wrapper message")
	wrapper.Tracef("wrapper message %d", 123)

	wrapper.Debug("wrapper message")
	wrapper.Debugf("wrapper message %d", 123)

	wrapper.Info("wrapper message")
	wrapper.Infof("wrapper message %d", 123)

	wrapper.Warn("wrapper message")
	wrapper.Warnf("wrapper message %d", 123)

	wrapper.Error("wrapper message")
	wrapper.Errorf("wrapper message %d", 123)

	wrapper.Critical("wrapper message")
	wrapper.Criticalf("wrapper message %d", 123)

	wrapper.Flush() // ensure all logs are flushed
	w.Flush()

	assert.Subset(t, strings.Split(b.String(), "\n"), []string{
		"[DEBUG] tRunner: wrapper message",
		"[DEBUG] tRunner: wrapper message 123",
		"[INFO] tRunner: wrapper message",
		"[INFO] tRunner: wrapper message 123",
		"[WARN] tRunner: wrapper message",
		"[WARN] tRunner: wrapper message 123",
		"[ERROR] tRunner: wrapper message",
		"[ERROR] tRunner: wrapper message 123",
		"[CRITICAL] tRunner: wrapper message",
		"[CRITICAL] tRunner: wrapper message 123",
	})
}
