// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LineParser handles decoded lines, parsing them into decoder.Message's using
// an embedded parsers.Parser.
type LineParser interface {
	// process handles a new line (message)
	process(content *message.Message, rawDataLen int)

	// flushChan returns a channel which will deliver a message when `flush` should be called.
	flushChan() <-chan time.Time

	// flush flushes partially-processed data.  It should be called either when flushChan has
	// a message, or when the decoder is stopped.
	flush()
}

// SingleLineParser makes sure that multiple lines from a same content
// are properly put together.
type SingleLineParser struct {
	outputFn func(*message.Message)
	parser   parsers.Parser
}

// NewSingleLineParser returns a new SingleLineParser.
func NewSingleLineParser(
	outputFn func(*message.Message),
	parser parsers.Parser) *SingleLineParser {
	return &SingleLineParser{
		outputFn: outputFn,
		parser:   parser,
	}
}

func (p *SingleLineParser) flushChan() <-chan time.Time {
	return nil
}

func (p *SingleLineParser) flush() {
	// do nothing
}

func (p *SingleLineParser) process(input *message.Message, rawDataLen int) {
	// Just parse and pass to the next step
	input, err := p.parser.Parse(input)
	if err != nil {
		log.Debug(err)
	}
	input.RawDataLen = rawDataLen
	p.outputFn(input)
}

// MultiLineParser makes sure that chunked lines are properly put together.
type MultiLineParser struct {
	outputFn func(*message.Message)

	// used to reconstruct the message

	bufferedMsg *message.Message
	buffer      *bytes.Buffer
	rawDataLen  int

	// configuration attributes

	flushTimeout time.Duration
	flushTimer   *time.Timer
	parser       parsers.Parser
	lineLimit    int
}

// NewMultiLineParser returns a new MultiLineParser.
func NewMultiLineParser(
	outputFn func(*message.Message),
	flushTimeout time.Duration,
	parser parsers.Parser,
	lineLimit int,
) *MultiLineParser {
	return &MultiLineParser{
		outputFn:     outputFn,
		buffer:       bytes.NewBuffer(nil),
		bufferedMsg:  nil,
		flushTimeout: flushTimeout,
		flushTimer:   nil,
		lineLimit:    lineLimit,
		parser:       parser,
	}
}

func (p *MultiLineParser) flushChan() <-chan time.Time {
	if p.flushTimer != nil && p.buffer.Len() > 0 {
		return p.flushTimer.C
	}
	return nil
}

func (p *MultiLineParser) flush() {
	p.sendLine()
}

// process buffers and aggregates partial lines
func (p *MultiLineParser) process(input *message.Message, rawDataLen int) {
	if p.flushTimer != nil && p.buffer.Len() > 0 {
		// stop the flush timer, as we now have data
		if !p.flushTimer.Stop() {
			<-p.flushTimer.C
		}
	}
	msg, err := p.parser.Parse(input)
	if err != nil {
		log.Debug(err)
	}
	// track the raw data length and the timestamp so that the agent tails
	// from the right place at restart
	p.rawDataLen += rawDataLen
	p.buffer.Write(msg.GetContent())
	p.bufferedMsg = msg

	if !msg.ParsingExtra.IsPartial || p.buffer.Len() >= p.lineLimit {
		// the current chunk marks the end of an aggregated line
		p.sendLine()
	}
	if p.buffer.Len() > 0 {
		// since there's buffered data, start the flush timer to flush it
		if p.flushTimer == nil {
			p.flushTimer = time.NewTimer(p.flushTimeout)
		} else {
			p.flushTimer.Reset(p.flushTimeout)
		}
	}
}

// sendBuffer forwards the content stored in the buffer
func (p *MultiLineParser) sendLine() {
	defer func() {
		p.buffer.Reset()
		p.bufferedMsg = nil
		p.rawDataLen = 0
	}()

	if p.bufferedMsg == nil || p.buffer.Len() == 0 {
		return
	}

	content := make([]byte, p.buffer.Len())
	copy(content, p.buffer.Bytes())
	if len(content) > 0 || p.rawDataLen > 0 {
		p.bufferedMsg.RawDataLen = p.rawDataLen
		p.bufferedMsg.SetContent(content)
		p.outputFn(p.bufferedMsg)
	}
}
