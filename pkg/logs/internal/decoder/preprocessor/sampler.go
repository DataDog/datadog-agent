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
type samplerEntry struct {
	tokens     []Token
	credits    float64   // remaining log allowance; decremented on each emitted log
	lastSeen   time.Time // used for credit refill
	matchCount int64     // total number of times this pattern has matched; drives sort order
}

// AdaptiveSampler rate-limits logs by structural pattern using per-pattern credit allowances.
// Structurally similar logs share a credit allowance.
// New or returning patterns receive a full burst allowance before being rate-limited.
//
// entries is maintained as a sorted list in descending matchCount order. The most
// frequently matched patterns appear at the front, so the scan exits early for the
// common case where a hot pattern is matched.
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

		// Determine outcome before bubbling: bubbling swaps entries by value, so
		// e (= &s.entries[i]) would alias a different entry after the first swap.
		allow := e.credits >= 1.0
		if allow {
			e.credits--
		}

		// Bubble the matched entry toward the front to maintain descending order.
		for i > 0 && s.entries[i-1].matchCount < s.entries[i].matchCount {
			s.entries[i-1], s.entries[i] = s.entries[i], s.entries[i-1]
			i--
		}

		if allow {
			return msg
		}
		tlmAdaptiveSamplerDropped.Inc(s.source)
		return nil
	}

	// No match — this is a new pattern. Evict the least-frequently-matched entry if full.
	if len(s.entries) >= s.config.MaxPatterns {
		s.entries = s.entries[:len(s.entries)-1]
	}
	// New patterns start with matchCount=1 and belong at the end of the sorted list.
	s.entries = append(s.entries, samplerEntry{
		tokens:     tokens,
		credits:    s.config.BurstSize - 1,
		lastSeen:   now,
		matchCount: 1,
	})
	return msg
}

// Flush is a no-op — the adaptive sampler does not buffer messages.
func (s *AdaptiveSampler) Flush() *message.Message {
	return nil
}
