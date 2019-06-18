// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
	"time"
)

func TestHandlePartialMessage(t *testing.T) {
	outputChan := make(chan *decoder.Output, 10)
	truncator := decoder.NewLineTruncator(outputChan, 60)
	multiHandler := decoder.NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	handler := NewPartialLineHandler(multiHandler)
	flushTimeout := 1 * time.Second
	scheduler := decoder.NewLineHandlerScheduler(make(chan *decoder.RichLine), flushTimeout, handler)
	scheduler.Start()
	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852911Z").
		Status(message.StatusInfo).
		IsLeading(false).
		IsTailing(false).
		Flag("P").
		ContentString("1.first message ").
		Build())

	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852912Z").
		Status(message.StatusInfo).
		IsLeading(false).
		IsTailing(false).
		Flag("P").
		ContentString("second message ").
		Build())

	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852913Z").
		Status(message.StatusInfo).
		IsLeading(false).
		IsTailing(false).
		Flag("F").
		ContentString("last message").
		Build())

	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852914Z").
		Status(message.StatusCritical).
		IsLeading(false).
		IsTailing(false).
		Flag("P").
		ContentString("2.message").
		Build())

	output := <-outputChan
	assert.Equal(t, "1.first message second message last message", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, 43, output.RawDataLen)

	time.Sleep(1 * time.Second)
	output = <-outputChan
	assert.Equal(t, "2.message", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852914Z", output.Timestamp)
	assert.Equal(t, "critical", output.Status)
	assert.Equal(t, 9, output.RawDataLen)
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
	scheduler.Stop()
}

func TestWrapSingleLineHandler(t *testing.T) {
	outputChan := make(chan *decoder.Output, 10)
	truncator := decoder.NewLineTruncator(outputChan, 60)
	singleLineHandler := decoder.NewSingleHandler(*truncator)
	handler := NewPartialLineHandler(singleLineHandler)
	flushTimeout := 1 * time.Second
	scheduler := decoder.NewLineHandlerScheduler(make(chan *decoder.RichLine), flushTimeout, handler)
	scheduler.Start()
	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852911Z").
		Status(message.StatusInfo).
		IsLeading(false).
		IsTailing(false).
		Flag("P").
		ContentString("1.first message ").
		Build())

	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852912Z").
		Status(message.StatusInfo).
		IsLeading(false).
		IsTailing(false).
		Flag("P").
		ContentString("2.second message ").
		Build())

	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852913Z").
		Status(message.StatusInfo).
		IsLeading(false).
		IsTailing(false).
		Flag("F").
		ContentString("3.last message").
		Build())

	scheduler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852914Z").
		Status(message.StatusCritical).
		IsLeading(false).
		IsTailing(false).
		Flag("P").
		ContentString("2.message").
		Build())

	output := <-outputChan
	assert.Equal(t, "1.first message 2.second message 3.last message", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, 47, output.RawDataLen)

	time.Sleep(1 * time.Second)
	output = <-outputChan
	assert.Equal(t, "2.message", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852914Z", output.Timestamp)
	assert.Equal(t, "critical", output.Status)
	assert.Equal(t, 9, output.RawDataLen)
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)
	scheduler.Stop()
}

func TestBufferFullSendMessage(t *testing.T) {
	outputChan := make(chan *decoder.Output, 3)
	truncator := decoder.NewLineTruncator(outputChan, 60)
	multiHandler := decoder.NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	handler := NewPartialLineHandler(multiHandler)
	handler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852911Z").
		Status(message.StatusInfo).
		IsLeading(false).
		IsTailing(true).
		Flag("F").
		ContentString("1.first message ").
		Build())

	handler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852913Z").
		Status(message.StatusInfo).
		IsLeading(true).
		IsTailing(false).
		Flag("F").
		ContentString("last message").
		Build())
	handler.Handle(decoder.NewRichLineBuilder().
		Timestamp("2019-06-06T16:35:55.930852914Z").
		Status(message.StatusCritical).
		IsLeading(false).
		IsTailing(false).
		Flag("F").
		ContentString("2.message").
		Build())

	output := <-outputChan
	assert.Equal(t, "1.first message ...TRUNCATED...", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, 16, output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...last message", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, 12, output.RawDataLen)
	assertNoMoreOutput(t, outputChan, 10*time.Millisecond)

	handler.Cleanup()
	output = <-outputChan
	assert.Equal(t, "2.message", string(output.Content))
	assert.Equal(t, "2019-06-06T16:35:55.930852914Z", output.Timestamp)
	assert.Equal(t, "critical", output.Status)
	assert.Equal(t, 9, output.RawDataLen)
	assertOutputChanClosed(t, outputChan)
}

func assertNoMoreOutput(t *testing.T, outputChan chan *decoder.Output, waitTimeout time.Duration) {
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

func assertOutputChanClosed(t *testing.T, outputChan chan *decoder.Output) {
	for {
		select {
		case _, open := <-outputChan:
			assert.Equal(t, false, open)
			return
		default:
			assert.Fail(t, "Output channel should be closed")
			return
		}
	}
}
