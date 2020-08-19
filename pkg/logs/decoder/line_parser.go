// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package decoder

import (
	"bytes"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LineParser e
type LineParser interface {
	Handle(input *DecodedInput)
	Start()
	Stop()
}

// SingleLineParser makes sure that multiple lines from a same content
// are properly put together.
type SingleLineParser struct {
	parser      parser.Parser
	inputChan   chan *DecodedInput
	lineHandler LineHandler
}

// NewSingleLineParser returns a new MultiLineHandler.
func NewSingleLineParser(parser parser.Parser, lineHandler LineHandler) *SingleLineParser {
	return &SingleLineParser{
		parser:      parser,
		inputChan:   make(chan *DecodedInput),
		lineHandler: lineHandler,
	}
}

// Handle puts all new lines into a channel for later processing.
func (p *SingleLineParser) Handle(input *DecodedInput) {
	p.inputChan <- input
}

// Start starts the parser.
func (p *SingleLineParser) Start() {
	p.lineHandler.Start()
	go p.run()
}

// Stop stops the parser.
func (p *SingleLineParser) Stop() {
	close(p.inputChan)
}

// run consumes new lines and processes them.
func (p *SingleLineParser) run() {
	for input := range p.inputChan {
		p.process(input)
	}
	p.lineHandler.Stop()
}

func (p *SingleLineParser) process(input *DecodedInput) {
	// Just parse an pass to the next step
	content, status, timestamp, _, err := p.parser.Parse(input.content)
	if err != nil {
		log.Debug(err)
	}
	p.lineHandler.Handle(NewMessage(content, status, input.rawDataLen, timestamp))
}

// MultiLineParser makes sure that chunked lines are properly put together.
type MultiLineParser struct {
	buffer       *bytes.Buffer
	flushTimeout time.Duration
	inputChan    chan *DecodedInput
	lineHandler  LineHandler
	parser       parser.Parser
	rawDataLen   int
	lineLimit    int
	status       string
	timestamp    string
}

// NewMultiLineParser returns a new MultiLineHandler.
func NewMultiLineParser(flushTimeout time.Duration, parser parser.Parser, lineHandler LineHandler, lineLimit int) *MultiLineParser {
	return &MultiLineParser{
		inputChan:    make(chan *DecodedInput),
		buffer:       bytes.NewBuffer(nil),
		flushTimeout: flushTimeout,
		lineHandler:  lineHandler,
		lineLimit:    lineLimit,
		parser:       parser,
	}
}

// Handle forward lines to lineChan to process them.
func (p *MultiLineParser) Handle(input *DecodedInput) {
	p.inputChan <- input
}

// Stop stops the handler.
func (p *MultiLineParser) Stop() {
	close(p.inputChan)
}

// Start starts the handler.
func (p *MultiLineParser) Start() {
	p.lineHandler.Start()
	go p.run()
}

// run processes new lines from the channel and makes sur the content is properly sent when
// it stayed for too long in the buffer.
func (p *MultiLineParser) run() {
	flushTimer := time.NewTimer(p.flushTimeout)
	defer func() {
		flushTimer.Stop()
		// make sure the content stored in the buffer gets sent,
		// this can happen when the stop is called in between two timer ticks.
		p.sendLine()
		p.lineHandler.Stop()
	}()
	for {
		select {
		case message, isOpen := <-p.inputChan:
			if !isOpen {
				//  inputChan has been closed, no more lines are expected
				return
			}
			// process the new line and restart the timeout
			if !flushTimer.Stop() {
				// timer stop doesn't not prevent the timer to tick,
				// makes sure the event is consumed to avoid sending
				// just one piece of the content.
				select {
				case <-flushTimer.C:
				default:
				}
			}
			p.process(message)
			flushTimer.Reset(p.flushTimeout)
		case <-flushTimer.C:
			// no chunk has been collected since a while,
			// the content is supposed to be complete.
			p.sendLine()
		}
	}
}

// process buffers and aggregates partial lines
func (p *MultiLineParser) process(input *DecodedInput) {
	content, status, timestamp, partial, err := p.parser.Parse(input.content)
	if err != nil {
		log.Debug(err)
	}
	// track the raw data length and the timestamp so that the agent tails
	// from the right place at restart
	p.rawDataLen += input.rawDataLen
	p.timestamp = timestamp
	p.status = status
	p.buffer.Write(content)

	if !partial || p.buffer.Len() >= p.lineLimit {
		// the current chunk marks the end of an aggregated line
		p.sendLine()
	}
}

// sendBuffer forwards the content stored in the buffer
// to the output channel.
func (p *MultiLineParser) sendLine() {
	defer func() {
		p.buffer.Reset()
		p.rawDataLen = 0
	}()

	content := make([]byte, p.buffer.Len())
	copy(content, p.buffer.Bytes())
	if len(content) > 0 || p.rawDataLen > 0 {
		p.lineHandler.Handle(NewMessage(content, p.status, p.rawDataLen, p.timestamp))
	}
}
