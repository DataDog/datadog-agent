// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Go vet raise an error when test the "Warn" method: call has possible formatting directive %s
// +build !dovet

package log

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func changeLogLevel(level string) error {
	if logger == nil {
		return errors.New("cannot set log-level: logger not initialized")
	}

	return logger.changeLogLevel(level)
}

func TestBasicLogging(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	SetupLogger(l, "debug")
	assert.NotNil(t, logger)

	Tracef("%s", "foo")
	Debugf("%s", "foo")
	Infof("%s", "foo")
	Warnf("%s", "foo")
	Errorf("%s", "foo")
	Criticalf("%s", "foo")
	w.Flush()

	// Trace will not be logged
	assert.Equal(t, strings.Count(b.String(), "foo"), 5)

	// Alias to avoid go-vet false positives
	Wn := Warn
	Err := Error
	Crt := Critical

	Trace("%s", "bar")
	Debug("%s", "bar")
	Info("%s", "bar")
	Wn("%s", "bar")
	Err("%s", "bar")
	Crt("%s", "bar")
	w.Flush()

	// Trace will not be logged
	assert.Equal(t, strings.Count(b.String(), "bar"), 5)
}

func TestLogBuffer(t *testing.T) {
	// reset buffer state
	logsBuffer = []func(){}
	bufferLogsBeforeInit = true
	logger = nil

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	Tracef("%s", "foo")
	Debugf("%s", "foo")
	Infof("%s", "foo")
	Warnf("%s", "foo")
	Errorf("%s", "foo")
	Criticalf("%s", "foo")

	SetupLogger(l, "debug")
	assert.NotNil(t, logger)

	w.Flush()

	// Trace will not be logged, Error and Critical will directly be logged to Stderr
	assert.Equal(t, strings.Count(b.String(), "foo"), 5)
}

func TestCredentialScrubbingLogging(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	SetupLogger(l, "info")
	assert.NotNil(t, logger)

	Info("this is an API KEY: ", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	Infof("this is a credential encoding urI: %s", "http://user:password@host:port")
	w.Flush()

	assert.Equal(t, strings.Count(b.String(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0)
	assert.Equal(t, strings.Count(b.String(), "http://user:password@host:port"), 0)
	assert.Equal(t, strings.Count(b.String(), "***************************aaaaa"), 1)
	assert.Equal(t, strings.Count(b.String(), "http://user:********@host:port"), 1)
}

func TestExtraLogging(t *testing.T) {
	var a, b bytes.Buffer
	w := bufio.NewWriter(&a)
	wA := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	lA, err := seelog.LoggerFromWriterWithMinLevelAndFormat(wA, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	SetupLogger(l, "info")
	assert.NotNil(t, logger)

	err = RegisterAdditionalLogger("extra", lA)
	assert.Nil(t, err)

	Info("this is an API KEY: ", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	Infof("this is a credential encoding urI: %s", "http://user:password@host:port")
	w.Flush()
	wA.Flush()

	assert.Equal(t, strings.Count(a.String(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 0)
	assert.Equal(t, strings.Count(a.String(), "http://user:password@host:port"), 0)
	assert.Equal(t, strings.Count(a.String(), "***************************aaaaa"), 1)
	assert.Equal(t, strings.Count(a.String(), "http://user:********@host:port"), 1)
	assert.Equal(t, a.String(), a.String())
}

func TestFormatErrorfScrubbing(t *testing.T) {
	err := formatErrorf("%s", "aaaaaaaaaaaaaaaaaaaaaaaaaaabaaaa")
	assert.Equal(t, "***************************baaaa", err.Error())
}

func TestFormatErrorScrubbing(t *testing.T) {
	err := formatError("aaaaaaaaaaaaaaaaaaaaaaaaaaabaaaa")
	assert.Equal(t, "***************************baaaa", err.Error())
}

func TestWarnNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Warn("test"))

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.CriticalLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "critical")

	assert.NotNil(t, Warn("test"))

	changeLogLevel("info")

	assert.NotNil(t, Warn("test"))
}

func TestWarnfNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Warn("test"))

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.CriticalLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "critical")

	assert.NotNil(t, Warn("test"))

	changeLogLevel("info")

	assert.NotNil(t, Warn("test"))
}

func TestErrorNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Error("test"))

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.CriticalLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "critical")

	assert.NotNil(t, Error("test"))

	changeLogLevel("info")

	assert.NotNil(t, Error("test"))
}

func TestErrorfNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Errorf("test"))

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.CriticalLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "critical")

	assert.NotNil(t, Errorf("test"))

	changeLogLevel("info")

	assert.NotNil(t, Errorf("test"))
}

func TestCriticalNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Critical("test"))

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.InfoLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	assert.NotNil(t, Critical("test"))
}

func TestCriticalfNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Criticalf("test"))

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.InfoLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	assert.NotNil(t, Criticalf("test"))
}
