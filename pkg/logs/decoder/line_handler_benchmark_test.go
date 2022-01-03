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

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func benchmarkSingleLineHandler(b *testing.B, logs int) {
	messages := make([]*Message, logs)
	for i := 0; i < logs; i++ {
		messages[i] = getDummyMessageWithLF(fmt.Sprintf("This is a log test line to benchmark the logs agent %d", i))
	}

	outputChan := make(chan *Message, 10)
	h := NewSingleLineHandler(outputChan, defaultContentLenLimit)
	h.Start()

	go func() {
		for {
			<-outputChan
		}
	}()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, v := range messages {
			h.inputChan <- v
		}
	}
}

func benchmarkAutoMultiLineHandler(b *testing.B, logs int, line string) {
	messages := make([]*Message, logs)
	for i := 0; i < logs; i++ {
		messages[i] = getDummyMessageWithLF(fmt.Sprintf("%s %d", line, i))
	}

	outputChan := make(chan *Message, 10)
	source := config.NewLogSource("config", &config.LogsConfig{})
	h := NewAutoMultilineHandler(outputChan, defaultContentLenLimit, 1000, 0.9, 30*time.Second, 1000*time.Millisecond, source, []*regexp.Regexp{}, &DetectedPattern{})
	h.Start()

	go func() {
		for {
			<-outputChan
		}
	}()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, v := range messages {
			h.inputChan <- v
		}
	}
}

func benchmarkMultiLineHandler(b *testing.B, logs int, line string) {
	messages := make([]*Message, logs)
	for i := 0; i < logs; i++ {
		messages[i] = getDummyMessageWithLF(fmt.Sprintf("%s %d", line, i))
	}

	outputChan := make(chan *Message, 10)
	h := NewMultiLineHandler(outputChan, regexp.MustCompile(`^[A-Za-z_]+ \d+, \d+ \d+:\d+:\d+ (AM|PM)`), 1000*time.Millisecond, 100)
	h.Start()

	go func() {
		for {
			<-outputChan
		}
	}()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, v := range messages {
			h.inputChan <- v
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
