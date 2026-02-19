// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Pipeline owns all preprocessor stages and wires them in the correct order:
// JSON aggregation → tokenization → labeling → aggregation
type Pipeline struct {
	jsonAggregator        *JSONAggregator
	tokenizer             *Tokenizer
	labeler               *Labeler
	aggregator            Aggregator
	enableJSONAggregation bool
	flushTimeout          time.Duration
	flushTimer            *time.Timer
}

// NewPipeline creates a new Pipeline with stages wired in the correct order.
func NewPipeline(aggregator Aggregator, tokenizer *Tokenizer, labeler *Labeler,
	jsonAggregator *JSONAggregator, enableJSONAggregation bool, flushTimeout time.Duration) *Pipeline {
	return &Pipeline{
		jsonAggregator:        jsonAggregator,
		tokenizer:             tokenizer,
		labeler:               labeler,
		aggregator:            aggregator,
		enableJSONAggregation: enableJSONAggregation,
		flushTimeout:          flushTimeout,
	}
}

// Process processes a message through the pipeline.
func (p *Pipeline) Process(msg *message.Message) {
	p.stopFlushTimerIfNeeded()
	defer p.startFlushTimerIfNeeded()

	if p.enableJSONAggregation {
		for _, m := range p.jsonAggregator.Process(msg) {
			p.processOne(m)
		}
	} else {
		p.processOne(msg)
	}
}

func (p *Pipeline) processOne(msg *message.Message) {
	// Step 1: Tokenize the complete (possibly JSON-aggregated) message
	msg.ParsingExtra.Tokens, msg.ParsingExtra.TokenIndices = p.tokenizer.Tokenize(msg.GetContent())
	// Steps 2+3: Label and aggregate; label stays local and never touches the message
	label := p.labeler.Label(msg)
	p.aggregator.Process(msg, label)
}

// FlushChan returns a channel that signals when a flush should occur.
func (p *Pipeline) FlushChan() <-chan time.Time {
	if p.flushTimer != nil {
		return p.flushTimer.C
	}
	return nil
}

// Flush flushes all pipeline stages in order.
func (p *Pipeline) Flush() {
	if p.enableJSONAggregation {
		for _, m := range p.jsonAggregator.Flush() {
			p.processOne(m)
		}
	}
	p.aggregator.Flush()
	p.stopFlushTimerIfNeeded()
}

func (p *Pipeline) isEmpty() bool {
	return p.aggregator.IsEmpty() && p.jsonAggregator.IsEmpty()
}

func (p *Pipeline) stopFlushTimerIfNeeded() {
	if p.flushTimer == nil || p.isEmpty() {
		return
	}
	if !p.flushTimer.Stop() {
		<-p.flushTimer.C
	}
}

func (p *Pipeline) startFlushTimerIfNeeded() {
	if p.isEmpty() {
		return
	}
	if p.flushTimer == nil {
		p.flushTimer = time.NewTimer(p.flushTimeout)
	} else {
		p.flushTimer.Reset(p.flushTimeout)
	}
}
