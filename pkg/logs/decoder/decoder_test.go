// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MockLineHandler struct {
	lineChan chan []byte
}

func NewMockLineHandler() *MockLineHandler {
	return &MockLineHandler{
		lineChan: make(chan []byte, 10),
	}
}

func (h *MockLineHandler) Handle(content []byte) {
	h.lineChan <- content
}

func (h *MockLineHandler) Start() {

}

func (h *MockLineHandler) Stop() {
	close(h.lineChan)
}

const contentLenLimit = 100

func TestDecodeIncomingData(t *testing.T) {
	h := NewMockLineHandler()
	d := New(nil, nil, h, contentLenLimit)

	var line []byte

	// one line in one raw should be sent
	d.decodeIncomingData([]byte("helloworld\n"))
	line = <-h.lineChan
	assert.Equal(t, "helloworld", string(line))
	assert.Equal(t, "", d.lineBuffer.String())

	// multiple lines in one raw should be sent
	d.decodeIncomingData([]byte("helloworld\nhowayou\ngoodandyou"))
	line = <-h.lineChan
	assert.Equal(t, "helloworld", string(line))
	line = <-h.lineChan
	assert.Equal(t, "howayou", string(line))
	assert.Equal(t, "goodandyou", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// multiple lines in multiple rows should be sent
	d.decodeIncomingData([]byte("helloworld\nthisisa"))
	line = <-h.lineChan
	assert.Equal(t, "helloworld", string(line))
	assert.Equal(t, "thisisa", d.lineBuffer.String())
	d.decodeIncomingData([]byte("longinput\nindeed"))
	line = <-h.lineChan
	assert.Equal(t, "thisisalonginput", string(line))
	assert.Equal(t, "indeed", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// one line in multiple rows should be sent
	d.decodeIncomingData([]byte("hello world"))
	d.decodeIncomingData([]byte("!\n"))
	line = <-h.lineChan
	assert.Equal(t, "hello world!", string(line))

	// too long line in one raw should be sent by chuncks
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	line = <-h.lineChan
	assert.Equal(t, contentLenLimit, len(line))
	line = <-h.lineChan
	assert.Equal(t, strings.Repeat("a", 10), string(line))

	// too long line in multiple rows should be sent by chuncks
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit-5)))
	d.decodeIncomingData([]byte(strings.Repeat("a", 15) + "\n"))
	line = <-h.lineChan
	assert.Equal(t, contentLenLimit, len(line))
	line = <-h.lineChan
	assert.Equal(t, strings.Repeat("a", 10), string(line))

	// empty lines should be sent
	d.decodeIncomingData([]byte("\n"))
	line = <-h.lineChan
	assert.Equal(t, "", string(line))
	assert.Equal(t, "", d.lineBuffer.String())

	// empty message should not change anything
	d.decodeIncomingData([]byte(""))
	assert.Equal(t, "", d.lineBuffer.String())
}

func TestDecoderLifeCycle(t *testing.T) {
	h := NewMockLineHandler()
	d := New(nil, nil, h, contentLenLimit)

	// lineHandler should not receive any lines
	d.Start()
	select {
	case <-h.lineChan:
		assert.Fail(t, "LineHandler should not handle anything")
	default:
		break
	}

	// lineHandler should not receive any lines
	h.Stop()
	select {
	case <-h.lineChan:
		break
	default:
		assert.Fail(t, "LineHandler should be stopped")
	}
}
