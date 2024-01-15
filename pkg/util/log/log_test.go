// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func changeLogLevel(level string) error {
	l := logger.Load()
	if l == nil {
		return errors.New("cannot set log-level: logger not initialized")
	}

	return l.changeLogLevel(level)
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
	assert.NotNil(t, logger.Load())

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
	logger.Store(nil)

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
	assert.NotNil(t, logger.Load())

	w.Flush()

	// Trace will not be logged, Error and Critical will directly be logged to Stderr
	assert.Equal(t, strings.Count(b.String(), "foo"), 5)
}
func TestLogBufferWithContext(t *testing.T) {
	// reset buffer state
	logsBuffer = []func(){}
	logger.Store(nil)

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
	assert.NotNil(t, logger.Load())
	w.Flush()

	// Trace will not be logged, Error and Critical will directly be logged to Stderr
	assert.Equal(t, strings.Count(b.String(), "baz"), 5)
}

// Set up for scrubbing tests, by temporarily setting Scrubber; this avoids testing
// the default scrubber's functionality in this module
func setupScrubbing(t *testing.T) {
	oldScrubber := scrubber.DefaultScrubber
	scrubber.DefaultScrubber = scrubber.New()
	scrubber.DefaultScrubber.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex: regexp.MustCompile("SECRET"),
		Repl:  []byte("******"),
	})
	t.Cleanup(func() { scrubber.DefaultScrubber = oldScrubber })
}

func TestCredentialScrubbingLogging(t *testing.T) {
	setupScrubbing(t)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	SetupLogger(l, "info")
	assert.NotNil(t, logger.Load())

	Info("don't tell anyone: ", "SECRET")
	Infof("this is a SECRET password: %s", "hunter2")
	w.Flush()

	assert.Equal(t, strings.Count(b.String(), "SECRET"), 0)
	assert.Equal(t, strings.Count(b.String(), "don't tell anyone:  ******"), 1)
	assert.Equal(t, strings.Count(b.String(), "this is a ****** password: hunter2"), 1)
}

func TestExtraLogging(t *testing.T) {
	setupScrubbing(t)

	var a, b bytes.Buffer
	w := bufio.NewWriter(&a)
	wA := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	lA, err := seelog.LoggerFromWriterWithMinLevelAndFormat(wA, seelog.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)

	SetupLogger(l, "info")
	assert.NotNil(t, logger.Load())

	err = RegisterAdditionalLogger("extra", lA)
	assert.Nil(t, err)

	Info("don't tell anyone: ", "SECRET")
	Infof("this is a SECRET password: %s", "hunter2")
	w.Flush()
	wA.Flush()

	assert.Equal(t, strings.Count(a.String(), "SECRET"), 0)
	assert.Equal(t, strings.Count(a.String(), "don't tell anyone:  ******"), 1)
	assert.Equal(t, strings.Count(a.String(), "this is a ****** password: hunter2"), 1)
	assert.Equal(t, b.String(), a.String())
}

func TestFormatErrorfScrubbing(t *testing.T) {
	setupScrubbing(t)

	err := formatErrorf("%s", "a SECRET message")
	assert.Equal(t, "a ****** message", err.Error())
}

func TestFormatErrorScrubbing(t *testing.T) {
	setupScrubbing(t)

	err := formatError("a big SECRET")
	assert.Equal(t, "a big ******", err.Error())
}

func TestFormatErrorcScrubbing(t *testing.T) {
	setupScrubbing(t)

	err := formatErrorc("super-SECRET")
	assert.Equal(t, "super-******", err.Error())

	err = formatErrorc("secrets", "key", "a SECRET", "SECRET-key2", "SECRET2")
	assert.Equal(t, "secrets (key:a ******, ******-key2:******2)", err.Error())
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

func TestDebugFuncNoExecute(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.InfoLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	i := 0
	DebugFunc(func() string { i = 1; return "hello" })

	w.Flush()

	assert.Equal(t, strings.Count(b.String(), "hello"), 0)
	assert.Equal(t, i, 0)
}

func TestDebugFuncExecute(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "debug")

	i := 0
	DebugFunc(func() string {
		i = 1
		return "hello"
	})

	w.Flush()

	assert.Equal(t, 1, strings.Count(b.String(), "hello"))
	assert.Equal(t, i, 1)
}

func TestFuncVersions(t *testing.T) {
	cases := []struct {
		seelogLevel        seelog.LogLevel
		strLogLevel        string
		logFunc            func(func() string)
		expectedToBeCalled bool
	}{
		{seelog.ErrorLvl, "error", DebugFunc, false},
		{seelog.WarnLvl, "warn", DebugFunc, false},
		{seelog.InfoLvl, "info", DebugFunc, false},
		{seelog.DebugLvl, "debug", DebugFunc, true},
		{seelog.TraceLvl, "trace", DebugFunc, true},

		{seelog.TraceLvl, "trace", TraceFunc, true},
		{seelog.InfoLvl, "info", TraceFunc, false},

		{seelog.InfoLvl, "info", InfoFunc, true},
		{seelog.WarnLvl, "warn", InfoFunc, false},

		{seelog.WarnLvl, "warn", WarnFunc, true},
		{seelog.ErrorLvl, "error", WarnFunc, false},

		{seelog.ErrorLvl, "error", ErrorFunc, true},
		{seelog.CriticalLvl, "critical", ErrorFunc, false},

		{seelog.CriticalLvl, "critical", CriticalFunc, true},
	}

	for _, tc := range cases {
		var b bytes.Buffer
		w := bufio.NewWriter(&b)

		l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, tc.seelogLevel, "[%LEVEL] %FuncShort: %Msg")
		SetupLogger(l, tc.strLogLevel)

		i := 0
		tc.logFunc(func() string { i = 1; return "hello" })

		w.Flush()

		if tc.expectedToBeCalled {
			assert.Equal(t, 1, strings.Count(b.String(), "hello"), tc)
			assert.Equal(t, 1, i, tc)
		} else {
			assert.Equal(t, 0, strings.Count(b.String(), "hello"), tc)
			assert.Equal(t, 0, i, tc)
		}
	}

}

func TestStackDepthfLogging(t *testing.T) {
	const stackDepth = 1

	cases := []struct {
		seelogLevel        seelog.LogLevel
		strLogLevel        string
		expectedToBeCalled int
	}{
		{seelog.CriticalLvl, "critical", 1},
		{seelog.ErrorLvl, "error", 2},
		{seelog.WarnLvl, "warn", 3},
		{seelog.InfoLvl, "info", 4},
		{seelog.DebugLvl, "debug", 5},
		{seelog.TraceLvl, "trace", 6},
	}

	for _, tc := range cases {
		t.Run(tc.strLogLevel, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)

			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, tc.seelogLevel, "[%LEVEL] %Func: %Msg\n")
			assert.Nil(t, err)

			SetupLogger(l, tc.strLogLevel)

			TracefStackDepth(stackDepth, "%s", "foo")
			DebugfStackDepth(stackDepth, "%s", "foo")
			InfofStackDepth(stackDepth, "%s", "foo")
			WarnfStackDepth(stackDepth, "%s", "foo")
			ErrorfStackDepth(stackDepth, "%s", "foo")
			CriticalfStackDepth(stackDepth, "%s", "foo")
			w.Flush()

			assert.Equal(t, tc.expectedToBeCalled, strings.Count(b.String(), "TestStackDepthfLogging"), tc)
		})
	}
}

func mockScrubBytesWithCount(t *testing.T) *atomic.Int32 {
	oldScrubber := scrubBytesFunc
	t.Cleanup(func() { scrubBytesFunc = oldScrubber })

	counterPtr := atomic.NewInt32(0)

	scrubBytesFunc = func(msg []byte) ([]byte, error) {
		counterPtr.Add(1)
		return msg, nil
	}

	return counterPtr
}

func testFunctionsScrubbingCount(t *testing.T, funcs []any, args []any) {
	for _, fun := range funcs {
		valTy := reflect.TypeOf(fun)
		require.Equal(t, valTy.Kind(), reflect.Func)

		// get the function name to use as test name
		val := reflect.ValueOf(fun)
		fun := runtime.FuncForPC(val.Pointer())
		require.NotNil(t, fun)

		funcName := fun.Name()
		if parts := strings.Split(funcName, "/"); len(parts) > 0 {
			funcName = parts[len(parts)-1]
		}

		testName := fmt.Sprintf("Scrub Count %s", funcName)
		t.Run(testName, func(t *testing.T) {
			// create a slice of reflect.Value from the args
			reflArgs := make([]reflect.Value, 0, len(args))
			for _, arg := range args {
				reflArgs = append(reflArgs, reflect.ValueOf(arg))
			}

			counter := mockScrubBytesWithCount(t)
			val.Call(reflArgs)
			require.Equal(t, 1, int(counter.Load()))
		})
	}
}

func TestLoggerScrubbingCount(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.TraceLvl, "[%LEVEL] %FuncShort: %Msg")
	require.NoError(t, err)
	SetupLogger(l, "trace")

	// package public methods
	testFunctionsScrubbingCount(
		t,
		[]any{Trace, Debug, Info, Warn, Error, Critical},
		[]any{"a", "b"},
	)
	testFunctionsScrubbingCount(
		t,
		[]any{Tracef, Debugf, Infof, Warnf, Errorf, Criticalf},
		[]any{"a %s c", "b"},
	)
	testFunctionsScrubbingCount(
		t,
		[]any{TracefStackDepth, DebugfStackDepth, InfofStackDepth, WarnfStackDepth, ErrorfStackDepth, CriticalfStackDepth},
		[]any{1, "a %s c", "b"},
	)
	testFunctionsScrubbingCount(
		t,
		[]any{Tracec, Debugc, Infoc, Warnc, Errorc, Criticalc},
		[]any{"a b", "1 %s 3", "2"},
	)
	testFunctionsScrubbingCount(
		t,
		[]any{TracecStackDepth, DebugcStackDepth, InfocStackDepth, WarncStackDepth, ErrorcStackDepth, CriticalcStackDepth},
		[]any{"a b", 1, "1 %s 3", "2"},
	)
	testFunctionsScrubbingCount(
		t,
		[]any{TraceFunc, DebugFunc, InfoFunc, WarnFunc, ErrorFunc, CriticalFunc},
		[]any{func() string { return "a b" }},
	)

	// loggerPointer methods
	testFunctionsScrubbingCount(
		t,
		[]any{logger.trace, logger.debug, logger.info, logger.warn, logger.error, logger.critical},
		[]any{"a b"},
	)
	testFunctionsScrubbingCount(
		t,
		[]any{logger.tracef, logger.debugf, logger.infof, logger.warnf, logger.errorf, logger.criticalf},
		[]any{"a %s c", "b"},
	)
	testFunctionsScrubbingCount(
		t,
		[]any{logger.traceStackDepth, logger.debugStackDepth, logger.infoStackDepth, logger.warnStackDepth, logger.errorStackDepth, logger.criticalStackDepth},
		[]any{"a b", 1},
	)
}
