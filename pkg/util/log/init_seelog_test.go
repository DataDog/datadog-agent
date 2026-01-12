// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
)

func benchmarkLogFormat(logFormat string, b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, logFormat)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		buff.Reset()
		l.Infof("Hello I am a log")
	}
}

func BenchmarkLogFormatFilename(b *testing.B) {
	benchmarkLogFormat("%Date(%s) | %LEVEL | (%File:%Line in %FuncShort) | %Msg", b)
}

func BenchmarkLogFormatShortFilePath(b *testing.B) {
	benchmarkLogFormat("%Date(%s) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg", b)
}

func TestExtractContextString(t *testing.T) {
	assert.Equal(t, `,"foo":"bar"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "bar"})))
	assert.Equal(t, `foo:bar | `, formatters.ExtraTextContext(toAttrHolder([]interface{}{"foo", "bar"})))
	assert.Equal(t, `,"foo":"bar","bar":"buzz"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "bar", "bar", "buzz"})))
	assert.Equal(t, `foo:bar,bar:buzz | `, formatters.ExtraTextContext(toAttrHolder([]interface{}{"foo", "bar", "bar", "buzz"})))
	assert.Equal(t, `,"foo":"b\"a\"r"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "b\"a\"r"})))
	assert.Equal(t, `,"foo":"3"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", 3})))
	assert.Equal(t, `,"foo":"4.131313131"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", float64(4.131313131)})))
	assert.Equal(t, "", formatters.ExtraJSONContext(toAttrHolder(nil)))
	assert.Equal(t, "", formatters.ExtraJSONContext(toAttrHolder([]interface{}{2, 3})))
	assert.Equal(t, `,"foo":"bar","bar":"buzz"`, formatters.ExtraJSONContext(toAttrHolder([]interface{}{"foo", "bar", 2, 3, "bar", "buzz"})))
}

func benchmarkLogFormatWithContext(logFormat string, b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, logFormat)
	context := []interface{}{"extra", "context", "foo", "bar"}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		buff.Reset()
		l.SetContext(context)
		l.Infof("Hello I am a log")
		l.SetContext(nil)
	}
}

func BenchmarkLogFormatWithoutContextFormatting(b *testing.B) {
	benchmarkLogFormatWithContext("%Date(%s) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg", b)
}

func BenchmarkLogFormatWithContextFormatting(b *testing.B) {
	benchmarkLogFormatWithContext("%Date(%s) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg %ExtraJSONContext", b)
}
