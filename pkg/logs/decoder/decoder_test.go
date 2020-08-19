// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package decoder

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"

	"github.com/stretchr/testify/assert"
)

type MockLineParser struct {
	inputChan chan *DecodedInput
}

func NewMockLineParser() *MockLineParser {
	return &MockLineParser{
		inputChan: make(chan *DecodedInput, 10),
	}
}

func (p *MockLineParser) Handle(input *DecodedInput) {
	p.inputChan <- input
}

func (p *MockLineParser) Start() {

}

func (p *MockLineParser) Stop() {
	close(p.inputChan)
}

const contentLenLimit = 100

func TestDecodeIncomingData(t *testing.T) {
	p := NewMockLineParser()
	d := New(nil, nil, p, contentLenLimit, &newLineMatcher{})

	var line *DecodedInput

	// one line in one raw should be sent
	d.decodeIncomingData([]byte("helloworld\n"))
	line = <-p.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "", d.lineBuffer.String())

	// multiple lines in one raw should be sent
	d.decodeIncomingData([]byte("helloworld\nhowayou\ngoodandyou"))
	line = <-p.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	line = <-p.inputChan
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, "goodandyou", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// multiple lines in multiple rows should be sent
	d.decodeIncomingData([]byte("helloworld\nthisisa"))
	line = <-p.inputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, "thisisa", d.lineBuffer.String())
	d.decodeIncomingData([]byte("longinput\nindeed"))
	line = <-p.inputChan
	assert.Equal(t, "thisisalonginput", string(line.content))
	assert.Equal(t, "indeed", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// one line in multiple rows should be sent
	d.decodeIncomingData([]byte("hello world"))
	d.decodeIncomingData([]byte("!\n"))
	line = <-p.inputChan
	assert.Equal(t, "hello world!", string(line.content))

	// too long line in one raw should be sent by chuncks
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	line = <-p.inputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	line = <-p.inputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))

	// too long line in multiple rows should be sent by chuncks
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit-5)))
	d.decodeIncomingData([]byte(strings.Repeat("a", 15) + "\n"))
	line = <-p.inputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	line = <-p.inputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))

	// empty lines should be sent
	d.decodeIncomingData([]byte("\n"))
	line = <-p.inputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, "", d.lineBuffer.String())

	// empty message should not change anything
	d.decodeIncomingData([]byte(""))
	assert.Equal(t, "", d.lineBuffer.String())
}

func TestDecoderLifeCycle(t *testing.T) {
	p := NewMockLineParser()
	d := New(nil, nil, p, contentLenLimit, &newLineMatcher{})

	// LineParser should not receive any lines
	d.Start()
	select {
	case <-p.inputChan:
		assert.Fail(t, "LineParser should not handle anything")
	default:
		break
	}

	// LineParser should not receive any lines
	p.Stop()
	select {
	case <-p.inputChan:
		break
	default:
		assert.Fail(t, "LineParser should be stopped")
	}
}

func TestDecoderInputNotDockerHeader(t *testing.T) {
	inputChan := make(chan *Input)
	h := NewMockLineParser()
	d := New(inputChan, nil, h, 100, &newLineMatcher{})
	d.Start()

	input := []byte("hello")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	inputChan <- NewInput(input)

	var output *DecodedInput
	output = <-h.inputChan
	expected1 := append([]byte("hello"), []byte{1, 0, 0, 0, 0}...)
	assert.Equal(t, expected1, output.content)

	output = <-h.inputChan
	expected2 := append([]byte{0, 0}, []byte("2018-06-14T18:27:03.246999277Z app logs")...)
	assert.Equal(t, expected2, output.content)
	d.Stop()
}

func TestDecoderWithDockerHeader(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoder(source, parser.NoopParser)
	d.Start()

	input := []byte("hello\n")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	d.InputChan <- NewInput(input)

	var output *Message
	output = <-d.OutputChan
	assert.Equal(t, "hello", string(output.Content))

	output = <-d.OutputChan
	expected := []byte{1, 0, 0, 0, 0}
	assert.Equal(t, expected, output.Content)

	output = <-d.OutputChan
	expected = append([]byte{0, 0}, []byte("2018-06-14T18:27:03.246999277Z app logs")...)
	assert.Equal(t, expected, output.Content)
	d.Stop()
}
