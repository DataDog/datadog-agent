// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func changeLogLevel(level LogLevel) error {
	return logger.changeLogLevel(level)
}

func TestBasicLogging(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(w, DebugLvl)
	assert.NoError(t, err)

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
		"[DEBUG] TestBasicLogging: number:1,str:hello | baz",
		"[INFO] TestBasicLogging: number:1,str:hello | baz",
		"[WARN] TestBasicLogging: number:1,str:hello | baz",
		"[ERROR] TestBasicLogging: number:1,str:hello | baz",
		"[CRITICAL] TestBasicLogging: number:1,str:hello | baz",
	})
}

func TestLogBuffer(t *testing.T) {
	// reset buffer state
	logsBuffer = []func(){}
	logger.Store(nil)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, DebugLvl)
	assert.NoError(t, err)

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

	l, err := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, DebugLvl)
	assert.NoError(t, err)

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

	l, err := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, DebugLvl)
	assert.NoError(t, err)

	SetupLogger(l, "info")
	assert.NotNil(t, logger.Load())

	Info("don't tell anyone: ", "SECRET")
	Infof("this is a SECRET password: %s", "hunter2")
	w.Flush()

	assert.Equal(t, strings.Count(b.String(), "SECRET"), 0)
	assert.Equal(t, strings.Count(b.String(), "don't tell anyone:  ******"), 1)
	assert.Equal(t, strings.Count(b.String(), "this is a ****** password: hunter2"), 1)
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

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, CriticalLvl)
	SetupLogger(l, "critical")

	assert.NotNil(t, Warn("test"))

	changeLogLevel(InfoLvl)

	assert.NotNil(t, Warn("test"))
}

func TestWarnfNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Warn("test"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, CriticalLvl)
	SetupLogger(l, "critical")

	assert.NotNil(t, Warn("test"))

	changeLogLevel(InfoLvl)

	assert.NotNil(t, Warn("test"))
}

func TestWarncNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Warnc("test", "key", "val"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(w, CriticalLvl)
	SetupLogger(l, "critical")

	assert.NotNil(t, Warnc("test", "key", "val"))

	changeLogLevel(InfoLvl)

	assert.NotNil(t, Warnc("test", "key", "val"))
}

func TestErrorNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Error("test"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, CriticalLvl)
	SetupLogger(l, "critical")

	assert.NotNil(t, Error("test"))

	changeLogLevel(InfoLvl)

	assert.NotNil(t, Error("test"))
}

func TestErrorfNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Errorf("test"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, CriticalLvl)
	SetupLogger(l, "critical")

	assert.NotNil(t, Errorf("test"))

	changeLogLevel(InfoLvl)

	assert.NotNil(t, Errorf("test"))
}

func TestErrorcNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Errorc("test", "key", "val"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(w, CriticalLvl)
	SetupLogger(l, "critical")

	assert.NotNil(t, Errorc("test", "key", "val"))

	changeLogLevel(InfoLvl)

	assert.NotNil(t, Errorc("test", "key", "val"))
}

func TestCriticalNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Critical("test"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, InfoLvl)
	SetupLogger(l, "info")

	assert.NotNil(t, Critical("test"))
}

func TestCriticalfNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Criticalf("test"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, InfoLvl)
	SetupLogger(l, "info")

	assert.NotNil(t, Criticalf("test"))
}

func TestCriticalcNotNil(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	assert.NotNil(t, Criticalc("test", "key", "val"))

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(w, InfoLvl)
	SetupLogger(l, "info")

	assert.NotNil(t, Criticalc("test", "key", "val"))
}

func TestDebugFuncNoExecute(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, InfoLvl)
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

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, DebugLvl)
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
		seelogLevel        LogLevel
		strLogLevel        string
		logFunc            func(func() string)
		expectedToBeCalled bool
	}{
		{ErrorLvl, "error", DebugFunc, false},
		{WarnLvl, "warn", DebugFunc, false},
		{InfoLvl, "info", DebugFunc, false},
		{DebugLvl, "debug", DebugFunc, true},
		{TraceLvl, "trace", DebugFunc, true},

		{TraceLvl, "trace", TraceFunc, true},
		{InfoLvl, "info", TraceFunc, false},

		{InfoLvl, "info", InfoFunc, true},
		{WarnLvl, "warn", InfoFunc, false},

		{WarnLvl, "warn", WarnFunc, true},
		{ErrorLvl, "error", WarnFunc, false},

		{ErrorLvl, "error", ErrorFunc, true},
		{CriticalLvl, "critical", ErrorFunc, false},

		{CriticalLvl, "critical", CriticalFunc, true},
	}

	for _, tc := range cases {
		var b bytes.Buffer
		w := bufio.NewWriter(&b)

		l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, tc.seelogLevel)
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
		seelogLevel        LogLevel
		strLogLevel        string
		expectedToBeCalled int
	}{
		{CriticalLvl, "critical", 1},
		{ErrorLvl, "error", 2},
		{WarnLvl, "warn", 3},
		{InfoLvl, "info", 4},
		{DebugLvl, "debug", 5},
		{TraceLvl, "trace", 6},
	}

	for _, tc := range cases {
		t.Run(tc.strLogLevel, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)

			l, err := loggerFromWriterWithMinLevelAndFormat(w, tc.seelogLevel, "[{{LEVEL}}] {{.func}}: {{.msg}}\n")
			assert.NoError(t, err)

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

func getFuncName(val reflect.Value) (string, error) {
	fun := runtime.FuncForPC(val.Pointer())
	if fun == nil {
		return "", fmt.Errorf("cannot get function name for %v", val)
	}

	funcName := fun.Name()
	if parts := strings.Split(funcName, "."); len(parts) > 0 {
		funcName = parts[len(parts)-1]
	}

	return funcName, nil
}

func TestLoggerScrubbingCount(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, TraceLvl)
	require.NoError(t, err)
	SetupLogger(l, "trace")

	testCases := []struct {
		name  string
		funcs []any
		args  []any
	}{
		// package public methods
		{
			"public functions basic",
			[]any{Trace, Debug, Info, Warn, Error, Critical},
			[]any{"a", "b"},
		},
		{
			"public functions with format",
			[]any{Tracef, Debugf, Infof, Warnf, Errorf, Criticalf},
			[]any{"a %s c", "b"},
		},
		{
			"public functions with format and stack depth",
			[]any{TracefStackDepth, DebugfStackDepth, InfofStackDepth, WarnfStackDepth, ErrorfStackDepth, CriticalfStackDepth},
			[]any{1, "a %s c", "b"},
		},
		{
			"public functions with context",
			[]any{Tracec, Debugc, Infoc, Warnc, Errorc, Criticalc},
			[]any{"a b", "1 %s 3", "2"},
		},
		{
			"public functions with context and stack depth",
			[]any{TracecStackDepth, DebugcStackDepth, InfocStackDepth, WarncStackDepth, ErrorcStackDepth, CriticalcStackDepth},
			[]any{"a b", 1, "1 %s 3", "2"},
		},
		{
			"public functions with anonymous function",
			[]any{TraceFunc, DebugFunc, InfoFunc, WarnFunc, ErrorFunc, CriticalFunc},
			[]any{func() string { return "a b" }},
		},
		// loggerPointer methods
		{
			"loggerPointer methods basic",
			[]any{logger.trace, logger.debug, logger.info, logger.warn, logger.error, logger.critical},
			[]any{"a b"},
		},
		{
			"loggerPointer methods with format",
			[]any{logger.tracef, logger.debugf, logger.infof, logger.warnf, logger.errorf, logger.criticalf},
			[]any{"a %s c", "b"},
		},
		{
			"loggerPointer methods with stack depth",
			[]any{logger.traceStackDepth, logger.debugStackDepth, logger.infoStackDepth, logger.warnStackDepth, logger.errorStackDepth, logger.criticalStackDepth},
			[]any{"a b", 1},
		},
	}

	for _, tc := range testCases {
		t.Run("scrub count "+tc.name, func(t *testing.T) {
			for _, fun := range tc.funcs {
				val := reflect.ValueOf(fun)
				funcName, err := getFuncName(val)
				if !assert.NoError(t, err) {
					continue
				}

				valTy := reflect.TypeOf(fun)
				if !assert.Equalf(t, valTy.Kind(), reflect.Func, "expected %s to be a function", funcName) {
					continue
				}

				// create a slice of reflect.Value from the args
				reflArgs := make([]reflect.Value, 0, len(tc.args))
				for _, arg := range tc.args {
					reflArgs = append(reflArgs, reflect.ValueOf(arg))
				}

				counter := mockScrubBytesWithCount(t)
				val.Call(reflArgs)
				assert.Equalf(t, 1, int(counter.Load()), "expected %s to call scrubBytesFunc once", funcName)
			}
		})
	}
}

func TestLogNilLogger(t *testing.T) {
	// reset buffer state
	logsBuffer = []func(){}

	logger.Store(nil)

	Debug("message")

	// should write to the logs buffer
	assert.Equal(t, 1, len(logsBuffer))
}

func TestLogNilInnerLogger(t *testing.T) {
	// reset buffer state
	logsBuffer = []func(){}

	SetupLogger(Default(), DebugStr)
	logger.Load().inner = nil

	Debug("message")

	// should write to the logs buffer
	assert.Equal(t, 1, len(logsBuffer))
}

func TestSetupLoggerWithUnknownLogLevel(t *testing.T) {
	SetupLogger(Default(), "unknownLogLevel")

	// providing an unknown log level sets the log level to InfoLvl
	loggerLogLevel, _ := GetLogLevel()

	assert.Equal(t, InfoLvl, loggerLogLevel)
}

func TestChangeLogLevel(t *testing.T) {
	testCases := []LogLevel{
		TraceLvl,
		DebugLvl,
		InfoLvl,
		WarnLvl,
		ErrorLvl,
		CriticalLvl,
		Off,
	}

	for _, tc := range testCases {
		t.Run("change log level to "+tc.String(), func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)

			l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, DebugLvl)

			SetupLogger(Default(), DebugStr)

			err := ChangeLogLevel(l, tc)
			assert.NoError(t, err)

			// log level should have been updated
			level, err := GetLogLevel()
			require.NoError(t, err)
			assert.Equal(t, tc, level)

			// inner logger should have been replaced
			assert.Equal(t, l, logger.Load().inner)
		})
	}
}

func TestChangeLogLevelNilLogger(t *testing.T) {
	logger.Store(nil)

	err := logger.changeLogLevel(InfoLvl)
	assert.Error(t, err)
	assert.Equal(t, "cannot change loglevel: logger not initialized", err.Error())
}

func TestChangeLogLevelNilInnerLogger(t *testing.T) {
	SetupLogger(Default(), DebugStr)
	logger.Load().inner = nil

	err := logger.changeLogLevel(InfoLvl)
	assert.Error(t, err)
	assert.Equal(t, "cannot change loglevel: logger is initialized however logger.inner is nil", err.Error())
}

func TestGetLogLevel(t *testing.T) {
	SetupLogger(Default(), WarnStr)

	level, err := GetLogLevel()
	assert.NoError(t, err)
	assert.Equal(t, WarnLvl, level)
}

func TestGetLogLevelNilLogger(t *testing.T) {
	logger.Store(nil)

	level, err := GetLogLevel()
	assert.Error(t, err)
	assert.Equal(t, InfoLvl, level)
	assert.Equal(t, "cannot get loglevel: logger not initialized", err.Error())
}

func TestGetLogLevelNilInnerLogger(t *testing.T) {
	SetupLogger(Default(), DebugStr)
	logger.Load().inner = nil

	level, err := GetLogLevel()
	assert.Error(t, err)
	assert.Equal(t, InfoLvl, level)
	assert.Equal(t, "cannot get loglevel: logger not initialized", err.Error())
}

func TestShouldLog(t *testing.T) {
	SetupLogger(Default(), TraceStr)

	testCases := []struct {
		logLevel LogLevel
		// expected results for each log level in the order
		// [TraceLvl, DebugLvl, InfoLvl, WarnLvl, ErrorLvl, CriticalLvl, Off]
		expectedShouldLog []bool
	}{
		{
			TraceLvl,
			[]bool{true, true, true, true, true, true, true},
		},
		{
			DebugLvl,
			[]bool{false, true, true, true, true, true, true},
		},
		{
			InfoLvl,
			[]bool{false, false, true, true, true, true, true},
		},
		{
			WarnLvl,
			[]bool{false, false, false, true, true, true, true},
		},
		{
			ErrorLvl,
			[]bool{false, false, false, false, true, true, true},
		},
		{
			CriticalLvl,
			[]bool{false, false, false, false, false, true, true},
		},
		{
			Off,
			[]bool{false, false, false, false, false, false, true},
		},
	}

	for _, tc := range testCases {
		t.Run("should log when log level is "+tc.logLevel.String(), func(t *testing.T) {
			changeLogLevel(tc.logLevel)

			for i, logLevel := range []LogLevel{TraceLvl, DebugLvl, InfoLvl, WarnLvl, ErrorLvl, CriticalLvl, Off} {
				shouldLog := ShouldLog(logLevel)
				expected := tc.expectedShouldLog[i]
				assert.Equal(t, expected, shouldLog, "expected ShouldLog(%s) to be %v when log level is %q", logLevel.String(), expected, tc.logLevel)
			}
		})
	}
}

func TestShouldLogNilLogger(t *testing.T) {
	logger.Store(nil)

	assert.False(t, ShouldLog(InfoLvl))
}

func TestValidateLogLevel(t *testing.T) {
	testCases := []struct {
		logLevelStr string
		expected    LogLevel
	}{
		// constant log levels
		{TraceStr, TraceLvl},
		{DebugStr, DebugLvl},
		{InfoStr, InfoLvl},
		{WarnStr, WarnLvl},
		{ErrorStr, ErrorLvl},
		{CriticalStr, CriticalLvl},
		{OffStr, Off},

		// uppercase versions
		{"TRACE", TraceLvl},
		{"Debug", DebugLvl},

		// agent5 specific "Warning" log level
		{"warning", WarnLvl},
	}

	for _, tc := range testCases {
		t.Run("validate "+tc.logLevelStr, func(t *testing.T) {
			valid, err := ValidateLogLevel(tc.logLevelStr)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, valid, "expected ValidateLogLevel(%s) to return %s", tc.logLevelStr, tc.expected)
		})
	}
}

func TestValidateLogLevelUnknownLevel(t *testing.T) {
	logLevel, err := ValidateLogLevel("unknownLogLevel")
	assert.Equal(t, Off, logLevel)
	assert.Error(t, err)
	assert.Equal(t, "unknown log level: "+strings.ToLower("unknownLogLevel"), err.Error())
}

func TestTraceNilLogger(t *testing.T) {
	// reset buffer state
	logsBuffer = []func(){}
	logger.Store(nil)

	logger.trace("message")

	// should not write to the logs buffer
	assert.Equal(t, 0, len(logsBuffer))
}
