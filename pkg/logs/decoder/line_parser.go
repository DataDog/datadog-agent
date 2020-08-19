// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package decoder

import (
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
	p.lineHandler.Stop()
}

// run consumes new lines and processes them.
func (p *SingleLineParser) run() {
	for input := range p.inputChan {
		p.process(input)
	}
}

func (p *SingleLineParser) process(input *DecodedInput) {
	// Just parse an pass to the next step
	content, status, timestamp, _, err := p.parser.Parse(input.content)
	if err != nil {
		log.Debug(err)
	}
	p.lineHandler.Handle(NewMessage(content, status, input.rawDataLen, timestamp))
}
