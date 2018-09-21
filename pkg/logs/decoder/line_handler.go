// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
)

// TRUNCATED is the warning we add at the beginning or/and at the end of a truncated message
var TRUNCATED = []byte("...TRUNCATED...")

// LineHandler handles byte slices to form line output
type LineHandler interface {
	Handle(content []byte)
	Start()
	Stop()
}

// SingleLineHandler creates and forward outputs to outputChan from single-lines
type SingleLineHandler struct {
	lineChan       chan []byte
	outputChan     chan *Output
	shouldTruncate bool
	parser         parser.Parser
}

// NewSingleLineHandler returns a new SingleLineHandler
func NewSingleLineHandler(outputChan chan *Output, parser parser.Parser) *SingleLineHandler {
	return &SingleLineHandler{
		lineChan:   make(chan []byte),
		outputChan: outputChan,
		parser:     parser,
	}
}

// Handle trims leading and trailing whitespaces from content,
// and sends it as a new Line to lineChan.
func (h *SingleLineHandler) Handle(content []byte) {
	h.lineChan <- content
}

// Stop stops the handler from processing new lines
func (h *SingleLineHandler) Stop() {
	close(h.lineChan)
}

// Start starts the handler
func (h *SingleLineHandler) Start() {
	go h.run()
}

// run consumes lines from lineChan to process them
func (h *SingleLineHandler) run() {
	for line := range h.lineChan {
		h.process(line)
	}
	close(h.outputChan)
}

// process creates outputs from lines and forwards them to outputChan
// When lines are too long, they are truncated
func (h *SingleLineHandler) process(line []byte) {
	lineLen := len(line)
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}

	var content []byte
	if h.shouldTruncate {
		// add TRUNCATED at the beginning of content
		content = append(TRUNCATED, line...)
		h.shouldTruncate = false
	} else {
		// keep content the same
		content = line
	}

	if lineLen < contentLenLimit {
		// send content
		// add 1 to take into account '\n' that we didn't include in content
		parsedMessage, _ := h.parser(content)
		output := NewOutput(parsedMessage.Content, lineLen+1)
		h.outputChan <- output
	} else {
		// add TRUNCATED at the end of content and send it
		content := append(content, TRUNCATED...)
		parsedMessage, _ := h.parser(content)
		output := NewOutput(parsedMessage.Content, lineLen)
		h.outputChan <- output
		h.shouldTruncate = true
	}
}

// defaultFlushTimeout represents the time we want to wait before flushing lineBuffer
// when no more line is received
const defaultFlushTimeout = 1000 * time.Millisecond

// MultiLineHandler reads lines from lineChan and uses lineBuffer to send them
// when a new line matches with re or flushTimer is fired
type MultiLineHandler struct {
	lineChan      chan []byte
	outputChan    chan *Output
	lineBuffer    *LineBuffer
	lineUnwrapper LineUnwrapper
	newContentRe  *regexp.Regexp
	flushTimeout  time.Duration
}

// NewMultiLineHandler returns a new MultiLineHandler
func NewMultiLineHandler(outputChan chan *Output, newContentRe *regexp.Regexp, flushTimeout time.Duration, lineUnwrapper LineUnwrapper) *MultiLineHandler {
	return &MultiLineHandler{
		lineChan:      make(chan []byte),
		outputChan:    outputChan,
		lineBuffer:    NewLineBuffer(),
		lineUnwrapper: lineUnwrapper,
		newContentRe:  newContentRe,
		flushTimeout:  flushTimeout,
	}
}

// Handle forward lines to lineChan to process them
func (h *MultiLineHandler) Handle(content []byte) {
	h.lineChan <- content
}

// Stop stops the lineHandler from processing lines
func (h *MultiLineHandler) Stop() {
	close(h.lineChan)
}

// Start starts the handler
func (h *MultiLineHandler) Start() {
	go h.run()
}

// run processes new lines from lineChan and flushes the buffer when the timeout expires
func (h *MultiLineHandler) run() {
	flushTimer := time.NewTimer(h.flushTimeout)
	defer func() {
		flushTimer.Stop()
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
			flushTimer.Stop()
			h.process(line)
			flushTimer.Reset(h.flushTimeout)
		case <-flushTimer.C:
			// the timout expired, the content is ready to be sent
			h.sendContent()
		}
	}
}

// process accumulates lines in lineBuffer and flushes lineBuffer when a new line matches with newContentRe
// When lines are too long, they are truncated
func (h *MultiLineHandler) process(line []byte) {
	unwrappedLine := h.lineUnwrapper.Unwrap(line)
	if h.newContentRe.Match(unwrappedLine) {
		// send content from lineBuffer
		h.sendContent()
	}
	if !h.lineBuffer.IsEmpty() {
		// unwrap all the following lines
		line = unwrappedLine
		// add '\n' to content in lineBuffer
		h.lineBuffer.AddEndOfLine()
	}
	if len(line)+h.lineBuffer.Length() < contentLenLimit {
		// add line to content in lineBuffer
		h.lineBuffer.Add(line)
	} else {
		// add line and truncate and flush content in lineBuffer
		h.lineBuffer.AddIncompleteLine(line)
		h.lineBuffer.AddTruncate(line)
		// send content from lineBuffer
		h.sendContent()
		// truncate next content
		h.lineBuffer.AddTruncate(line)
	}
}

// sendContent forwards the content from lineBuffer to outputChan
func (h *MultiLineHandler) sendContent() {
	defer h.lineBuffer.Reset()
	content, rawDataLen := h.lineBuffer.Content()
	content = bytes.TrimSpace(content)
	if len(content) > 0 {
		output := NewOutput(content, rawDataLen)
		h.outputChan <- output
	}
}
