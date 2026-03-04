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
// JSON aggregation → tokenization → labeling → aggregation → sampling → outputChan
type Preprocessor struct {
	outputChan     chan *message.Message
	jsonAggregator JSONAggregator
	tokenizer      *Tokenizer
	labeler        Labeler
	aggregator     Aggregator
	sampler        Sampler
	flushTimeout   time.Duration
	flushTimer     *time.Timer
}

// NewPreprocessor creates a new Preprocessor.
// Use NoopJSONAggregator for paths that don't aggregate JSON.
// Use NoopLabeler for paths that don't use auto multiline detection (regex, pass-through).
func NewPreprocessor(aggregator Aggregator, tokenizer *Tokenizer, labeler Labeler, sampler Sampler,
	outputChan chan *message.Message, jsonAggregator JSONAggregator, flushTimeout time.Duration) *Preprocessor {
	return &Preprocessor{
		outputChan:     outputChan,
		jsonAggregator: jsonAggregator,
		tokenizer:      tokenizer,
		labeler:        labeler,
		aggregator:     aggregator,
		sampler:        sampler,
		flushTimeout:   flushTimeout,
	}
}

// Process processes a message through all preprocessor stages in order.
// Step 1: Aggregate JSON logs
func (p *Preprocessor) Process(msg *message.Message) {
	p.stopFlushTimerIfNeeded()
	defer p.startFlushTimerIfNeeded()

	for _, m := range p.jsonAggregator.Process(msg) {
		p.tokenizeLabelAndAggregate(m)
	}
}

// Steps 2, 3, and 4: tokenize, label, and aggregate each log
func (p *Preprocessor) tokenizeLabelAndAggregate(msg *message.Message) {
	tokens, tokenIndices := p.tokenizer.Tokenize(msg.GetContent())
	label := p.labeler.Label(msg.GetContent(), tokens, tokenIndices)
	for _, completed := range p.aggregator.Process(msg, label) {
		p.sample(completed)
	}
}

// Step 5: Sample and emit the log
func (p *Preprocessor) sample(msg *message.Message) {
	if out := p.sampler.Process(msg); out != nil {
		p.outputChan <- out
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
	for _, m := range p.jsonAggregator.Flush() {
		p.tokenizeLabelAndAggregate(m)
	}
	for _, completed := range p.aggregator.Flush() {
		p.sample(completed)
	}
	if out := p.sampler.Flush(); out != nil {
		p.outputChan <- out
	}
	p.stopFlushTimerIfNeeded()
}

func (p *Preprocessor) isEmpty() bool {
	return p.aggregator.IsEmpty() && p.jsonAggregator.IsEmpty()
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
