// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSingleHandler(t *testing.T) {
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 20)
	handler := NewSingleHandler(*truncator)
	handler.Handle(richLine("Message longer than max length of truncator", true, true))
	output := <-outputChan
	assert.Equal(t, "...TRUNCATED...Message longer than ...TRUNCATED...", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, len("Message longer than "), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...max length of trunca...TRUNCATED...", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, len("max length of trunca"), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...tor...TRUNCATED...", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, len("tor"), output.RawDataLen)
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
}

func TestLineTruncatorWithDefaultLimits(t *testing.T) {
	outputChan := make(chan *Output, 4)
	truncator := NewLineTruncator(outputChan, defaultMaxSendLength)
	var inputBuf bytes.Buffer
	expected := make([]string, 4)
	for i := 0; i < 4; i++ {
		h := strconv.Itoa(i)
		inputBuf.WriteString(h)
		inputBuf.WriteString(strings.Repeat("a", defaultMaxSendLength-len(h)))
		expected[i] = h + strings.Repeat("a", defaultMaxSendLength-len(h))
	}
	expected[0] = expected[0] + string(truncatedString)
	expected[1] = string(truncatedString) + expected[1] + string(truncatedString)
	expected[2] = string(truncatedString) + expected[2] + string(truncatedString)
	expected[3] = string(truncatedString) + expected[3]
	input := richLine(inputBuf.String(), false, false)
	assert.Equal(t, defaultMaxDecodeLength, input.Size)
	truncator.truncate(input)
	for i := 0; i < 4; i++ {
		output := <-outputChan
		assert.Equal(t, expected[i], string(output.Content))
		assert.Equal(t, input.Timestamp, output.Timestamp)
		assert.Equal(t, input.Status, output.Status)
		assert.Equal(t, defaultMaxSendLength, output.RawDataLen)
	}
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
}

func TestLineTruncatorLargeLine(t *testing.T) {
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 20)
	truncator.truncate(richLine("Message longer than max length of truncator", false, false))

	output := <-outputChan
	assert.Equal(t, "Message longer than ...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...max length of trunca...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...tor", string(output.Content))

	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, len("tor"), output.RawDataLen)
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
}

func TestLineTruncatorNoMiddleLine(t *testing.T) {
	outputChan := make(chan *Output, 2)
	truncator := NewLineTruncator(outputChan, 40)
	truncator.truncate(richLine("Message longer than max length of truncator", false, false))

	output := <-outputChan
	assert.Equal(t, "Message longer than max length of trunca...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...tor", string(output.Content))
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
}

func TestTruncateLargeLineThenSmallLine(t *testing.T) {
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 40)
	truncator.truncate(richLine("Message longer than max length of truncator that's for sure", false, true))
	truncator.truncate(richLine("Short message", false, false))
	output := <-outputChan
	assert.Equal(t, "Message longer than max length of trunca...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...tor that's for sure...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "Short message", string(output.Content))
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
}

func TestTruncateSmallLineThenLargeLine(t *testing.T) {
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 40)
	truncator.truncate(richLine("Short message", false, false))
	truncator.truncate(richLine("Message longer than max length of truncator that's for sure", false, true))
	output := <-outputChan
	assert.Equal(t, "Short message", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "Message longer than max length of trunca...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...tor that's for sure...TRUNCATED...", string(output.Content))

	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
}

func TestLineTruncatorSmallLine(t *testing.T) {
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 200)

	truncator.truncate(richLine("small message with leading and tailing", true, true))
	output := <-outputChan
	assert.Equal(t, "...TRUNCATED...small message with leading and tailing...TRUNCATED...", string(output.Content))

	truncator.truncate(richLine("small message with leading", true, false))
	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...small message with leading", string(output.Content))

	truncator.truncate(richLine("small message with tailing", false, true))
	output = <-outputChan
	assert.Equal(t, "small message with tailing...TRUNCATED...", string(output.Content))

	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, len("small message with tailing"), output.RawDataLen)
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
}

func assertNoMoreOutput(t *testing.T, outputChan chan *Output, waitTimeout time.Duration) {
	// wait for 3 times of waitTimeout to see if there is any more output.
	for i := 0; i < 3; i++ {
		select {
		case <-outputChan:
			assert.Fail(t, "There should be no more output from this channel")
			return
		default:
			time.Sleep(waitTimeout)
		}
	}
}

func richLine(content string, needLeading bool, needTailing bool) *RichLine {
	return NewRichLineBuilder().
		ContentString(content).
		Timestamp("2019-06-06T16:35:55.930852911Z").
		Status(message.StatusInfo).
		IsLeading(needLeading).
		IsTailing(needTailing).
		Build()
}
