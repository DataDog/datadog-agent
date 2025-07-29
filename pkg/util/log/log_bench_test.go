// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"testing"
)

func BenchmarkLogVanilla(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, DebugLvl, "[%LEVEL] %FuncShort: %Msg")

	for n := 0; n < b.N; n++ {
		l.Infof("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogVanillaLevels(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, InfoLvl, "[%LEVEL] %FuncShort: %Msg")

	for n := 0; n < b.N; n++ {
		l.Debugf("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogScrubbing(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	for n := 0; n < b.N; n++ {
		Infof("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogScrubbingLevels(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	for n := 0; n < b.N; n++ {
		Debugf("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogWithContext(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := LoggerFromWriterWithMinLevelAndFormat(w, DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	for n := 0; n < b.N; n++ {
		Infoc("this is a credential encoding uri: %s", "http://user:password@host:port", "extra", "context")
	}
}
