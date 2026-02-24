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

// Preprocessor owns all preprocessor stages and wires them in the correct order:
// JSON aggregation (optional) → tokenization → labeling → aggregation → sampling
type Preprocessor struct {
	jsonAggregator        *JSONAggregator
	tokenizer             *Tokenizer
	labeler               *Labeler
	aggregator            Aggregator
	sampler               Sampler
	enableJSONAggregation bool
	flushTimeout          time.Duration
	flushTimer            *time.Timer
}

// NewPreprocessor creates a new Preprocessor.
// Pass nil for jsonAggregator when JSON aggregation is not needed.
// Pass nil for labeler when labeling is not needed (regex and pass-through paths).
func NewPreprocessor(aggregator Aggregator, tokenizer *Tokenizer, labeler *Labeler, sampler Sampler,
	jsonAggregator *JSONAggregator, enableJSONAggregation bool, flushTimeout time.Duration) *Preprocessor {
	return &Preprocessor{
		jsonAggregator:        jsonAggregator,
		tokenizer:             tokenizer,
		labeler:               labeler,
		aggregator:            aggregator,
		sampler:               sampler,
		enableJSONAggregation: enableJSONAggregation,
		flushTimeout:          flushTimeout,
	}
}

// Process processes a message through the preprocessor.
func (p *Preprocessor) Process(msg *message.Message) {
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

func (p *Preprocessor) processOne(msg *message.Message) {
	// Step 1: Tokenize and label — only when a labeler is present (auto multiline paths).
	// Tokens stay local; they are never stored on the message.
	var label Label
	if p.labeler != nil {
		var tokens []Token
		var tokenIndices []int
		tokens, tokenIndices = p.tokenizer.Tokenize(msg.GetContent())
		label = p.labeler.Label(msg.GetContent(), tokens, tokenIndices)
	}
	// Step 2: Aggregate (may buffer; returns zero or more completed messages)
	for _, completed := range p.aggregator.Process(msg, label) {
		// Step 3: Sample/emit
		p.sampler.Process(completed)
	}
}

// FlushChan returns a channel that signals when a flush should occur.
func (p *Preprocessor) FlushChan() <-chan time.Time {
	if p.flushTimer != nil {
		return p.flushTimer.C
	}
	return nil
}

// Flush flushes all preprocessor stages in order.
func (p *Preprocessor) Flush() {
	if p.enableJSONAggregation && p.jsonAggregator != nil {
		for _, m := range p.jsonAggregator.Flush() {
			p.processOne(m)
		}
	}
	for _, completed := range p.aggregator.Flush() {
		p.sampler.Process(completed)
	}
	p.sampler.Flush()
	p.stopFlushTimerIfNeeded()
}

func (p *Preprocessor) isEmpty() bool {
	jsonEmpty := p.jsonAggregator == nil || p.jsonAggregator.IsEmpty()
	return p.aggregator.IsEmpty() && jsonEmpty
}

func (p *Preprocessor) stopFlushTimerIfNeeded() {
	if p.flushTimer == nil || p.isEmpty() {
		return
	}
	if !p.flushTimer.Stop() {
		<-p.flushTimer.C
	}
}

func (p *Preprocessor) startFlushTimerIfNeeded() {
	if p.isEmpty() {
		return
	}
	if p.flushTimer == nil {
		p.flushTimer = time.NewTimer(p.flushTimeout)
	} else {
		p.flushTimer.Reset(p.flushTimeout)
	}
}
