// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import "time"

// GDecoder should replace current Decoder object.
type GDecoder struct {
	InputChan     chan *Input
	OutputChan    chan *Output
	lineGenerator LineGenerator
}

func (d *GDecoder) Start() {
	d.lineGenerator.Start()
}

func (d *GDecoder) Stop() {
	close(d.InputChan)
}

const defaultMaxDecodeLength = 1024 * 1024 // 1MB
const defaultMaxSendLength = 256 * 1024    // 256KB

type LineHandlerScheduler struct {
	inputChan    chan *RichLine
	flushTimeout time.Duration
	lineHandler  Handler
}

func (l *LineHandlerScheduler) Stop() {
	close(l.inputChan)
}

func (l *LineHandlerScheduler) Handle(line *RichLine) {
	l.inputChan <- line
}

func (l *LineHandlerScheduler) Start() {
	go l.run(l.lineHandler.Handle, l.lineHandler.SendResult)
}

func (l *LineHandlerScheduler) run(handle func(line *RichLine), sendResult func()) {
	timer := time.NewTimer(l.flushTimeout)
	// cleanup
	defer func() {
		l.lineHandler.SendResult()
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
