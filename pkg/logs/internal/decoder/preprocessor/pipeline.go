// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Pipeline owns all preprocessor stages and wires them in the correct order:
// JSON aggregation (optional) → tokenization → combining → sampling
type Pipeline struct {
	jsonAggregator        *JSONAggregator
	tokenizer             *Tokenizer
	combiner              Combiner
	sampler               Sampler
	enableJSONAggregation bool
	flushTimeout          time.Duration
	flushTimer            *time.Timer
}

// NewPipeline creates a new Pipeline.
// Pass nil for jsonAggregator when JSON aggregation is not needed.
func NewPipeline(combiner Combiner, tokenizer *Tokenizer, sampler Sampler,
	jsonAggregator *JSONAggregator, enableJSONAggregation bool, flushTimeout time.Duration) *Pipeline {
	return &Pipeline{
		jsonAggregator:        jsonAggregator,
		tokenizer:             tokenizer,
		combiner:              combiner,
		sampler:               sampler,
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
	// Step 2: Combine (may buffer; returns zero or more completed messages)
	for _, completed := range p.combiner.Process(msg) {
		// Step 3: Sample/emit
		p.sampler.Process(completed)
	}
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
	if p.enableJSONAggregation && p.jsonAggregator != nil {
		for _, m := range p.jsonAggregator.Flush() {
			p.processOne(m)
		}
	}
	for _, completed := range p.combiner.Flush() {
		p.sampler.Process(completed)
	}
	p.sampler.Flush()
	p.stopFlushTimerIfNeeded()
}

func (p *Pipeline) isEmpty() bool {
	jsonEmpty := p.jsonAggregator == nil || p.jsonAggregator.IsEmpty()
	return p.combiner.IsEmpty() && jsonEmpty
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
