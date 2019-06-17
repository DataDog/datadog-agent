// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
	"time"
)

func TestSchedulerFlushTimeout(t *testing.T) {
	outputChan := make(chan *Output)
	truncator := NewLineTruncator(outputChan, 60)
	flushTimeout := 10 * time.Millisecond

	scheduler := &LineHandlerScheduler{
		inputChan:    make(chan *RichLine),
		flushTimeout: flushTimeout,
		lineHandler:  NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator),
	}
	scheduler.Start()
	scheduler.Handle(richLine("last message", false, false))

	output := <-outputChan
	assert.Equal(t, "last message", string(output.Content))
	assertNoMoreOutput(t, outputChan, flushTimeout)
	scheduler.Stop()
}

func TestSchedulerHandleResultBeforeInputChanClose(t *testing.T) {
	outputChan := make(chan *Output)
	truncator := NewLineTruncator(outputChan, 60)
	scheduler := &LineHandlerScheduler{
		inputChan:    make(chan *RichLine),
		flushTimeout: 30 * time.Minute,
		lineHandler:  NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator),
	}
	scheduler.Start()
	scheduler.Handle(richLine("last message", false, false))
	scheduler.Stop()
	output := <-outputChan
	assert.Equal(t, "last message", string(output.Content))
	assert.Equal(t, len("last message"), output.RawDataLen)
	assertOutputChanClosed(t, outputChan)
}

func assertOutputChanClosed(t *testing.T, outputChan chan *Output) {
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
