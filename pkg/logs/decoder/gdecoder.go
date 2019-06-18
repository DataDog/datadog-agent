// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"time"
)

// GDecoder should replace current Decoder object.
// Decoder encapsulates the logic for processing logs. The communication is done by
// InputChan and OutputChan channels. Logs are received by byte arrays as contents of
// Inputs and sent with extra formatting and resizing as contents of Outputs.
//
// The main components of decoder logic are: LineGenerator, LineHandlerScheduler,
// LineHandler, LineTruncator.
//
// The data flows like this:
// Input -> LineGenerator(cache and convert) -> RichLine -> LineHandlerScheduler ->
// LineHandler -> RichLine(s) -> LineTruncator -> Output
//
// The main job of LineGenerator is to read Inputs byte by byte forms Line with extra
// information and send to downstream.
// LineHandlerScheduler keeps a timer to make sure LineHandlers clean up their caches
// before terminating.
// LineHandler handles the Line according to specifications.
// LineTruncator finally apply the truncation rules that are accumulated by the upstream.
type GDecoder struct {
	InputChan     chan *Input
	OutputChan    chan *Output
	lineGenerator LineGenerator
}

// Start prepares routine for consuming inputs.
func (d *GDecoder) Start() {
	d.lineGenerator.Start()
}

// Stop closes the input channel in order to terminate the decoding job.
func (d *GDecoder) Stop() {
	close(d.InputChan)
}

const defaultMaxDecodeLength = 1024 * 1024 // 1MB
const defaultMaxSendLength = 256 * 1024    // 256KB

// LineHandlerScheduler contains the logic to process / reads input channel with
// a timer. It's to make sure, when timer times out, to clean up the cache of
// line handler, if exists, before termination.
type LineHandlerScheduler struct {
	inputChan    chan *RichLine
	flushTimeout time.Duration
	lineHandler  Handler
}

// NewLineHandlerScheduler create a new instance of handler scheduler.
func NewLineHandlerScheduler(inputChan chan *RichLine, flushTimeout time.Duration, handler Handler) *LineHandlerScheduler {
	return &LineHandlerScheduler{
		inputChan:    inputChan,
		flushTimeout: flushTimeout,
		lineHandler:  handler,
	}
}

// Stop closes the input channel to stop receiving inputs.
func (l *LineHandlerScheduler) Stop() {
	close(l.inputChan)
}

// Handle puts the specific line to input channel.
func (l *LineHandlerScheduler) Handle(line *RichLine) {
	l.inputChan <- line
}

// Start starts a routine for consuming input channel.
func (l *LineHandlerScheduler) Start() {
	go l.run(l.lineHandler.Handle, l.lineHandler.SendResult)
}

// run sets a timer and consumes input channel. It resets the timer for each handle
// operation. Once timeout, the cleanup will be done.
func (l *LineHandlerScheduler) run(handle func(line *RichLine), sendResult func()) {
	timer := time.NewTimer(l.flushTimeout)
	// cleanup
	defer func() {
		l.lineHandler.Cleanup()
		timer.Stop()
	}()
	// run processing logic
	for {
		select {
		case line, isOpen := <-l.inputChan:
			if !isOpen {
				// InputChan has been closed, no new line are expected
				return
			}
			// Stops the timer and process the line, then restart the timer.
			if !timer.Stop() {
				// timer has already expired or been stopped, check the
				// return value and drain the channel.
				select {
				case <-timer.C:
				default:
				}
			}
			handle(line)
			timer.Reset(l.flushTimeout)
		case <-timer.C: // when timeout, handles existing result.
			sendResult()
		}
	}
}
