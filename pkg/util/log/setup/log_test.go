// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func benchmarkLogFormat(logFormat string, b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, log.TemplateFormatter(logFormat))

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		buff.Reset()
		l.Infof("Hello I am a log")
	}
}

func BenchmarkLogFormatFilename(b *testing.B) {
	benchmarkLogFormat("{{.DateTime}} | {{.LEVEL}} | ({{.file}}:{{.line}} in {{.FuncShort}}) | {{.msg}}", b)
}

func BenchmarkLogFormatShortFilePath(b *testing.B) {
	benchmarkLogFormat("{{.DateTime}} | {{.LEVEL}} | ({{.ShortFile}}:{{.line}} in {{.FuncShort}}) | {{.msg}}", b)
}

func benchmarkLogFormatWithContext(logFormat string, b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, log.TemplateFormatter(logFormat))
	context := []interface{}{"extra", "context", "foo", "bar"}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		buff.Reset()
		l.Logf(log.InfoLvl, 0, context, "Hello I am a log")
	}
}

func BenchmarkLogFormatWithoutContextFormatting(b *testing.B) {
	benchmarkLogFormatWithContext("{{.DateTime}} | {{.LEVEL}} | ({{.ShortFile}}:{{.line}} in {{.FuncShort}}) | {{.msg}}", b)
}

func BenchmarkLogFormatWithContextFormatting(b *testing.B) {
	benchmarkLogFormatWithContext("{{.DateTime}} | {{.LEVEL}} | ({{.ShortFile}}:{{.line}} in {{.FuncShort}}) | {{.msg}} %ExtraJSONContext", b)
}
