// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Go vet raise an error when test the "Warn" method: call has possible formatting directive %s
// +build !dovet

package log

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
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

// createExtraTextContext defines custom formatter for context logging on tests.
func createExtraTextContext(string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, _ := context.CustomContext().([]interface{})
		builder := strings.Builder{}
		for i := 0; i < len(contextList); i += 2 {
			builder.WriteString(fmt.Sprintf("%s:%v", contextList[i], contextList[i+1]))
			if i != len(contextList)-2 {
				builder.WriteString(", ")
			} else {
				builder.WriteString(" | ")
			}
		}
		return builder.String()
	}
}

func TestBasicLogging(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %ExtraTextContext%Msg\n")
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

	Trace("bar")
	Debug("bar")
	Info("bar")
	Wn("bar")
	Err("bar")
	Crt("bar")
	w.Flush()

	// Trace will not be logged
	assert.Equal(t, strings.Count(b.String(), "bar"), 5)

	Tracec("baz", "number", 1, "str", "hello")
	Debugc("baz", "number", 1, "str", "hello")
	Infoc("baz", "number", 1, "str", "hello")
	Warnc("baz", "number", 1, "str", "hello")
	Errorc("baz", "number", 1, "str", "hello")
	Criticalc("baz", "number", 1, "str", "hello")
	w.Flush()

	// Trace will not be logged
	assert.Subset(t, strings.Split(b.String(), "\n"), []string{
		"[DEBUG] TestBasicLogging: number:1, str:hello | baz",
		"[INFO] TestBasicLogging: number:1, str:hello | baz",
		"[WARN] TestBasicLogging: number:1, str:hello | baz",
		"[ERROR] TestBasicLogging: number:1, str:hello | baz",
		"[CRITICAL] TestBasicLogging: number:1, str:hello | baz",
	})
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
func TestLogBufferWithContext(t *testing.T) {
	// reset buffer state
	logsBuffer = []func(){}
	bufferLogsBeforeInit = true
	logger = nil

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	Tracec("baz", "number", 1, "str", "hello")
	Debugc("baz", "number", 1, "str", "hello")
	Infoc("baz", "number", 1, "str", "hello")
	Warnc("baz", "number", 1, "str", "hello")
	Errorc("baz", "number", 1, "str", "hello")
	Criticalc("baz", "number", 1, "str", "hello")

	SetupLogger(l, "debug")
	assert.NotNil(t, logger)
	w.Flush()

	// Trace will not be logged, Error and Critical will directly be logged to Stderr
	assert.Equal(t, strings.Count(b.String(), "baz"), 5)
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

func TestFormatErrorcScrubbing(t *testing.T) {
	err := formatErrorc("aaaaaaaaaaaaaaaaaaaaaaaaaaabaaaa")
	assert.Equal(t, "***************************baaaa", err.Error())

	err = formatErrorc("API key test", "key", "aaaaaaaaaaaaaaaaaaaaaaaaaaabaaaa")
	assert.Equal(t, "API key test (key:***************************baaaa)", err.Error())

	err = formatErrorc("API key test", "key", "aaaaaaaaaaaaaaaaaaaaaaaaaaabaaaa", "key2", "val2")
	assert.Equal(t, "API key test (key:***************************baaaa, key2:val2)", err.Error())
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

func TestWarncNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Warnc("test", "key", "val"))

	seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.CriticalLvl, "[%LEVEL] %FuncShort: %ExtraTextContext%Msg")
	SetupLogger(l, "critical")

	assert.NotNil(t, Warnc("test", "key", "val"))

	changeLogLevel("info")

	assert.NotNil(t, Warnc("test", "key", "val"))
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

func TestErrorcNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Errorc("test", "key", "val"))

	seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.CriticalLvl, "[%LEVEL] %FuncShort: %ExtraTextContext%Msg")
	SetupLogger(l, "critical")

	assert.NotNil(t, Errorc("test", "key", "val"))

	changeLogLevel("info")

	assert.NotNil(t, Errorc("test", "key", "val"))
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

func TestCriticalcNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Criticalc("test", "key", "val"))

	seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.InfoLvl, "[%LEVEL] %FuncShort: %ExtraTextContext%Msg")
	SetupLogger(l, "info")

	assert.NotNil(t, Criticalc("test", "key", "val"))
}
