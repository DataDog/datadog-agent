// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
	"time"
)

func TestMultiLineHandlerMatchMultipleLine(t *testing.T) {
	outputChan := make(chan *Output, 10)
	truncator := NewLineTruncator(outputChan, 60)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	handler.Handle(richLine("last message", false, false))
	handler.Handle(richLine("2. first message", false, false))
	handler.Handle(richLine("continue message 1", false, false))
	handler.Handle(richLine("continue message 2", false, false))
	handler.Handle(richLine("3. third message very very long, longer than max length of truncator", false, false))
	handler.SendResult()

	output := <-outputChan
	assert.Equal(t, "last message", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2. first message\\ncontinue message 1\\ncontinue message 2", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "3. third message very very long, longer than max length of t...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...runcator", string(output.Content))

	assertNoMoreOutput(t, outputChan, flushTimeout)
	handler.Cleanup()
}

func TestMultiLineHandlerNotTrim(t *testing.T) {
	outputChan := make(chan *Output, 1)
	truncator := NewLineTruncator(outputChan, 200)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	handler.Handle(richLine("  ", false, false))
	handler.Handle(richLine("  last message   ", false, false))
	handler.SendResult()

	output := <-outputChan
	assert.Equal(t, "  \\n  last message   ", string(output.Content))
	assertNoMoreOutput(t, outputChan, flushTimeout)
	handler.Cleanup()
}

func TestMultiLIneHandlerHandleNilContent(t *testing.T) {
	outputChan := make(chan *Output, 1)
	truncator := NewLineTruncator(outputChan, 60)
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	handler.Handle(&RichLine{
		Line: parser.Line{
			Prefix: parser.Prefix{
				Timestamp: "2019-06-06T16:35:55.930852911Z",
				Status:    message.StatusInfo,
			},
		},
	})
	handler.SendResult()
	assertNoMoreOutput(t, outputChan, 1*time.Millisecond)
	handler.Cleanup()
	output := <-outputChan
	assert.Nil(t, output)
	assertOutputChanClosed(t, outputChan)
}
