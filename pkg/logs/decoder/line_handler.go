// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// truncatedFlag is the flag that is added at the beginning
// or/and at the end of every trucated lines.
var truncatedFlag = []byte("...TRUNCATED...")

// escapedLineFeed is used to escape new line character
// for multiline message.
// New line character needs to be escaped because they are used
// as delimiter for transport.
var escapedLineFeed = []byte(`\n`)

// LineHandler handles raw lines to form structured lines
type LineHandler interface {
	Handle(content []byte)
	Start()
	Stop()
}

// SingleLineHandler takes care of tracking the line length
// and truncating them when they are too long.
type SingleLineHandler struct {
	lineChan       chan []byte
	outputChan     chan *Output
	shouldTruncate bool
	parser         parser.Parser
	lineLimit      int
}

// NewSingleLineHandler returns a new SingleLineHandler.
func NewSingleLineHandler(outputChan chan *Output, parser parser.Parser, lineLimit int) *SingleLineHandler {
	return &SingleLineHandler{
		lineChan:   make(chan []byte),
		outputChan: outputChan,
		parser:     parser,
		lineLimit:  lineLimit,
	}
}

// Handle puts all new lines into a channel for later processing.
func (h *SingleLineHandler) Handle(content []byte) {
	h.lineChan <- content
}

// Stop stops the handler.
func (h *SingleLineHandler) Stop() {
	close(h.lineChan)
}

// Start starts the handler.
func (h *SingleLineHandler) Start() {
	go h.run()
}

// run consumes new lines and processes them.
func (h *SingleLineHandler) run() {
	for line := range h.lineChan {
		h.process(line)
	}
	close(h.outputChan)
}

// process transforms a raw line into a structured line,
// it guarantees that the content of the line won't exceed
// the limit and that the length of the line is properly tracked
// so that the agent restarts tailing from the right place.
func (h *SingleLineHandler) process(line []byte) {
	isTruncated := h.shouldTruncate
	h.shouldTruncate = false

	rawLen := len(line)
	if rawLen < h.lineLimit {
		// lines are delimited on '\n' character when bellow the limit
		// so we need to make sure it's properly accounted
		rawLen++
	}

	content, status, timestamp, _, err := h.parser.Parse(line)
	if err != nil {
		log.Debug(err)
	}
	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		// don't send empty lines
		return
	}

	if isTruncated {
		// the previous line has been truncated because it was too long,
		// the new line is just a remainder,
		// adding the truncated flag at the beginning of the content
		content = append(truncatedFlag, content...)
	}

	if len(content) < h.lineLimit {
		h.outputChan <- NewOutput(content, status, rawLen, timestamp)
	} else {
		// the line is too long, it needs to be cut off and send,
		// adding the truncated flag the end of the content
		content = append(content, truncatedFlag...)
		h.outputChan <- NewOutput(content, status, rawLen, timestamp)
		// make sure the following part of the line will be cut off as well
		h.shouldTruncate = true
	}
}

// defaultFlushTimeout represents the time after which a multiline
// will be be considered as complete.
const defaultFlushTimeout = 1000 * time.Millisecond

// MultiLineHandler makes sure that multiple lines from a same content
// are properly put together.
type MultiLineHandler struct {
	lineChan       chan []byte
	outputChan     chan *Output
	parser         parser.Parser
	newContentRe   *regexp.Regexp
	buffer         *bytes.Buffer
	flushTimeout   time.Duration
	lineLimit      int
	shouldTruncate bool
	linesLen       int
	status         string
	timestamp      string
}

// NewMultiLineHandler returns a new MultiLineHandler.
func NewMultiLineHandler(outputChan chan *Output, newContentRe *regexp.Regexp, flushTimeout time.Duration, parser parser.Parser, lineLimit int) *MultiLineHandler {
	return &MultiLineHandler{
		lineChan:     make(chan []byte),
		outputChan:   outputChan,
		parser:       parser,
		newContentRe: newContentRe,
		buffer:       bytes.NewBuffer(nil),
		flushTimeout: flushTimeout,
		lineLimit:    lineLimit,
	}
}

// Handle forward lines to lineChan to process them.
func (h *MultiLineHandler) Handle(content []byte) {
	h.lineChan <- content
}

// Stop stops the handler.
func (h *MultiLineHandler) Stop() {
	close(h.lineChan)
}

// Start starts the handler.
func (h *MultiLineHandler) Start() {
	go h.run()
}

// run processes new lines from the channel and makes sur the content is properly sent when
// it stayed for too long in the buffer.
func (h *MultiLineHandler) run() {
	flushTimer := time.NewTimer(h.flushTimeout)
	defer func() {
		flushTimer.Stop()
		// make sure the content stored in the buffer gets sent,
		// this can happen when the stop is called in between two timer ticks.
		h.sendBuffer()
		close(h.outputChan)
	}()
	for {
		select {
		case line, isOpen := <-h.lineChan:
			if !isOpen {
				// lineChan has been closed, no more lines are expected
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
			h.process(line)
			flushTimer.Reset(h.flushTimeout)
		case <-flushTimer.C:
			// no line has been collected since a while,
			// the content is supposed to be complete.
			h.sendBuffer()
		}
	}
}

// process aggregates multiple lines to form a full multiline message,
// it stops when a line matches with the new content regular expression.
// It also makes sure that the content will never exceed the limit
// and that the length of the lines is properly tracked
// so that the agent restarts tailing from the right place.
func (h *MultiLineHandler) process(line []byte) {
	content, status, timestamp, _, err := h.parser.Parse(line)
	if err != nil {
		log.Debug(err)
	}

	if h.newContentRe.Match(content) {
		// the current line is part of a new message,
		// send the buffer
		h.sendBuffer()
	}

	isTruncated := h.shouldTruncate
	h.shouldTruncate = false

	rawLen := len(line)
	if rawLen < h.lineLimit {
		// lines are delimited on '\n' character when bellow the limit
		// so we need to make sure it's properly accounted
		rawLen++
	}

	// track the raw data length and the timestamp so that the agent tails
	// from the right place at restart
	h.linesLen += rawLen
	h.timestamp = timestamp
	h.status = status

	if h.buffer.Len() > 0 {
		// the buffer already contains some data which means that
		// the current line is not the first line of the message
		h.buffer.Write(escapedLineFeed)
	}

	if isTruncated {
		// the previous line has been truncated because it was too long,
		// the new line is just a remainder,
		// adding the truncated flag at the beginning of the content
		h.buffer.Write(truncatedFlag)
	}

	h.buffer.Write(content)

	if h.buffer.Len() >= h.lineLimit {
		// the multiline message is too long, it needs to be cut off and send,
		// adding the truncated flag the end of the content
		h.buffer.Write(truncatedFlag)
		h.sendBuffer()
		h.shouldTruncate = true
	}
}

// sendBuffer forwards the content stored in the buffer
// to the output channel.
func (h *MultiLineHandler) sendBuffer() {
	defer func() {
		h.buffer.Reset()
		h.linesLen = 0
		h.shouldTruncate = false
	}()

	data := bytes.TrimSpace(h.buffer.Bytes())
	content := make([]byte, len(data))
	copy(content, data)

	if len(content) > 0 {
		h.outputChan <- NewOutput(content, h.status, h.linesLen, h.timestamp)
	}
}
