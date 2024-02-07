// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package decoder

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	status "github.com/DataDog/datadog-agent/pkg/logs/logstatus"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func benchmarkSingleLineHandler(b *testing.B, logs int) {
	messages := make([]*message.Message, logs)
	for i := 0; i < logs; i++ {
		messages[i] = getDummyMessageWithLF(fmt.Sprintf("This is a log test line to benchmark the logs agent %d", i))
	}

	h := NewSingleLineHandler(func(*message.Message) {}, coreConfig.DefaultMaxMessageSizeBytes)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, v := range messages {
			h.process(v)
		}
	}
}

func benchmarkAutoMultiLineHandler(b *testing.B, logs int, line string) {
	messages := make([]*message.Message, logs)
	for i := 0; i < logs; i++ {
		messages[i] = getDummyMessageWithLF(fmt.Sprintf("%s %d", line, i))
	}

	source := sources.NewReplaceableSource(sources.NewLogSource("config", &config.LogsConfig{}))
	h := NewAutoMultilineHandler(func(*message.Message) {}, coreConfig.DefaultMaxMessageSizeBytes, 1000, 0.9, 30*time.Second, 1000*time.Millisecond, source, []*regexp.Regexp{}, &DetectedPattern{}, status.NewInfoRegistry())

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, v := range messages {
			h.process(v)
		}
	}
}

func benchmarkMultiLineHandler(b *testing.B, logs int, line string) {
	messages := make([]*message.Message, logs)
	for i := 0; i < logs; i++ {
		messages[i] = getDummyMessageWithLF(fmt.Sprintf("%s %d", line, i))
	}

	h := NewMultiLineHandler(func(*message.Message) {}, regexp.MustCompile(`^[A-Za-z_]+ \d+, \d+ \d+:\d+:\d+ (AM|PM)`), 1000*time.Millisecond, 100, false)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, v := range messages {
			h.process(v)
		}
	}
}

func BenchmarkSingleLineHandler1(b *testing.B)      { benchmarkSingleLineHandler(b, 1) }
func BenchmarkSingleLineHandler10(b *testing.B)     { benchmarkSingleLineHandler(b, 10) }
func BenchmarkSingleLineHandler100(b *testing.B)    { benchmarkSingleLineHandler(b, 100) }
func BenchmarkSingleLineHandler1000(b *testing.B)   { benchmarkSingleLineHandler(b, 1000) }
func BenchmarkSingleLineHandler10000(b *testing.B)  { benchmarkSingleLineHandler(b, 10000) }
func BenchmarkSingleLineHandler100000(b *testing.B) { benchmarkSingleLineHandler(b, 100000) }

var nonMatchLine = "This is a log test line to benchmark the logs agent blah blah blah"
var matchLine = "Jul 12, 2021 12:55:15 PM This is a log test line to benchmark the logs agent blah blah blah"

func BenchmarkAutoMultiLineHandler1(b *testing.B) { benchmarkAutoMultiLineHandler(b, 1, nonMatchLine) }
func BenchmarkAutoMultiLineHandler10(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 10, nonMatchLine)
}
func BenchmarkAutoMultiLineHandler100(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 100, nonMatchLine)
}
func BenchmarkAutoMultiLineHandler1000(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 1000, nonMatchLine)
}
func BenchmarkAutoMultiLineHandler10000(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 10000, nonMatchLine)
}
func BenchmarkAutoMultiLineHandler100000(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 100000, nonMatchLine)
}

func BenchmarkAutoMultiLineHandlerMatch1(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 1, matchLine)
}
func BenchmarkAutoMultiLineHandlerMatch10(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 10, matchLine)
}
func BenchmarkAutoMultiLineHandlerMatch100(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 100, matchLine)
}
func BenchmarkAutoMultiLineHandlerMatch1000(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 1000, matchLine)
}
func BenchmarkAutoMultiLineHandlerMatch10000(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 10000, matchLine)
}
func BenchmarkAutoMultiLineHandlerMatch100000(b *testing.B) {
	benchmarkAutoMultiLineHandler(b, 100000, matchLine)
}
func BenchmarkMultiLineHandler1(b *testing.B) {
	benchmarkMultiLineHandler(b, 1, matchLine)
}
func BenchmarkMultiLineHandler10(b *testing.B) {
	benchmarkMultiLineHandler(b, 10, matchLine)
}
func BenchmarkMultiLineHandler100(b *testing.B) {
	benchmarkMultiLineHandler(b, 100, matchLine)
}
func BenchmarkMultiLineHandler1000(b *testing.B) {
	benchmarkMultiLineHandler(b, 1000, matchLine)
}
func BenchmarkMultiLineHandler10000(b *testing.B) {
	benchmarkMultiLineHandler(b, 10000, matchLine)
}
func BenchmarkMultiLineHandler100000(b *testing.B) {
	benchmarkMultiLineHandler(b, 100000, matchLine)
}
