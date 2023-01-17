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
)

func BenchmarkLogVanilla(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")

	for n := 0; n < b.N; n++ {
		l.Infof("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogVanillaLevels(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.InfoLvl, "[%LEVEL] %FuncShort: %Msg")

	for n := 0; n < b.N; n++ {
		l.Debugf("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogScrubbing(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	for n := 0; n < b.N; n++ {
		Infof("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogScrubbingLevels(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	for n := 0; n < b.N; n++ {
		Debugf("this is a credential encoding uri: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogScrubbingMulti(b *testing.B) {
	var buffA, buffB bytes.Buffer
	wA := bufio.NewWriter(&buffA)
	wB := bufio.NewWriter(&buffB)

	lA, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(wA, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	lB, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(wB, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")

	SetupLogger(lA, "info")
	_ = RegisterAdditionalLogger("extra", lB)

	Info("this is an API KEY: ", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	Infof("this is a credential encoding urI: %s", "http://user:password@host:port")

	for n := 0; n < b.N; n++ {
		Infof("this is a credential encoding urI: %s", "http://user:password@host:port")
	}
}

func BenchmarkLogWithContext(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupLogger(l, "info")

	for n := 0; n < b.N; n++ {
		Infoc("this is a credential encoding uri: %s", "http://user:password@host:port", "extra", "context")
	}
}
