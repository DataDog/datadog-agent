// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
	"time"
)

func TestLineGeneratorWithNoPrefix(t *testing.T) {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 60)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := NewLineHandlerScheduler(make(chan *RichLine), flushTimeout, handler)
	generator := NewLineGenerator(600, inputChan, &NewLineMatcher{}, &parser.NoopConvertor{}, *scheduler)

	generator.Start()
	inputChan <- NewInput([]byte("1.first ch"))
	inputChan <- NewInput([]byte("unk\n2.sec"))
	inputChan <- NewInput([]byte("ond chunk\n"))
	inputChan <- NewInput([]byte("3.multi li"))
	inputChan <- NewInput([]byte("ne\nsecond"))
	inputChan <- NewInput([]byte(" of multi "))
	inputChan <- NewInput([]byte("line\nend "))
	inputChan <- NewInput([]byte("of multili"))
	inputChan <- NewInput([]byte("ne\n"))

	outputs := make([]*Output, 3)
	for i := 0; i < len(outputs); i++ {
		outputs[i] = <-outputChan
	}
	expectedContents := []string{
		"1.first chunk",
		"2.second chunk",
		"3.multi line\\nsecond of multi line\\nend of multiline"}
	for i := 0; i < len(outputs); i++ {
		output := outputs[i]
		assert.Equal(t, expectedContents[i], string(output.Content))
		assert.Equal(t, len(expectedContents[i]), output.RawDataLen)
		assert.Equal(t, "", output.Timestamp)
		assert.Equal(t, "", output.Status)
	}
	assertNoMoreOutput(t, outputChan, flushTimeout)
}

func TestLineGeneratorMatchEndlineAlsoMaxLen(t *testing.T) {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 20)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := NewLineHandlerScheduler(make(chan *RichLine), flushTimeout, handler)
	generator := NewLineGenerator(51, inputChan, &NewLineMatcher{}, &parser.NoopConvertor{}, *scheduler)

	generator.Start()
	inputChan <- NewInput([]byte("1.a very ver"))
	inputChan <- NewInput([]byte("y long log w"))
	inputChan <- NewInput([]byte("hich exceeds"))
	inputChan <- NewInput([]byte(" the hard li"))
	inputChan <- NewInput([]byte("mit\n2nd l"))
	inputChan <- NewInput([]byte("ine\n other "))
	inputChan <- NewInput([]byte("line\n"))

	outputs := make([]*Output, 4)
	for i := 0; i < len(outputs); i++ {
		outputs[i] = <-outputChan
	}
	expectedContents := []string{
		"1.a very very long l...TRUNCATED...",
		"...TRUNCATED...og which exceeds the...TRUNCATED...",
		"...TRUNCATED... hard limit\\n2nd lin...TRUNCATED...",
		"...TRUNCATED...e\\n other line"}
	expectedRawDataLens := []int{
		len(expectedContents[0]) - 15, len(expectedContents[1]) - 30, len(expectedContents[2]) - 30, len(expectedContents[3]) - 15}
	for i := 0; i < len(outputs); i++ {
		output := outputs[i]
		assert.Equal(t, expectedContents[i], string(output.Content))
		assert.Equal(t, expectedRawDataLens[i], output.RawDataLen)
		assert.Equal(t, "", output.Timestamp)
		assert.Equal(t, "", output.Status)
	}
	assertNoMoreOutput(t, outputChan, flushTimeout)
}

func TestLongLineIsAlsoMultiLine(t *testing.T) {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 25)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := NewLineHandlerScheduler(make(chan *RichLine), flushTimeout, handler)
	generator := NewLineGenerator(50, inputChan, &NewLineMatcher{}, &parser.NoopConvertor{}, *scheduler)
	generator.Start()
	inputChan <- NewInput([]byte("1.a very ver"))
	inputChan <- NewInput([]byte("y long log w"))
	inputChan <- NewInput([]byte("hich exceeds"))
	inputChan <- NewInput([]byte(" the hard li"))
	inputChan <- NewInput([]byte("mit\n2nd l"))
	inputChan <- NewInput([]byte("ine\n other "))
	inputChan <- NewInput([]byte("line\n"))

	outputs := make([]*Output, 3)
	for i := 0; i < len(outputs); i++ {
		outputs[i] = <-outputChan
	}
	expectedContents := []struct {
		string
		int
	}{
		{
			"1.a very very long log wh...TRUNCATED...",
			len("1.a very very long log wh")},
		{
			"...TRUNCATED...ich exceeds the hard limi...TRUNCATED...",
			len("ich exceeds the hard limi")},
		{
			"...TRUNCATED...t\\n2nd line\\n other line",
			len("t\\n2nd line\\n other line")}}
	for i := 0; i < len(outputs); i++ {
		output := outputs[i]
		assert.Equal(t, expectedContents[i].string, string(output.Content))
		assert.Equal(t, expectedContents[i].int, output.RawDataLen)
		assert.Equal(t, "", output.Timestamp)
		assert.Equal(t, "", output.Status)
	}
	assertNoMoreOutput(t, outputChan, flushTimeout)
}

func TestLongLineThenMultiLine(t *testing.T) {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 25)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := NewLineHandlerScheduler(make(chan *RichLine), flushTimeout, handler)
	generator := NewLineGenerator(50, inputChan, &NewLineMatcher{}, &parser.NoopConvertor{}, *scheduler)
	generator.Start()
	inputChan <- NewInput([]byte("1.a very ver"))
	inputChan <- NewInput([]byte("y long log w"))
	inputChan <- NewInput([]byte("hich exceeds"))
	inputChan <- NewInput([]byte(" the hard li"))
	inputChan <- NewInput([]byte("mit\n2.2nd l"))
	inputChan <- NewInput([]byte("ine\n other "))
	inputChan <- NewInput([]byte("line\n"))

	outputs := make([]*Output, 4)
	for i := 0; i < len(outputs); i++ {
		outputs[i] = <-outputChan
	}
	expectedContents := []struct {
		string
		int
	}{
		{
			"1.a very very long log wh...TRUNCATED...",
			len("1.a very very long log wh")},
		{
			"...TRUNCATED...ich exceeds the hard limi...TRUNCATED...",
			len("ich exceeds the hard limi")},
		{
			"...TRUNCATED...t",
			len("t")},
		{
			"2.2nd line\\n other line",
			len("2.2nd line\\n other line")}}
	for i := 0; i < len(outputs); i++ {
		output := outputs[i]
		assert.Equal(t, expectedContents[i].string, string(output.Content))
		assert.Equal(t, expectedContents[i].int, output.RawDataLen)
		assert.Equal(t, "", output.Timestamp)
		assert.Equal(t, "", output.Status)
	}
	assertNoMoreOutput(t, outputChan, flushTimeout)
}

func TestMulitLineThenLongLine(t *testing.T) {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output, 6)
	truncator := NewLineTruncator(outputChan, 25)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := NewLineHandlerScheduler(make(chan *RichLine), flushTimeout, handler)
	generator := NewLineGenerator(50, inputChan, &NewLineMatcher{}, &parser.NoopConvertor{}, *scheduler)
	generator.Start()
	inputChan <- NewInput([]byte("1.a very ver"))
	inputChan <- NewInput([]byte("y long\n log"))
	inputChan <- NewInput([]byte(" which exce\n"))
	inputChan <- NewInput([]byte("eds the har\n"))
	inputChan <- NewInput([]byte("d limit\n2.2n"))
	inputChan <- NewInput([]byte("d very very v"))
	inputChan <- NewInput([]byte("ery long long"))
	inputChan <- NewInput([]byte("1 2 3 4 5 6 7"))
	inputChan <- NewInput([]byte("1 2 3 4 5 6 7"))
	inputChan <- NewInput([]byte("1 2 3 4 5 6 7"))
	inputChan <- NewInput([]byte("\n"))

	outputs := make([]*Output, 6)
	for i := 0; i < len(outputs); i++ {
		outputs[i] = <-outputChan
	}
	expectedContents := []struct {
		string
		int
	}{
		{
			"1.a very very long\\n log ...TRUNCATED...",
			len("1.a very very long\\n log ")},
		{
			"...TRUNCATED...which exce\\neds the har\\n...TRUNCATED...",
			len("which exce\\neds the har\\n")},
		{
			"...TRUNCATED...d limit",
			len("d limit")},
		{
			"2.2nd very very very long...TRUNCATED...",
			len("2.2nd very very very long")},
		{
			"...TRUNCATED... long1 2 3 4 5 6 71 2 3 4...TRUNCATED...",
			len(" long1 2 3 4 5 6 71 2 3 4")},
		{
			"...TRUNCATED... 5 6 71 2 3 4 5 6 7",
			len(" 5 6 71 2 3 4 5 6 7")}}
	for i := 0; i < len(outputs); i++ {
		output := outputs[i]
		assert.Equal(t, expectedContents[i].string, string(output.Content))
		assert.Equal(t, expectedContents[i].int, output.RawDataLen)
		assert.Equal(t, "", output.Timestamp)
		assert.Equal(t, "", output.Status)
	}
	assertNoMoreOutput(t, outputChan, flushTimeout)
}

func TestLineLongerThanHardLimit(t *testing.T) {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 50)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := NewLineHandlerScheduler(make(chan *RichLine), flushTimeout, handler)
	generator := NewLineGenerator(100, inputChan, &NewLineMatcher{}, &parser.NoopConvertor{}, *scheduler)
	generator.Start()
	inputChan <- NewInput([]byte("1.a very ver"))
	inputChan <- NewInput([]byte("y long log w"))
	inputChan <- NewInput([]byte("hich exceeds"))
	inputChan <- NewInput([]byte(" the hard li"))
	inputChan <- NewInput([]byte("mit of deco"))
	inputChan <- NewInput([]byte("der, it als"))
	inputChan <- NewInput([]byte("o exceed th"))
	inputChan <- NewInput([]byte("e send limi"))
	inputChan <- NewInput([]byte("t of messag"))
	inputChan <- NewInput([]byte("e.\n"))

	outputs := make([]*Output, 3)
	for i := 0; i < len(outputs); i++ {
		outputs[i] = <-outputChan
	}
	expectedContents := []struct {
		string
		int
	}{
		{
			"1.a very very long log which exceeds the hard limi...TRUNCATED...",
			len("1.a very very long log which exceeds the hard limi")},
		{
			"...TRUNCATED...t of decoder, it also exceed the send limit of mes...TRUNCATED...",
			len("t of decoder, it also exceed the send limit of mes")},
		{
			"...TRUNCATED...sage.",
			len("sage.")}}
	for i := 0; i < len(outputs); i++ {
		output := outputs[i]
		assert.Equal(t, expectedContents[i].string, string(output.Content))
		assert.Equal(t, expectedContents[i].int, output.RawDataLen)
		assert.Equal(t, "", output.Timestamp)
		assert.Equal(t, "", output.Status)
	}
	assertNoMoreOutput(t, outputChan, flushTimeout)
}

func TestEmptyLine(t *testing.T) {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output, 3)
	truncator := NewLineTruncator(outputChan, 50)
	flushTimeout := 10 * time.Millisecond
	handler := NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := NewLineHandlerScheduler(make(chan *RichLine), flushTimeout, handler)
	generator := NewLineGenerator(100, inputChan, &NewLineMatcher{}, &parser.NoopConvertor{}, *scheduler)
	generator.Start()

	// empty new line will be ignored.
	inputChan <- NewInput([]byte("1.first chunk "))
	inputChan <- NewInput([]byte("end\n\n"))
	output := <-outputChan
	assert.Equal(t, "1.first chunk end", string(output.Content))
	assertNoMoreOutput(t, outputChan, flushTimeout)

	// line match multiline regex will be sent.
	inputChan <- NewInput([]byte("1.first chunk "))
	inputChan <- NewInput([]byte("end\n2. \n"))
	output = <-outputChan
	assert.Equal(t, "1.first chunk end", string(output.Content))
	output = <-outputChan
	assert.Equal(t, "2. ", string(output.Content))
	assertNoMoreOutput(t, outputChan, flushTimeout)

	// empty line will be kept, since it's from user.
	inputChan <- NewInput([]byte(" \n"))
	output = <-outputChan
	assert.Equal(t, " ", string(output.Content))
	assertNoMoreOutput(t, outputChan, flushTimeout)
}
