package log

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestBasicLogging(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	SetupDatadogLogger(l)
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

	Err := Error // Alias to avoid go-vet false positive

	Trace("%s", "bar")
	Debug("%s", "bar")
	Info("%s", "bar")
	Warn("%s", "bar")
	Err("%s", "bar")
	Critical("%s", "bar")
	w.Flush()

	// Trace will not be logged
	assert.Equal(t, strings.Count(b.String(), "bar"), 5)
}

func TestCredentialScrubbingLogging(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)

	SetupDatadogLogger(l)
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

	SetupDatadogLogger(l)
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
