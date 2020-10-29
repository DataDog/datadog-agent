// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package watchdog

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

var testLogBuf bytes.Buffer

func init() {
	logger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(&testLogBuf, seelog.DebugLvl, "%Ns [%Level] %Msg")
	if err != nil {
		panic(err)
	}
	err = seelog.ReplaceLogger(logger)
	if err != nil {
		panic(err)
	}
	log.SetupLogger(logger, "INFO")
}

func TestLogOnPanicMain(t *testing.T) {
	assert := assert.New(t)

	defer func() {
		r := recover()
		assert.NotNil(r, "panic should bubble up and be trapped here")
		assert.Contains(fmt.Sprintf("%v", r),
			"integer divide by zero",
			"divide by zero panic should be forwarded")
		msg := testLogBuf.String()
		assert.Contains(msg,
			"Unexpected panic: runtime error: integer divide by zero",
			"divide by zero panic should be reported in log")
		assert.Contains(msg,
			"github.com/DataDog/datadog-agent/pkg/trace/watchdog.TestLogOnPanicMain",
			"log should contain a reference to this test func name as it displays the stack trace")
	}()
	defer LogOnPanic()
	zero := 0
	_ = 1 / zero
}

func TestLogOnPanicGoroutine(t *testing.T) {
	assert := assert.New(t)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer func() {
			r := recover()
			assert.NotNil(r, "panic should bubble up and be trapped here")
			assert.Contains(fmt.Sprintf("%v", r),
				"what could possibly go wrong?",
				"custom panic should be forwarded")
			msg := testLogBuf.String()
			assert.Contains(msg,
				"Unexpected panic: what could possibly go wrong?",
				"custom panic should be reported in log")
			assert.Contains(msg,
				"github.com/DataDog/datadog-agent/pkg/trace/watchdog.TestLogOnPanicGoroutine",
				"log should contain a reference to this test func name as it displays the stack trace")
			wg.Done()
		}()
		defer LogOnPanic()
		panic("what could possibly go wrong?")
	}()
	defer func() {
		r := recover()
		assert.Nil(r, "this should trap no error at all, what we demonstrate here is that recover needs to be called on a per-goroutine base")
	}()
	wg.Wait()
}

func TestShortErrMsg(t *testing.T) {
	assert := assert.New(t)

	expected := map[string]string{
		"exceeded max connections":   "exceeded max conn...",
		"cannot configure dogstatsd": "cannot configure ...",
		"ooops":                      "ooops",
		"0123456789abcdef":           "0123456789abcdef",
		"0123456789abcdef0":          "0123456789abcdef0",
		"0123456789abcdef01":         "0123456789abcdef0...",
		"0123456789abcdef012":        "0123456789abcdef0...",
		"0123456789abcdef0123":       "0123456789abcdef0...",
		"0123456789abcdef01234":      "0123456789abcdef0...",
		"":                           "",
		"αβγ":                        "αβγ",
	}

	for k, v := range expected {
		assert.Equal(v, shortErrMsg(k), "short error message for '%s' should be '%s'", k, v)
	}
}
