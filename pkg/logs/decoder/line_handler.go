// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	lineLen := len(line)
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}

	var content []byte
	if h.shouldTruncate {
		// add TRUNCATED at the beginning of content
		content = append(truncatedFlag, line...)
		h.shouldTruncate = false
	} else {
		// keep content the same
		content = line
	}

	if lineLen < h.lineLimit {
		// send content
		// add 1 to take into account '\n' that we didn't include in content
		output, status, timestamp, err := h.parser.Parse(content)
		if err != nil {
			log.Debug(err)
		}
		if len(output) > 0 {
			h.outputChan <- NewOutput(output, status, lineLen+1, timestamp)
		}
	} else {
		// add TRUNCATED at the end of content and send it
		content := append(content, truncatedFlag...)
		output, status, timestamp, err := h.parser.Parse(content)
		if err != nil {
			log.Debug(err)
		}
		if len(output) > 0 {
			h.outputChan <- NewOutput(output, status, lineLen, timestamp)
			h.shouldTruncate = true
		}
	}
}

// defaultFlushTimeout represents the time after which a multiline
// will be be considered as complete.
const defaultFlushTimeout = 1000 * time.Millisecond

// MultiLineHandler makes sure that multiple lines from a same content
// are properly put together.
type MultiLineHandler struct {
	lineChan          chan []byte
	outputChan        chan *Output
	lineBuffer        *LineBuffer
	lastSeenTimestamp string
	newContentRe      *regexp.Regexp
	flushTimeout      time.Duration
	parser            parser.Parser
	lineLimit         int
}

// NewMultiLineHandler returns a new MultiLineHandler.
func NewMultiLineHandler(outputChan chan *Output, newContentRe *regexp.Regexp, flushTimeout time.Duration, parser parser.Parser, lineLimit int) *MultiLineHandler {
	return &MultiLineHandler{
		lineChan:     make(chan []byte),
		outputChan:   outputChan,
		lineBuffer:   NewLineBuffer(),
		newContentRe: newContentRe,
		flushTimeout: flushTimeout,
		parser:       parser,
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

// run processes new lines from lineChan and flushes the buffer when the timeout expires
func (h *MultiLineHandler) run() {
	flushTimer := time.NewTimer(h.flushTimeout)
	defer func() {
		flushTimer.Stop()
		// last send before closing the channel to flush lineBuffer. This can occur when
		// container stops before meeting sendContent condition.
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
				// stop doesn't not prevent a tick from the Timer if it happens at the same time
				// we read from the timer channel to prevent an incorrect read
				// in <-flushTimer.C in the case below
				select {
				case <-flushTimer.C:
				default:
				}
			}
			h.process(line)
			flushTimer.Reset(h.flushTimeout)
		case <-flushTimer.C:
			// the timout expired, the content is ready to be sent
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
	unwrappedLine, _, timestamp, err := h.parser.Parse(line)
	h.lastSeenTimestamp = timestamp
	if err != nil {
		log.Debug(err)
	}
	if h.newContentRe.Match(unwrappedLine) {
		// send content from lineBuffer
		h.sendBuffer()
	}
	if !h.lineBuffer.IsEmpty() {
		// unwrap all the following lines
		line = unwrappedLine
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
		h.lineBuffer.AddTruncate(line)
		// send content from lineBuffer
		h.sendBuffer()
		// truncate next content
		h.lineBuffer.AddTruncate(line)
	}
}

// sendBuffer forwards the content stored in the buffer
// to the output channel.
func (h *MultiLineHandler) sendBuffer() {
	defer h.lineBuffer.Reset()
	content, rawDataLen := h.lineBuffer.Content()
	content = bytes.TrimSpace(content)
	if len(content) > 0 {
		output, status, _, err := h.parser.Parse(content)
		if err != nil {
			log.Debug(err)
		}
		if len(output) > 0 {
			// The output.Timestamp filled by the Parse function is the ts of the first
			// log line, in order to be useful to setLastSince function, we need to replace
			// it with the ts of the last log line. Note: this timestamp is NOT used for stamp
			// the log record, it's ONLY used to recover well when tailing back the container.
			h.outputChan <- NewOutput(output, status, rawDataLen, h.lastSeenTimestamp)
		}
	}
}
