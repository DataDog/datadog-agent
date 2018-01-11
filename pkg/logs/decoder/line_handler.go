// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"sync"
	"time"
)

// TRUNCATED is the warning we add at the beginning or/and at the end of a truncated message
var TRUNCATED = []byte("...TRUNCATED...")

// LineHandler handles byte slices to form line output
type LineHandler interface {
	Handle(content []byte)
	Stop()
}

// SingleLineHandler creates and forward outputs to outputChan from single-lines
type SingleLineHandler struct {
	lineChan       chan []byte
	outputChan     chan *Output
	shouldTruncate bool
}

// NewSingleLineHandler returns a new SingleLineHandler
func NewSingleLineHandler(outputChan chan *Output) *SingleLineHandler {
	lineChan := make(chan []byte)
	lineHandler := SingleLineHandler{
		lineChan:   lineChan,
		outputChan: outputChan,
	}
	go lineHandler.start()
	return &lineHandler
}

// Handle trims leading and trailing whitespaces from content,
// and sends it as a new Line to lineChan.
func (lh *SingleLineHandler) Handle(content []byte) {
	lh.lineChan <- bytes.TrimSpace(content)
}

// Stop stops the handler from processing new lines
func (lh *SingleLineHandler) Stop() {
	close(lh.lineChan)
}

// start consumes lines from lineChan to process them
func (lh *SingleLineHandler) start() {
	for line := range lh.lineChan {
		lh.process(line)
	}
	lh.outputChan <- newStopOutput()
}

// process creates outputs from lines and forwards them to outputChan
// When lines are too long, they are truncated
func (lh *SingleLineHandler) process(line []byte) {
	lineLen := len(line)
	if lineLen == 0 {
		return
	}

	var content []byte
	if lh.shouldTruncate {
		// add TRUNCATED at the beginning of content
		content = append(TRUNCATED, line...)
		lh.shouldTruncate = false
	} else {
		// keep content the same
		content = line
	}

	if lineLen < contentLenLimit {
		// send content
		output := NewOutput(content, lineLen+1) // add 1 to take into account '\n'
		lh.outputChan <- output
	} else {
		// add TRUNCATED at the end of content and send it
		content := append(content, TRUNCATED...)
		output := NewOutput(content, lineLen)
		lh.outputChan <- output
		lh.shouldTruncate = true
	}
}

// flushTimeout represents the time we want to wait before flushing lineBuffer
// when no more line is received
const flushTimeout = 1 * time.Second

// MultiLineHandler reads lines from lineChan and uses lineBuffer to send them
// when a new line matches with re or flushTimer is fired
type MultiLineHandler struct {
	lineChan      chan []byte
	outputChan    chan *Output
	lineBuffer    *LineBuffer
	lineUnwrapper LineUnwrapper
	newContentRe  *regexp.Regexp
	flushTimer    *time.Timer
	mu            sync.Mutex
	shouldStop    bool
}

// NewMultiLineHandler returns a new MultiLineHandler
func NewMultiLineHandler(outputChan chan *Output, newContentRe *regexp.Regexp, lineUnwrapper LineUnwrapper) *MultiLineHandler {
	lineChan := make(chan []byte)
	lineBuffer := NewLineBuffer()
	flushTimer := time.NewTimer(flushTimeout)
	lineHandler := MultiLineHandler{
		lineChan:      lineChan,
		outputChan:    outputChan,
		lineBuffer:    lineBuffer,
		lineUnwrapper: lineUnwrapper,
		newContentRe:  newContentRe,
		flushTimer:    flushTimer,
	}
	go lineHandler.start()
	return &lineHandler
}

// Handle forward lines to lineChan to process them
func (lh *MultiLineHandler) Handle(content []byte) {
	lh.lineChan <- content
}

// Stop stops the lineHandler from processing lines
func (lh *MultiLineHandler) Stop() {
	lh.mu.Lock()
	close(lh.lineChan)
	lh.shouldStop = true
	// assure to stop timer goroutine
	lh.flushTimer.Reset(flushTimeout)
	lh.mu.Unlock()
}

// start starts the delimiter
func (lh *MultiLineHandler) start() {
	go lh.handleExpiration()
	go lh.run()
}

// run reads lines from lineChan to process them
func (lh *MultiLineHandler) run() {
	// read and process lines safely
	for line := range lh.lineChan {
		lh.mu.Lock()
		// prevent timer from firing
		lh.flushTimer.Stop()
		lh.process(line)
		// restart timer if no more lines are received
		lh.flushTimer.Reset(flushTimeout)
		lh.mu.Unlock()
	}
}

// handleExpiration flushes content in lineBuffer when flushTimer expires
func (lh *MultiLineHandler) handleExpiration() {
	for range lh.flushTimer.C {
		lh.mu.Lock()
		lh.sendContent()
		lh.mu.Unlock()
		if lh.shouldStop {
			break
		}
	}
	lh.outputChan <- newStopOutput()
}

// process accumulates lines in lineBuffer and flushes lineBuffer when a new line matches with newContentRe
// When lines are too long, they are truncated
func (lh *MultiLineHandler) process(line []byte) {
	unwrappedLine := lh.lineUnwrapper.Unwrap(line)
	if lh.newContentRe.Match(unwrappedLine) {
		// send content from lineBuffer
		lh.sendContent()
	}
	if !lh.lineBuffer.IsEmpty() {
		// unwrap all the following lines
		line = unwrappedLine
		// add '\n' to content in lineBuffer
		lh.lineBuffer.AddEndOfLine()
	}
	if len(line)+lh.lineBuffer.Length() < contentLenLimit {
		// add line to content in lineBuffer
		lh.lineBuffer.Add(line)
	} else {
		// add line and truncate and flush content in lineBuffer
		lh.lineBuffer.AddIncompleteLine(line)
		lh.lineBuffer.AddTruncate(line)
		// send content from lineBuffer
		lh.sendContent()
		// truncate next content
		lh.lineBuffer.AddTruncate(line)
	}
}

// sendContent forwards the content from lineBuffer to outputChan
func (lh *MultiLineHandler) sendContent() {
	defer lh.lineBuffer.Reset()
	content, rawDataLen := lh.lineBuffer.Content()
	content = bytes.TrimSpace(content)
	if len(content) > 0 {
		output := NewOutput(content, rawDataLen)
		lh.outputChan <- output
	}
}
