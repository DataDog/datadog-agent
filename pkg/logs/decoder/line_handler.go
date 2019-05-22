// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TRUNCATED is the warning we add at the beginning or/and at the end of a truncated message
var TRUNCATED = []byte("...TRUNCATED...")

// LineHandlerRunner handles byte slices to form line output
type LineHandlerRunner interface {
	Handle(content []byte)
	Start()
	Stop()
}

// Handler defines the input and output chan for handling a line.
type Handler struct {
	InputChan  chan []byte
	OutputChan chan *Output
}

// Run takes a function for handling the message from input chan.
func (h *Handler) Run(process func([]byte)) {
	for line := range h.InputChan {
		process(line)
	}
	close(h.OutputChan)
}

// RunWithTimer takes a timer, a function for handling the message and a function for sending the transformed message.
func (h *Handler) RunWithTimer(timeout time.Duration, process func([]byte), sendContent func()) {
	flushTimer := time.NewTimer(timeout)
	defer func() {
		flushTimer.Stop()
		if h.OutputChan != nil {
			close(h.OutputChan)
		}
	}()
	for {
		select {
		case line, isOpen := <-h.InputChan:
			if !isOpen {
				// lineChan has been closed, no more lines are expected
				return
			}
			// process the new line and restart the timeout
			if !flushTimer.Stop() {
				// stop doesn't not prevent a tick from the Timer if it happens at the same time
				// we read from the timer channel to prevent an incorrect read
				// in <-flushTimer.C in the case below
				select {
				case <-flushTimer.C:
				default:
				}
			}
			process(line)
			flushTimer.Reset(timeout)
		case <-flushTimer.C:
			// the timout expired, the content is ready to be sent
			sendContent()
		}
	}
}

// NewLineHandlerRunner returns appropriate LineHandler according to the source.
func NewLineHandlerRunner(outputChan chan *Output, parser parser.Parser, source *config.LogSource, contentLenLimit int) LineHandlerRunner {
	var lineHandlerRunner LineHandlerRunner
	for _, rule := range source.Config.ProcessingRules {
		if rule.Type == config.MultiLine {
			lineHandlerRunner = NewMultiLineHandler(outputChan, rule.Regex, DefaultFlushTimeout, parser, contentLenLimit)
		}
	}
	if lineHandlerRunner == nil {
		lineHandlerRunner = NewSingleLineHandler(outputChan, parser, contentLenLimit)
	}
	return lineHandlerRunner
}

// SingleLineHandler creates and forward outputs to outputChan from single-lines
type SingleLineHandler struct {
	Handler
	lineLimit      int
	shouldTruncate bool
	parser.Parser
}

// NewSingleLineHandler returns a new SingleLineHandler
func NewSingleLineHandler(outputChan chan *Output, parser parser.Parser, lineLimit int) *SingleLineHandler {
	return &SingleLineHandler{
		Handler:   Handler{InputChan: make(chan []byte), OutputChan: outputChan},
		lineLimit: lineLimit,
		Parser:    parser,
	}
}

// Handle trims leading and trailing whitespaces from content,
// and sends it as a new Line to lineChan.
func (h *SingleLineHandler) Handle(content []byte) {
	h.InputChan <- content
}

// Stop stops the handler from processing new lines
func (h *SingleLineHandler) Stop() {
	close(h.InputChan)
}

// Start starts the handler
func (h *SingleLineHandler) Start() {
	go h.Run(h.process)
}

// process creates outputs from lines and forwards them to outputChan
// When the line length exceed (include equals) line length limit,
// "...TRUNCATED..." flag will be appended.
func (h *SingleLineHandler) process(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	content, status, timestamp, err := h.Parse(line)
	lineLen := len(content)
	if err != nil {
		log.Debug(err)
	}

	if h.shouldTruncate {
		content = append(TRUNCATED, content...)
		h.shouldTruncate = false
	}

	if lineLen < h.lineLimit {
		if len(content) > 0 {
			h.OutputChan <- NewOutput(content, status, lineLen+1, timestamp)
		}
	} else {
		content = append(content, TRUNCATED...)
		h.OutputChan <- NewOutput(content, status, lineLen, timestamp)
		h.shouldTruncate = true
	}
}

// DefaultFlushTimeout represents the time we want to wait before flushing lineBuffer
// when no more line is received
const DefaultFlushTimeout = 1000 * time.Millisecond

// MultiLineHandler reads lines from lineChan and uses lineBuffer to send them
// when a new line matches with re or flushTimer is fired
type MultiLineHandler struct {
	Handler
	lineBuffer        *LineBuffer
	lastSeenTimestamp string
	newContentRe      *regexp.Regexp
	flushTimeout      time.Duration
	lineLimit         int
	parser.Parser
}

// NewMultiLineHandler returns a new MultiLineHandler
func NewMultiLineHandler(outputChan chan *Output, newContentRe *regexp.Regexp, flushTimeout time.Duration, parser parser.Parser, lineLimit int) *MultiLineHandler {
	return &MultiLineHandler{
		Handler:      Handler{InputChan: make(chan []byte), OutputChan: outputChan},
		lineBuffer:   NewLineBuffer(),
		newContentRe: newContentRe,
		flushTimeout: flushTimeout,
		lineLimit:    lineLimit,
		Parser:       parser,
	}
}

// Handle forward lines to lineChan to process them
func (h *MultiLineHandler) Handle(content []byte) {
	h.InputChan <- content
}

// Stop stops the lineHandler from processing lines
func (h *MultiLineHandler) Stop() {
	close(h.InputChan)
}

// Start starts the handler
func (h *MultiLineHandler) Start() {
	go h.RunWithTimer(h.flushTimeout, h.process, h.sendContent)
}

// process accumulates lines in lineBuffer and flushes lineBuffer when a new line matches with newContentRe
// When lines are too long, they are truncated
func (h *MultiLineHandler) process(line []byte) {
	unwrappedLine, _, timestamp, err := h.Parse(line)
	if timestamp != "" {
		h.lastSeenTimestamp = timestamp
	}

	if err != nil {
		log.Debug(err)
	}
	if h.newContentRe.Match(unwrappedLine) {
		// send content from lineBuffer
		h.sendContent()
	}
	line = unwrappedLine
	if !h.lineBuffer.IsEmpty() {
		// add '\n' to content in lineBuffer
		h.lineBuffer.AddEndOfLine()
	}

	// NOTES: this check takes into account the length of "...TRUNCATED..."
	// which in some scenario is outputting an ending message with
	// ...TRUNCATED... as only content, see unit test line_handler_test.go/TestMultiLineHandler
	if len(line)+h.lineBuffer.Length() < h.lineLimit {
		// add line to content in lineBuffer
		h.lineBuffer.Add(line)
	} else {
		// add line and truncate and flush content in lineBuffer
		h.lineBuffer.AddIncompleteLine(line)
		h.lineBuffer.AddTruncate()
		// send content from lineBuffer
		h.sendContent()
		// truncate next content
		h.lineBuffer.AddTruncate()
	}
}

// sendContent forwards the content from lineBuffer to outputChan
func (h *MultiLineHandler) sendContent() {
	defer h.lineBuffer.Reset()
	content, rawDataLen := h.lineBuffer.Content()
	content = bytes.TrimSpace(content)
	if len(content) > 0 {
		// The output.Timestamp filled by the Parse function is the ts of the first
		// log line, in order to be useful to setLastSince function, we need to replace
		// it with the ts of the last log line. Note: this timestamp is NOT used for stamp
		// the log record, it's ONLY used to recover well when tailing back the container.
		h.OutputChan <- NewOutput(content, message.StatusInfo, rawDataLen, h.lastSeenTimestamp)
	}
}
