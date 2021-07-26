// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package decoder

import (
	"fmt"
	"testing"
)

func benchmarkSingleLineHandler(b *testing.B, logs int) {
	tags := make([]*Message, logs)
	for i := 0; i < logs; i++ {
		tags[i] = getDummyMessageWithLF(fmt.Sprintf("This is a log test line to benchmark the logs agent blah blah blah %d", i))
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
		for _, v := range tags {
			h.inputChan <- v
		}
	}
}

func benchmarkAutoMultiLineHandler(b *testing.B, logs int, line string) {
	tags := make([]*Message, logs)
	for i := 0; i < logs; i++ {
		tags[i] = getDummyMessageWithLF(fmt.Sprintf("%s %d", line, i))
	}

	outputChan := make(chan *Message, 10)
	h := NewAutoMultilineHandler(outputChan, defaultContentLenLimit, 100, 0.9, 1000)
	h.Start()

	go func() {
		for {
			<-outputChan
		}
	}()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, v := range tags {
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
