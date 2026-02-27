// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// Sampler is the final stage of the Preprocessor. It receives one completed log
// message and returns it unchanged or nil if the message should be dropped.
// tokens are the tokenized first line of the message, used to identify its pattern.
type Sampler interface {
	// Process handles a completed log message and returns it, or nil to drop it.
	Process(msg *message.Message, tokens []Token) *message.Message

	// Flush flushes any buffered state and returns a pending message, or nil if empty.
	Flush() *message.Message
}

// NoopSampler passes all messages through without modification.
// It is the default implementation used until adaptive sampling logic is added.
type NoopSampler struct{}

// NewNoopSampler returns a new NoopSampler.
func NewNoopSampler() *NoopSampler {
	return &NoopSampler{}
}

// Process returns the message unchanged.
func (s *NoopSampler) Process(msg *message.Message, _ []Token) *message.Message {
	return msg
}

// Flush is a no-op since NoopSampler has no buffered state.
func (s *NoopSampler) Flush() *message.Message {
	return nil
}

var tlmAdaptiveSamplerDropped = telemetry.NewCounter("logs_adaptive_sampler", "dropped",
	[]string{"source"}, "Number of log messages dropped by the adaptive sampler")

// AdaptiveSamplerConfig holds the configuration for the AdaptiveSampler.
type AdaptiveSamplerConfig struct {
	// MaxPatterns is the maximum number of distinct patterns tracked simultaneously.
	// When full, the least-frequently-matched pattern is evicted to make room for new ones.
	MaxPatterns int
	// RateLimit is the steady-state number of logs per second allowed per pattern.
	RateLimit float64
	// BurstSize is the maximum credits a pattern can accumulate. A new or returning
	// pattern can emit up to BurstSize logs before being rate-limited.
	BurstSize float64
	// MatchThreshold is the fraction of tokens that must match for two logs to be
	// considered the same pattern. Range [0, 1].
	MatchThreshold float64
}

// samplerEntry tracks the credit-based rate limiting state for a single log pattern.
// entries form a max-heap ordered by matchCount so the most frequently matched
// patterns appear early in the scan and are found with fewer comparisons.
type samplerEntry struct {
	tokens     []Token
	credits    float64   // remaining log allowance; decremented on each emitted log
	lastSeen   time.Time // used for credit refill
	matchCount int64     // total number of times this pattern has matched; drives heap order
}

// AdaptiveSampler rate-limits logs by structural pattern using per-pattern credit allowances.
// Structurally similar logs share a credit allowance.
// New or returning patterns receive a full burst allowance before being rate-limited.
//
// entries is maintained as a max-heap ordered by matchCount. This biases the linear
// scan toward high-frequency patterns, reducing average scan depth for skewed
// log streams (where a small number of patterns account for most volume).
type AdaptiveSampler struct {
	entries []samplerEntry
	config  AdaptiveSamplerConfig
	source  string // used as a telemetry tag
	now     func() time.Time
}

// NewAdaptiveSampler creates a new AdaptiveSampler.
// source is the log source name used for telemetry tagging.
func NewAdaptiveSampler(config AdaptiveSamplerConfig, source string) *AdaptiveSampler {
	return &AdaptiveSampler{
		entries: make([]samplerEntry, 0, config.MaxPatterns),
		config:  config,
		source:  source,
		now:     time.Now,
	}
}

// Process applies credit-based rate limiting to the message.
// Returns the message if allowed, nil if dropped.
func (s *AdaptiveSampler) Process(msg *message.Message, tokens []Token) *message.Message {
	now := s.now()

	for i := range s.entries {
		e := &s.entries[i]
		if !IsMatch(e.tokens, tokens, s.config.MatchThreshold) {
			continue
		}
		// Refill credits based on time elapsed since last seen.
		elapsed := now.Sub(e.lastSeen).Seconds()
		e.credits += elapsed * s.config.RateLimit
		if e.credits > s.config.BurstSize {
			e.credits = s.config.BurstSize
		}
		e.lastSeen = now
		e.matchCount++
		// siftUp is called after the credits check so that e (= &s.entries[i]) still
		// points to this entry: siftUp swaps entries by value, which would cause e to
		// alias a different entry if called before the allow/drop decision.
		if e.credits >= 1.0 {
			e.credits--
			s.siftUp(i)
			return msg
		}
		s.siftUp(i)
		// Rate limit exceeded — drop.
		tlmAdaptiveSamplerDropped.Inc(s.source)
		return nil
	}

	// No match — this is a new pattern. Evict the least-frequently-matched entry if full.
	if len(s.entries) >= s.config.MaxPatterns {
		s.evictLeastFrequent()
	}
	// Start with a full burst, consuming one token for the current message.
	s.entries = append(s.entries, samplerEntry{
		tokens:     tokens,
		credits:    s.config.BurstSize - 1,
		lastSeen:   now,
		matchCount: 1,
	})
	s.siftUp(len(s.entries) - 1)
	return msg
}

// Flush is a no-op — the adaptive sampler does not buffer messages.
func (s *AdaptiveSampler) Flush() *message.Message {
	return nil
}

// siftUp moves the entry at index i upward until the max-heap invariant is restored.
func (s *AdaptiveSampler) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if s.entries[parent].matchCount >= s.entries[i].matchCount {
			break
		}
		s.entries[parent], s.entries[i] = s.entries[i], s.entries[parent]
		i = parent
	}
}

// siftDown moves the entry at index i downward until the max-heap invariant is restored.
func (s *AdaptiveSampler) siftDown(i int) {
	n := len(s.entries)
	for {
		largest := i
		l, r := 2*i+1, 2*i+2
		if l < n && s.entries[l].matchCount > s.entries[largest].matchCount {
			largest = l
		}
		if r < n && s.entries[r].matchCount > s.entries[largest].matchCount {
			largest = r
		}
		if largest == i {
			break
		}
		s.entries[i], s.entries[largest] = s.entries[largest], s.entries[i]
		i = largest
	}
}

// heapify rebuilds the heap in-place using Floyd's algorithm — O(n).
// Call this after bulk-loading entries directly into the slice.
func (s *AdaptiveSampler) heapify() {
	for i := len(s.entries)/2 - 1; i >= 0; i-- {
		s.siftDown(i)
	}
}

// removeAt removes the entry at index i and restores the heap invariant — O(log n).
func (s *AdaptiveSampler) removeAt(i int) {
	last := len(s.entries) - 1
	if i == last {
		s.entries = s.entries[:last]
		return
	}
	s.entries[i] = s.entries[last]
	s.entries = s.entries[:last]
	// The replacement could be larger (needs sift-up) or smaller (needs sift-down).
	// siftDown is a no-op if the replacement is already >= its children;
	// siftUp is a no-op if the replacement is already <= its parent.
	s.siftDown(i)
	s.siftUp(i)
}

// evictLeastFrequent removes the entry with the lowest matchCount.
// In a max-heap the minimum is always a leaf node (indices n/2..n-1), so only
// the leaf range needs to be scanned — roughly half the entries.
func (s *AdaptiveSampler) evictLeastFrequent() {
	n := len(s.entries)
	if n == 0 {
		return
	}
	leafStart := n / 2
	minIdx := leafStart
	for i := leafStart + 1; i < n; i++ {
		if s.entries[i].matchCount < s.entries[minIdx].matchCount {
			minIdx = i
		}
	}
	s.removeAt(minIdx)
}
