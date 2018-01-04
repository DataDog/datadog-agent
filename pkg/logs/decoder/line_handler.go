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

// Line represents content separated by two '\n'
type Line struct {
	content []byte
}

// newLine returns a new Line
func newLine(content []byte) *Line {
	return &Line{
		content: content,
	}
}

// LineHandler handles byte slices to form line output
type LineHandler interface {
	Handle(content []byte)
	Stop()
}

// SingleLineHandler creates and forward outputs to outputChan from single-lines
type SingleLineHandler struct {
	lineChan       chan *Line
	outputChan     chan *Output
	shouldTruncate bool
}

// NewSingleLineHandler returns a new SingleLineHandler
func NewSingleLineHandler(outputChan chan *Output) *SingleLineHandler {
	lineChan := make(chan *Line)
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
	lh.lineChan <- newLine(bytes.TrimSpace(content))
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
func (lh *SingleLineHandler) process(line *Line) {
	lineLen := len(line.content)
	if lineLen == 0 {
		return
	}

	var content []byte
	if lh.shouldTruncate {
		// add TRUNCATED at the beginning of content
		content = append(TRUNCATED, line.content...)
		lh.shouldTruncate = false
	} else {
		// keep content the same
		content = line.content
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

// MultiLineLineHandler reads lines from lineChan and uses lineBuffer to send them
// when a new line matches with re or flushTimer is fired
type MultiLineLineHandler struct {
	lineChan     chan *Line
	lineBuffer   *LineBuffer
	newContentRe *regexp.Regexp
	flushTimer   *time.Timer
	mu           sync.Mutex
	shouldStop   bool
}

// NewMultiLineLineHandler returns a new MultiLineLineHandler
func NewMultiLineLineHandler(outputChan chan *Output, newContentRe *regexp.Regexp) *MultiLineLineHandler {
	lineChan := make(chan *Line)
	lineBuffer := NewLineBuffer(outputChan)
	flushTimer := time.NewTimer(flushTimeout)
	lineHandler := MultiLineLineHandler{
		lineChan:     lineChan,
		lineBuffer:   lineBuffer,
		newContentRe: newContentRe,
		flushTimer:   flushTimer,
	}
	go lineHandler.start()
	return &lineHandler
}

// Handle forward lines to lineChan to process them
func (lh *MultiLineLineHandler) Handle(content []byte) {
	lh.lineChan <- newLine(content)
}

// Stop stops the lineHandler from processing lines
func (lh *MultiLineLineHandler) Stop() {
	lh.mu.Lock()
	close(lh.lineChan)
	lh.shouldStop = true
	// assure to stop timer goroutine
	lh.flushTimer.Reset(flushTimeout)
	lh.mu.Unlock()
}

// start starts the delimiter
func (lh *MultiLineLineHandler) start() {
	go lh.handleExpiration()
	go lh.run()
}

// run reads lines from lineChan to process them
func (lh *MultiLineLineHandler) run() {
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
func (lh *MultiLineLineHandler) handleExpiration() {
	for range lh.flushTimer.C {
		lh.mu.Lock()
		lh.lineBuffer.Flush()
		lh.mu.Unlock()
		if lh.shouldStop {
			break
		}
	}
	lh.lineBuffer.Stop()
}

// process accumulates lines in lineBuffer and flushes lineBuffer when a new line matches with newContentRe
// When lines are too long, they are truncated
func (lh *MultiLineLineHandler) process(line *Line) {
	if lh.newContentRe.Match(line.content) {
		// send content in lineBuffer
		lh.lineBuffer.Flush()
	}
	if !lh.lineBuffer.IsEmpty() {
		// add '\n' to content in lineBuffer
		lh.lineBuffer.AddEndOfLine()
	}
	if len(line.content)+lh.lineBuffer.Length() < contentLenLimit {
		// add line to content in lineBuffer
		lh.lineBuffer.Add(line)
	} else {
		// add line and truncate and flush content in lineBuffer
		lh.lineBuffer.AddIncompleteLine(line)
		lh.lineBuffer.AddTruncate(line)
		lh.lineBuffer.Flush()
		// truncate next content
		lh.lineBuffer.AddTruncate(line)
	}
}
