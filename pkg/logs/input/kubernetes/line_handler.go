/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-2019 Datadog, Inc.
 */

package kubernetes

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"time"
)

// LineHandler receive line as byte slice and merge lines.
// The inputChan receives:
// [timestamp stream flag message]
// T1 S1 P M1
// T2 S2 F M2
// The content send to nextRunner should be:
// T2 S2 F M1M2
// Normally, a line (T1 S1 P M1) should be less than 16K which is way less than contentLenLimit(256K).
type LineHandler struct {
	decoder.Handler
	nextRunner decoder.LineHandlerRunner
	prefix
	flushTimeout    time.Duration
	lineBuffer      *decoder.LineBuffer
	contentLenLimit int
	parser
}

const (
	fullMessage = "F"
)

type prefix struct {
	timestamp string
	status    string
	flag      string
}

func (p *prefix) formatBytes() []byte {
	return []byte(fmt.Sprintf("%v %v %v ", p.timestamp, p.status, p.flag))
}

// NewLineHandler returns a initialized LineHandler with given parameters.
func NewLineHandler(lineHandlerRunner decoder.LineHandlerRunner, flushTimeout time.Duration, contentLenLimit int) *LineHandler {
	return &LineHandler{
		Handler: decoder.Handler{
			InputChan: make(chan []byte),
		},
		nextRunner:      lineHandlerRunner,
		flushTimeout:    flushTimeout,
		lineBuffer:      decoder.NewLineBuffer(),
		contentLenLimit: contentLenLimit,
	}
}

// Handle send the content to input chan
func (h *LineHandler) Handle(content []byte) {
	h.InputChan <- content
}

// Stop close the inout chan and stops the next runner.
func (h *LineHandler) Stop() {
	close(h.InputChan)
	h.nextRunner.Stop()
}

// Start starts the nextRunner and start handler runner with timer.
func (h *LineHandler) Start() {
	h.nextRunner.Start()
	go h.RunWithTimer(h.flushTimeout, h.process, h.sendAndReset)
}

// process assumes line length does not exceed contentLenLimit, since the
// contentLenLimit normally is 256K and kubernetes log each line doesn't
// exceed 16K.
func (h *LineHandler) process(line []byte) {
	content, status, timestamp, flag, err := h.ParseFull(line)
	if err != nil { // this case should not happen with length limitation.
		log.Debug(err)
	} else {
		h.prefix = prefix{timestamp: timestamp, status: status, flag: flag}
	}

	content0, content1 := split(content, h.contentLenLimit-h.lineBuffer.Length())
	h.lineBuffer.AddIncompleteLine(content0)
	isFullContent := h.lineBuffer.Length() >= h.contentLenLimit
	if fullMessage == h.prefix.flag || isFullContent {
		h.sendAndReset()
	}

	if content1 != nil {
		h.lineBuffer.AddIncompleteLine(content1)
		h.sendAndReset()
	}
}

func split(content []byte, threshold int) ([]byte, []byte) {

	var limit = len(content)
	if limit <= threshold || threshold < 0 {
		return content[0:limit], nil
	}
	return content[0:threshold], content[threshold:]
}

func (h *LineHandler) sendAndReset() {
	defer h.lineBuffer.Reset()
	content, _ := h.lineBuffer.Content()
	if len(content) > 0 {
		content = append(h.prefix.formatBytes(), content...)
		h.nextRunner.Handle(content)
	}

}
