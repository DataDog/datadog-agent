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
	outputChan      chan *message.Message
	jsonAggregator  JSONAggregator
	tokenizer       *Tokenizer
	labeler         Labeler
	aggregator      Aggregator
	sampler         Sampler
	flushTimeout    time.Duration
	flushTimer      *time.Timer
	labelerMaxBytes int // tokens beyond this byte offset are not passed to the labeler; 0 = no limit
}

// NewPreprocessor creates a new Preprocessor.
// Use NoopJSONAggregator for paths that don't aggregate JSON.
// Use NoopLabeler for paths that don't use auto multiline detection (regex, pass-through).
// labelerMaxBytes limits how many bytes of tokens the labeler sees; 0 means no limit (all tokens).
// This allows the tokenizer to produce a wider token window for the sampler while keeping the
// labeler focused on the log header it actually needs (e.g. timestamp detection).
func NewPreprocessor(aggregator Aggregator, tokenizer *Tokenizer, labeler Labeler, sampler Sampler,
	outputChan chan *message.Message, jsonAggregator JSONAggregator, flushTimeout time.Duration, labelerMaxBytes int) *Preprocessor {
	return &Preprocessor{
		outputChan:      outputChan,
		jsonAggregator:  jsonAggregator,
		tokenizer:       tokenizer,
		labeler:         labeler,
		aggregator:      aggregator,
		sampler:         sampler,
		flushTimeout:    flushTimeout,
		labelerMaxBytes: labelerMaxBytes,
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

// Steps 2, 3, and 4: tokenize, label, and aggregate each log.
// The tokenizer may produce tokens for more bytes than the labeler needs (e.g. when the
// adaptive sampler is active and has a wider window). limitTokensToBytes slices the token
// array so the labeler only sees tokens within its configured byte budget, while the
// aggregator (and sampler) receive the full token slice.
func (p *Preprocessor) tokenizeLabelAndAggregate(msg *message.Message) {
	tokens, tokenIndices := p.tokenizer.Tokenize(msg.GetContent())
	labelTokens, labelIndices := limitTokensToBytes(tokens, tokenIndices, p.labelerMaxBytes)
	label := p.labeler.Label(msg.GetContent(), labelTokens, labelIndices)
	for _, completed := range p.aggregator.Process(msg, label, tokens) {
		p.sample(completed)
	}
}

// limitTokensToBytes returns the prefix of tokens whose start byte index is less than maxBytes.
// If maxBytes is 0, all tokens are returned unchanged (no limit).
func limitTokensToBytes(tokens []Token, indices []int, maxBytes int) ([]Token, []int) {
	if maxBytes <= 0 {
		return tokens, indices
	}
	for i, idx := range indices {
		if idx >= maxBytes {
			return tokens[:i], indices[:i]
		}
	}
	return tokens, indices
}

// Step 5: Sample and emit the log
func (p *Preprocessor) sample(completed AggregatedMessageWithTokens) {
	if out := p.sampler.Process(completed.Msg, completed.Tokens); out != nil {
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
	for _, c := range p.aggregator.Flush() {
		p.sample(c)
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
