/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2019 Datadog, Inc.
 */

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
	"time"
)

func TestMultiLineHandler_SendMultiKubesLines(t *testing.T) {
	outputChan := make(chan *decoder.Output, 3)
	var h = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 29)
	h.Start()

	h.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout F 1. message cut by kubernetes "))
	h.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout F continue"))
	h.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F 2. full message"))

	var output *decoder.Output

	output = <-outputChan
	assert.Equal(t, "1. message cut by kubernetes ...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...continue", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123508Z", output.Timestamp)
	assert.Equal(t, "2. full message", string(output.Content))

	h.Stop()
}

func TestSingleLineHandler_HandleKubesLine(t *testing.T) {
	outputChan := make(chan *decoder.Output, 3)
	h := decoder.NewSingleLineHandler(outputChan, Parser, 10)
	h.Start()

	h.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout F 1. message longer "))
	h.Handle([]byte("than 10"))
	h.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F 2. message longer than limit"))

	var output *decoder.Output

	output = <-outputChan
	assert.Equal(t, "1. message longer...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...than 10", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123508Z", output.Timestamp)
	assert.Equal(t, "2. message longer than limit...TRUNCATED...", string(output.Content))
}

func TestLineHandler_FailedParseMessage(t *testing.T) {
	var outputChan = make(chan *decoder.Output)
	var lineHandlerRunner = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 60)
	var wrapper = NewLineHandler(lineHandlerRunner, 10*time.Microsecond, 60)
	wrapper.Start()
	wrapper.Handle([]byte("1. invalid"))
	var output *decoder.Output
	output = <-outputChan
	assert.Equal(t, "1. invalid", string(output.Content))
	wrapper.Stop()
}

func TestLineHandler_HandleFullMessage(t *testing.T) {
	var outputChan = make(chan *decoder.Output)
	var lineHandlerRunner = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 100)
	var wrapper = NewLineHandler(lineHandlerRunner, 2*time.Second, 100)
	wrapper.Start()
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout F 1. first full message"))
	var output *decoder.Output
	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123506Z", output.Timestamp)
	assert.Equal(t, "1. first full message", string(output.Content))
	wrapper.Stop()
}

func TestLineHandler_MergePartialMessage(t *testing.T) {
	var outputChan = make(chan *decoder.Output)

	var lineHandlerRunner = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 70)
	var wrapper = NewLineHandler(lineHandlerRunner, 100*time.Microsecond, 70)
	wrapper.Start()

	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout P 1. message cut by "))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123507Z stdout F kubernetes"))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F 2. full message"))
	var output *decoder.Output
	output = <-outputChan
	assert.Equal(t, "1. message cut by kubernetes", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123508Z", output.Timestamp)
	assert.Equal(t, "2. full message", string(output.Content))

	wrapper.Stop()
}

func TestLineHandler_CutByDecoder(t *testing.T) {
	var outputChan = make(chan *decoder.Output, 2)

	var lineHandlerRunner = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 56)
	var wrapper = NewLineHandler(lineHandlerRunner, 2*time.Second, 56)
	wrapper.Start()

	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout P 1. message cut by "))
	wrapper.Handle([]byte("decoder "))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F last message"))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F 2. second message"))

	var output *decoder.Output
	output = <-outputChan
	assert.Equal(t, "1. message cut by decoder last message", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123508Z", output.Timestamp)
	assert.Equal(t, "2. second message", string(output.Content))

	wrapper.Stop()
}

func TestLineHandler_FullMessageCutByDecoder(t *testing.T) {
	var outputChan = make(chan *decoder.Output, 2)

	var lineHandlerRunner = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 56)
	var wrapper = NewLineHandler(lineHandlerRunner, 10*time.Microsecond, 56)
	wrapper.Start()

	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout F 1. message cut by "))
	wrapper.Handle([]byte("decoder"))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F 2. second message"))
	var output *decoder.Output
	output = <-outputChan
	assert.Equal(t, "1. message cut by \\ndecoder", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123508Z", output.Timestamp)
	assert.Equal(t, "2. second message", string(output.Content))

	wrapper.Stop()
}

func TestLineHandler_ContentLineTooLong(t *testing.T) {
	var outputChan = make(chan *decoder.Output, 3)

	var lineHandlerRunner = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 20)
	var wrapper = NewLineHandler(lineHandlerRunner, 10*time.Microsecond, 20)
	wrapper.Start()

	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout P 1. message cut by "))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout F application"))
	var output *decoder.Output
	output = <-outputChan
	assert.Equal(t, "1. message cut by ap...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123506Z", output.Timestamp)
	assert.Equal(t, "...TRUNCATED...plication...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123506Z", output.Timestamp)
	assert.Equal(t, "...TRUNCATED...", string(output.Content))

	wrapper.Stop()
}

func TestLineHandler_ExceedContentLengthLimit(t *testing.T) {
	var outputChan = make(chan *decoder.Output, 3)
	var lineHandlerRunner = decoder.NewMultiLineHandler(outputChan, regexp.MustCompile("[0-9]+\\. "), 10*time.Microsecond, Parser, 22)
	var wrapper = NewLineHandler(lineHandlerRunner, 10*time.Microsecond, 22)
	wrapper.Start()

	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout P 1. message cut "))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123507Z stdout P by "))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F application"))
	var output *decoder.Output
	output = <-outputChan
	assert.Equal(t, "1. message cut by appl...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...ication...TRUNCATED...", string(output.Content))

	output = <-outputChan
	assert.Equal(t, "2019-05-15T13:34:26.011123508Z", output.Timestamp)
	assert.Equal(t, "...TRUNCATED...", string(output.Content))
	wrapper.Stop()
}

func TestLineHandler_WrapSingleLineHandler(t *testing.T) {
	var outputChan = make(chan *decoder.Output, 2)
	var lineHandlerRunner = decoder.NewSingleLineHandler(outputChan, Parser, 22)
	var wrapper = NewLineHandler(lineHandlerRunner, 10*time.Microsecond, 22)
	wrapper.Start()

	wrapper.Handle([]byte("2019-05-15T13:34:26.011123506Z stdout P 1. message cut "))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123507Z stdout P by "))
	wrapper.Handle([]byte("2019-05-15T13:34:26.011123508Z stdout F application"))
	var output *decoder.Output
	output = <-outputChan

	assert.Equal(t, "1. message cut by appl...TRUNCATED...", string(output.Content))

	output = <-outputChan

	assert.Equal(t, "2019-05-15T13:34:26.011123508Z", output.Timestamp)
	assert.Equal(t, "...TRUNCATED...ication", string(output.Content))
	wrapper.Stop()
}
