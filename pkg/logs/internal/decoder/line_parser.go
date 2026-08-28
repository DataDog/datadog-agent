// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"sort"
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

	// flushChan returns a channel which will deliver a message when flushTimedOut should be called.
	flushChan() <-chan time.Time

	// flush forces all partially-processed data downstream when the decoder stops.
	flush()

	// flushTimedOut flushes data whose aggregation timeout has elapsed. It should
	// be called when flushChan delivers a message.
	flushTimedOut()
}

// SingleLineParser makes sure that multiple lines from a same content
// are properly put together.
type SingleLineParser struct {
	lineHandler LineHandler
	parser      parsers.Parser
}

// NewSingleLineParser returns a new SingleLineParser.
func NewSingleLineParser(
	lineHandler LineHandler,
	parser parsers.Parser) *SingleLineParser {
	return &SingleLineParser{
		lineHandler: lineHandler,
		parser:      parser,
	}
}

func (p *SingleLineParser) flushChan() <-chan time.Time {
	return nil
}

func (p *SingleLineParser) flush() {
	// do nothing
}

func (p *SingleLineParser) flushTimedOut() {
	// do nothing
}

func (p *SingleLineParser) process(input *message.Message, rawDataLen int) {
	// Just parse and pass to the next step
	input, err := p.parser.Parse(input)
	if err != nil {
		log.Debug(err)
	}
	input.RawDataLen = rawDataLen
	p.lineHandler.process(input)
}

type partialLineState struct {
	bufferedMsg       *message.Message
	buffer            bytes.Buffer
	rawDataLen        int
	isBufferTruncated bool
	deadline          time.Time
	sequence          uint64
}

// MultiLineParser makes sure that chunked lines are properly put together.
type MultiLineParser struct {
	lineHandler LineHandler

	// Partial lines are accumulated independently by stream. Parsers that do not
	// identify a stream use the empty-string entry and retain the original
	// single-buffer behavior.
	buffers      map[string]*partialLineState
	nextSequence uint64

	// configuration attributes

	flushTimeout time.Duration
	flushTimer   *time.Timer
	parser       parsers.Parser
	lineLimit    int
}

// NewMultiLineParser returns a new MultiLineParser.
func NewMultiLineParser(
	lineHandler LineHandler,
	flushTimeout time.Duration,
	parser parsers.Parser,
	lineLimit int,
) *MultiLineParser {
	return &MultiLineParser{
		lineHandler:  lineHandler,
		buffers:      make(map[string]*partialLineState),
		flushTimeout: flushTimeout,
		flushTimer:   nil,
		lineLimit:    lineLimit,
		parser:       parser,
	}
}

func (p *MultiLineParser) flushChan() <-chan time.Time {
	if p.flushTimer != nil && len(p.buffers) > 0 {
		return p.flushTimer.C
	}
	return nil
}

func (p *MultiLineParser) flush() {
	p.stopFlushTimer()
	for _, stream := range p.bufferStreams() {
		p.sendLine(stream)
	}
}

func (p *MultiLineParser) flushTimedOut() {
	p.stopFlushTimer()
	now := time.Now()
	for _, stream := range p.bufferStreams() {
		if !p.buffers[stream].deadline.After(now) {
			p.sendLine(stream)
		}
	}
	p.resetFlushTimer()
}

// process buffers and aggregates partial lines
func (p *MultiLineParser) process(input *message.Message, rawDataLen int) {
	p.stopFlushTimer()

	msg, err := p.parser.Parse(input)
	if err != nil {
		log.Debug(err)
	}

	stream := msg.ParsingExtra.Stream
	state, found := p.buffers[stream]
	if !found {
		state = &partialLineState{sequence: p.nextSequence}
		p.nextSequence++
		p.buffers[stream] = state
	}

	// track the raw data length and the timestamp so that the agent tails
	// from the right place at restart
	state.rawDataLen += rawDataLen
	state.buffer.Write(msg.GetContent())
	state.bufferedMsg = msg

	if state.buffer.Len() >= p.lineLimit {
		// buffer exceeds size cap — mark as truncated and let SingleLineHandler
		// handle the ...TRUNCATED... byte markers and metric downstream
		state.isBufferTruncated = true
	}

	if !msg.ParsingExtra.IsPartial || state.buffer.Len() >= p.lineLimit {
		// the current chunk marks the end of an aggregated line
		p.sendLine(stream)
	} else {
		state.deadline = time.Now().Add(p.flushTimeout)
	}

	p.resetFlushTimer()
}

// sendLine forwards the content stored for one stream.
func (p *MultiLineParser) sendLine(stream string) {
	state, found := p.buffers[stream]
	if !found {
		return
	}
	defer delete(p.buffers, stream)

	// Skip only when there is nothing to send. Complete but empty lines (empty
	// content, non-zero rawDataLen) are still forwarded so downstream
	// aggregators can observe blank lines.
	if state.bufferedMsg == nil || state.rawDataLen == 0 {
		return
	}

	content := make([]byte, state.buffer.Len())
	copy(content, state.buffer.Bytes())
	state.bufferedMsg.RawDataLen = state.rawDataLen
	state.bufferedMsg.SetContent(content)
	state.bufferedMsg.ParsingExtra.IsTruncated = state.bufferedMsg.ParsingExtra.IsTruncated || state.isBufferTruncated
	p.lineHandler.process(state.bufferedMsg)
}

func (p *MultiLineParser) bufferStreams() []string {
	streams := make([]string, 0, len(p.buffers))
	for stream := range p.buffers {
		streams = append(streams, stream)
	}
	sort.Slice(streams, func(i, j int) bool {
		return p.buffers[streams[i]].sequence < p.buffers[streams[j]].sequence
	})
	return streams
}

func (p *MultiLineParser) stopFlushTimer() {
	if p.flushTimer == nil {
		return
	}
	if !p.flushTimer.Stop() {
		select {
		case <-p.flushTimer.C:
		default:
		}
	}
}

func (p *MultiLineParser) resetFlushTimer() {
	if len(p.buffers) == 0 {
		return
	}

	var earliest time.Time
	for _, state := range p.buffers {
		if earliest.IsZero() || state.deadline.Before(earliest) {
			earliest = state.deadline
		}
	}

	delay := time.Until(earliest)
	if delay < 0 {
		delay = 0
	}
	if p.flushTimer == nil {
		p.flushTimer = time.NewTimer(delay)
	} else {
		p.flushTimer.Reset(delay)
	}
}
