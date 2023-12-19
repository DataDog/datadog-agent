// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package decoder

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// All valid whitespace characters
const whitespace = "\t\n\v\f\r\u0085\u00a0 "
const contentLenLimit = 100

func getDummyMessage(content string) *message.Message {
	return NewMessage([]byte(content), "info", len(content), "2018-06-14T18:27:03.246999277Z")
}

func getDummyMessageWithLF(content string) *message.Message {
	return NewMessage([]byte(content), "info", len(content)+1, "2018-06-14T18:27:03.246999277Z")
}

func lineHandlerChans() (func(*message.Message), chan *message.Message) {
	ch := make(chan *message.Message, 20)
	return func(m *message.Message) { ch <- m }, ch
}

func assertNothingInChannel(t *testing.T, ch chan *message.Message) {
	select {
	case <-ch:
		assert.Fail(t, "unexpected message")
	default:
	}
}

func TestSingleLineHandler(t *testing.T) {
	outputFn, outputChan := lineHandlerChans()
	h := NewSingleLineHandler(outputFn, 100)

	var output *message.Message
	var line string

	// valid line should be sent
	line = "hello world"
	h.process(getDummyMessageWithLF(line))
	output = <-outputChan
	assert.Equal(t, line, string(output.GetContent()))
	assert.Equal(t, len(line)+1, output.RawDataLen)

	// too long line should be truncated
	line = strings.Repeat("a", contentLenLimit+10)
	h.process(getDummyMessage(line))
	output = <-outputChan
	assert.Equal(t, len(line)+len(truncatedFlag), len(output.GetContent()))
	assert.Equal(t, len(line), output.RawDataLen)

	line = strings.Repeat("a", contentLenLimit+10)
	h.process(getDummyMessage(line))
	output = <-outputChan
	assert.Equal(t, len(truncatedFlag)+len(line)+len(truncatedFlag), len(output.GetContent()))
	assert.Equal(t, len(line), output.RawDataLen)

	line = strings.Repeat("a", 10)
	h.process(getDummyMessageWithLF(line))
	output = <-outputChan
	assert.Equal(t, string(truncatedFlag)+line, string(output.GetContent()))
	assert.Equal(t, len(line)+1, output.RawDataLen)
}

func TestTrimSingleLine(t *testing.T) {
	outputFn, outputChan := lineHandlerChans()
	h := NewSingleLineHandler(outputFn, 100)

	var output *message.Message

	// All leading and trailing whitespace characters should be trimmed
	line := whitespace + "foo" + whitespace + "bar" + whitespace
	h.process(getDummyMessageWithLF(line))
	output = <-outputChan
	assert.Equal(t, "foo"+whitespace+"bar", string(output.GetContent()))
	assert.Equal(t, len(line)+1, output.RawDataLen)
}

func TestMultiLineHandler(t *testing.T) {
	re := regexp.MustCompile(`[0-9]+\.`)
	outputFn, outputChan := lineHandlerChans()
	h := NewMultiLineHandler(outputFn, re, 250*time.Millisecond, 20, false)

	var output *message.Message

	// two lines long message should be sent
	h.process(getDummyMessageWithLF("1.first"))
	h.process(getDummyMessageWithLF("second"))

	// one line long message should be sent
	h.process(getDummyMessageWithLF("2. first line"))

	output = <-outputChan
	var expectedContent = "1.first\\nsecond"
	assert.Equal(t, expectedContent, string(output.GetContent()))
	assert.Equal(t, len(expectedContent), output.RawDataLen)

	assertNothingInChannel(t, outputChan)
	h.flush()

	output = <-outputChan
	assert.Equal(t, "2. first line", string(output.GetContent()))
	assert.Equal(t, len("2. first line")+1, output.RawDataLen)

	assertNothingInChannel(t, outputChan)
	h.flush()

	// too long line should be truncated
	h.process(getDummyMessage("3. stringssssssize20"))
	h.process(getDummyMessageWithLF("con"))

	output = <-outputChan
	assert.Equal(t, "3. stringssssssize20...TRUNCATED...", string(output.GetContent()))
	assert.Equal(t, len("3. stringssssssize20"), output.RawDataLen)

	assertNothingInChannel(t, outputChan)
	h.flush()

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...con", string(output.GetContent()))
	assert.Equal(t, 4, output.RawDataLen)

	// second line + TRUNCATED too long
	h.process(getDummyMessage("4. stringssssssize20"))
	h.process(getDummyMessageWithLF("continue"))

	output = <-outputChan
	assert.Equal(t, "4. stringssssssize20...TRUNCATED...", string(output.GetContent()))
	assert.Equal(t, len("4. stringssssssize20"), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...continue...TRUNCATED...", string(output.GetContent()))
	assert.Equal(t, 9, output.RawDataLen)

	// continuous too long lines
	h.process(getDummyMessage("5. stringssssssize20"))
	longLineTracingSpaces := "continu             "
	h.process(getDummyMessage(longLineTracingSpaces))
	h.process(getDummyMessageWithLF("end"))
	shortLineTracingSpaces := "6. next line      "
	h.process(getDummyMessageWithLF(shortLineTracingSpaces))

	output = <-outputChan
	assert.Equal(t, "5. stringssssssize20...TRUNCATED...", string(output.GetContent()))
	assert.Equal(t, len("5. stringssssssize20"), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...continu             ...TRUNCATED...", string(output.GetContent()))
	assert.Equal(t, len(longLineTracingSpaces), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...end", string(output.GetContent()))
	assert.Equal(t, len("end\n"), output.RawDataLen)

	assertNothingInChannel(t, outputChan)
	h.flush()

	output = <-outputChan
	assert.Equal(t, "6. next line", string(output.GetContent()))
	assert.Equal(t, len(shortLineTracingSpaces)+1, output.RawDataLen)
}

func TestTrimMultiLine(t *testing.T) {
	re := regexp.MustCompile(`[0-9]+\.`)
	outputFn, outputChan := lineHandlerChans()
	h := NewMultiLineHandler(outputFn, re, 250*time.Millisecond, 100, false)

	var output *message.Message

	// All leading and trailing whitespace characters should be trimmed
	h.process(getDummyMessageWithLF(whitespace + "foo" + whitespace + "bar" + whitespace))

	assertNothingInChannel(t, outputChan)
	h.flush()

	output = <-outputChan
	assert.Equal(t, "foo"+whitespace+"bar", string(output.GetContent()))
	assert.Equal(t, len(whitespace+"foo"+whitespace+"bar"+whitespace)+1, output.RawDataLen)

	// With line break
	h.process(getDummyMessageWithLF(whitespace + "foo" + whitespace))
	h.process(getDummyMessageWithLF("bar" + whitespace))

	assertNothingInChannel(t, outputChan)
	h.flush()

	output = <-outputChan
	assert.Equal(t, "foo"+whitespace+"\\n"+"bar", string(output.GetContent()))
	assert.Equal(t, len(whitespace+"foo"+whitespace)+1+len("bar"+whitespace)+1, output.RawDataLen)
}

func TestMultiLineHandlerDropsEmptyMessages(t *testing.T) {
	re := regexp.MustCompile(`[0-9]+\.`)
	outputFn, outputChan := lineHandlerChans()
	h := NewMultiLineHandler(outputFn, re, 250*time.Millisecond, 100, false)

	h.process(getDummyMessage(""))

	h.process(getDummyMessage("1.third line"))
	h.process(getDummyMessage("fourth line"))

	var output *message.Message

	assertNothingInChannel(t, outputChan)
	h.flush()

	output = <-outputChan
	assert.Equal(t, "1.third line\\nfourth line", string(output.GetContent()))
}

func TestSingleLineHandlerSendsRawInvalidMessages(t *testing.T) {
	outputFn, outputChan := lineHandlerChans()
	h := NewSingleLineHandler(outputFn, 100)

	h.process(getDummyMessage("one message"))

	output := <-outputChan
	assert.Equal(t, "one message", string(output.GetContent()))
}

func TestMultiLineHandlerSendsRawInvalidMessages(t *testing.T) {
	re := regexp.MustCompile(`[0-9]+\.`)
	outputFn, outputChan := lineHandlerChans()
	h := NewMultiLineHandler(outputFn, re, 250*time.Millisecond, 100, false)

	h.process(getDummyMessage("1.third line"))
	h.process(getDummyMessage("fourth line"))

	var output *message.Message

	assertNothingInChannel(t, outputChan)
	h.flush()

	output = <-outputChan
	assert.Equal(t, "1.third line\\nfourth line", string(output.GetContent()))
}

func TestAutoMultiLineHandlerStaysSingleLineMode(t *testing.T) {

	outputFn, outputChan := lineHandlerChans()
	source := sources.NewReplaceableSource(sources.NewLogSource("config", &config.LogsConfig{}))
	detectedPattern := &DetectedPattern{}
	h := NewAutoMultilineHandler(outputFn, 100, 5, 1.0, 250*time.Millisecond, 250*time.Millisecond, source, []*regexp.Regexp{}, detectedPattern, status.NewInfoRegistry())

	for i := 0; i < 6; i++ {
		h.process(getDummyMessageWithLF("blah"))
		<-outputChan
	}
	assert.NotNil(t, h.singleLineHandler)
	assert.Nil(t, h.multiLineHandler)
	assert.Nil(t, detectedPattern.Get())
}

func TestAutoMultiLineHandlerSwitchesToMultiLineMode(t *testing.T) {
	outputFn, outputChan := lineHandlerChans()
	source := sources.NewReplaceableSource(sources.NewLogSource("config", &config.LogsConfig{}))
	detectedPattern := &DetectedPattern{}
	h := NewAutoMultilineHandler(outputFn, 100, 5, 1.0, 250*time.Millisecond, 250*time.Millisecond, source, []*regexp.Regexp{}, detectedPattern, status.NewInfoRegistry())

	for i := 0; i < 6; i++ {
		h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message"))
		h.flush() // multiline handler needs to be flushed on 6th iteration
		<-outputChan
	}
	assert.Nil(t, h.singleLineHandler)
	assert.NotNil(t, h.multiLineHandler)
	assert.NotNil(t, detectedPattern.Get())
}

func TestAutoMultiLineHandlerHandelsMessage(t *testing.T) {
	outputFn, outputChan := lineHandlerChans()
	source := sources.NewReplaceableSource(sources.NewLogSource("config", &config.LogsConfig{}))
	h := NewAutoMultilineHandler(outputFn, 500, 1, 1.0, 250*time.Millisecond, 250*time.Millisecond, source, []*regexp.Regexp{}, &DetectedPattern{}, status.NewInfoRegistry())

	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 1"))
	<-outputChan
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 2"))
	h.process(getDummyMessageWithLF("java.lang.Exception: boom"))
	h.process(getDummyMessageWithLF("at Main.funcd(Main.java:62)"))
	h.process(getDummyMessageWithLF("at Main.funcc(Main.java:60)"))
	h.process(getDummyMessageWithLF("at Main.funcb(Main.java:58)"))
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM another test message"))
	output := <-outputChan

	assert.Equal(t, "Jul 12, 2021 12:55:15 PM test message 2\\njava.lang.Exception: boom\\nat Main.funcd(Main.java:62)\\nat Main.funcc(Main.java:60)\\nat Main.funcb(Main.java:58)", string(output.GetContent()))
}

func TestAutoMultiLineHandlerHandelsMessageConflictingPatterns(t *testing.T) {
	outputFn, outputChan := lineHandlerChans()
	source := sources.NewReplaceableSource(sources.NewLogSource("config", &config.LogsConfig{}))
	h := NewAutoMultilineHandler(outputFn, 500, 4, 0.75, 250*time.Millisecond, 250*time.Millisecond, source, []*regexp.Regexp{}, &DetectedPattern{}, status.NewInfoRegistry())

	// we will match both patterns, but one will win with a threshold of 0.75
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 1"))
	h.process(getDummyMessageWithLF("Jul, 1-sep-12 10:20:30 pm test message 2"))
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 3"))
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 4"))

	for i := 0; i < 4; i++ {
		<-outputChan
	}
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 2"))
	h.process(getDummyMessageWithLF("java.lang.Exception: boom"))
	h.process(getDummyMessageWithLF("at Main.funcd(Main.java:62)"))
	h.process(getDummyMessageWithLF("at Main.funcc(Main.java:60)"))
	h.process(getDummyMessageWithLF("at Main.funcb(Main.java:58)"))
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM another test message"))
	output := <-outputChan

	assert.Equal(t, "Jul 12, 2021 12:55:15 PM test message 2\\njava.lang.Exception: boom\\nat Main.funcd(Main.java:62)\\nat Main.funcc(Main.java:60)\\nat Main.funcb(Main.java:58)", string(output.GetContent()))
}

func TestAutoMultiLineHandlerHandelsMessageConflictingPatternsNoWinner(t *testing.T) {
	outputFn, outputChan := lineHandlerChans()
	source := sources.NewReplaceableSource(sources.NewLogSource("config", &config.LogsConfig{}))
	h := NewAutoMultilineHandler(outputFn, 500, 4, 0.75, 250*time.Millisecond, 250*time.Millisecond, source, []*regexp.Regexp{}, &DetectedPattern{}, status.NewInfoRegistry())

	// we will match both patterns, but neither will win because it doesn't meet the threshold
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 1"))
	h.process(getDummyMessageWithLF("Jul, 1-sep-12 10:20:30 pm test message 2"))
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 3"))
	h.process(getDummyMessageWithLF("Jul, 1-sep-12 10:20:30 pm test message 4"))

	for i := 0; i < 4; i++ {
		<-outputChan
	}
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message 2"))
	output := <-outputChan

	assert.NotNil(t, h.singleLineHandler)
	assert.Nil(t, h.multiLineHandler)

	assert.Equal(t, "Jul 12, 2021 12:55:15 PM test message 2", string(output.GetContent()))
}

func TestAutoMultiLineHandlerSwitchesToMultiLineModeWithDelay(t *testing.T) {
	outputFn, _ := lineHandlerChans()
	source := sources.NewReplaceableSource(sources.NewLogSource("config", &config.LogsConfig{}))
	detectedPattern := &DetectedPattern{}

	h := NewAutoMultilineHandler(outputFn, 100, 5, 1.0, 250*time.Millisecond, 250*time.Millisecond, source, []*regexp.Regexp{}, detectedPattern, status.NewInfoRegistry())
	clock := clock.NewMock()
	h.clk = clock

	// Advance the clock past the (10ms) detection timeout. (timer should not have started yet)
	clock.Add(time.Second)

	// Process a log that will match - the timer should start now
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message"))

	// Have not finished detection yet
	assert.NotNil(t, h.singleLineHandler)
	assert.Nil(t, h.multiLineHandler)

	// Advance the clock past the (10ms) detection timeout.
	clock.Add(time.Second)

	// Process a log that will match. The timer has already timed out
	h.process(getDummyMessageWithLF("Jul 12, 2021 12:55:15 PM test message"))

	// Have not finished detection yet
	assert.Nil(t, h.singleLineHandler)
	assert.NotNil(t, h.multiLineHandler)
}
