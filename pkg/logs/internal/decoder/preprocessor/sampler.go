// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"hash/fnv"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
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

var tlmAdaptiveSamplerDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "dropped",
	[]string{"source", "detection_only"}, "Number of log messages dropped by the adaptive sampler, or that would be dropped when detection_only is true")

var tlmAdaptiveSamplerBytesDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "bytes_dropped",
	[]string{"source", "detection_only"}, "Number of bytes dropped by the adaptive sampler, or that would be dropped when detection_only is true")

var tlmAdaptiveSamplerKept = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "kept",
	[]string{"source"}, "Number of log messages emitted by the adaptive sampler")

var tlmAdaptiveSamplerNewPatterns = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "new_patterns",
	[]string{"source"}, "Number of new log patterns added to the adaptive sampler pattern table")

var tlmAdaptiveSamplerEvictions = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "evictions",
	[]string{"source"}, "Number of pattern table evictions performed by the adaptive sampler")

var tlmAdaptiveSamplerProtected = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "protected",
	[]string{"source"}, "Number of important log messages that bypassed adaptive sampling")

var tlmAdaptiveSamplerTagBytesDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "tag_bytes_dropped",
	[]string{"source", "detection_only"}, "Estimated pre-tailer tag metadata bytes for logs dropped by the adaptive sampler, or that would be dropped when detection_only is true")

func adaptiveSamplerSampledCountTag(count int64) string {
	return "adaptive_sampler_sampled_count:" + strconv.FormatInt(count, 10)
}

const adaptiveSamplerNoisyLogTag = "noisy_log:true"

func adaptiveSamplerLogHash(tokens []Token) string {
	var b [1]byte
	h := fnv.New64a()
	for _, t := range tokens {
		b[0] = byte(t)
		_, _ = h.Write(b[:])
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

func adaptiveSamplerLogHashTag(tokens []Token) string {
	return "log_hash:" + adaptiveSamplerLogHash(tokens)
}

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
	// ProtectImportantLogs bypasses rate limiting for logs containing critical severity
	// keywords (FATAL, ERROR, PANIC, etc.). Protected logs are never dropped.
	ProtectImportantLogs bool
	// DetectionOnly tags messages that would be dropped, without dropping them.
	DetectionOnly bool
	// TagPatternHash tags messages with the hash of their structural pattern.
	TagPatternHash bool
	// Include limits adaptive sampling to messages matching at least one filter.
	// When empty, all messages are eligible unless excluded.
	Include []AdaptiveSamplerFilter
	// IncludeConfigured records whether Include was explicitly configured, so an
	// invalid or empty include list does not accidentally sample every message.
	IncludeConfigured bool
	// Exclude makes matching messages bypass adaptive sampling. Exclude takes
	// precedence over Include.
	Exclude []AdaptiveSamplerFilter
}

// AdaptiveSamplerFilter matches messages by raw-content regex, structural sample,
// or both.
type AdaptiveSamplerFilter struct {
	Regex        *regexp.Regexp
	SampleTokens []Token
}

// samplerEntry tracks the credit-based rate limiting state for a single log pattern.
type samplerEntry struct {
	tokens         []Token
	credits        float64   // remaining log allowance; decremented on each emitted log
	lastSeen       time.Time // used for credit refill
	matchCount     int64     // total number of times this pattern has matched; drives sort order
	sampled        int64     // number of dropped matches since the last emitted log
	appliedBoostID uint64    // latest boost whose one-time credit grant was applied
}

type adaptiveSamplerLimits struct {
	rateLimit float64
	burstSize float64
	boost     adaptivesampling.SamplingBoost
	boosted   bool
}

// AdaptiveSampler rate-limits logs by structural pattern using per-pattern credit allowances.
// Structurally similar logs share a credit allowance.
// New or returning patterns receive a full burst allowance before being rate-limited.
//
// entries is maintained as a sorted list in descending matchCount order. The most
// frequently matched patterns appear at the front, so the scan exits early for the
// common case where a hot pattern is matched.
type AdaptiveSampler struct {
	entries           []samplerEntry
	config            AdaptiveSamplerConfig
	source            string // used as a telemetry tag
	now               func() time.Time
	baseBytesEstimate int
	boostLookupLogs   atomic.Uint64
	boostMissLogs     atomic.Uint64
	boostOutcomeLogs  atomic.Uint64
}

// NewAdaptiveSampler creates a new AdaptiveSampler.
// source is the log source name used for telemetry tagging.
// baseBytesEstimate is the static portion of the ddtags byte count (source config
// tags + sourcecategory), computed once at decoder construction time.
func NewAdaptiveSampler(config AdaptiveSamplerConfig, source string, baseBytesEstimate int) *AdaptiveSampler {
	return &AdaptiveSampler{
		entries:           make([]samplerEntry, 0, config.MaxPatterns),
		config:            config,
		source:            source,
		now:               time.Now,
		baseBytesEstimate: baseBytesEstimate,
	}
}

// isImportant reports whether the token sequence contains a critical severity keyword.
// Logs matching this check are exempt from adaptive sampling and always passed through.
func isImportant(tokens []Token) bool {
	for _, t := range tokens {
		switch t {
		case Fatal, Error, Panic, Alert, Severe, Critical, Emergency, Warn,
			Exception, Crash, Failure, Deadlock, Timeout:
			return true
		}
	}
	return false
}

func (f AdaptiveSamplerFilter) matches(msg *message.Message, tokens []Token, matchThreshold float64) bool {
	matched := false
	if f.Regex != nil {
		if !f.Regex.Match(msg.GetContent()) {
			return false
		}
		matched = true
	}
	if len(f.SampleTokens) > 0 {
		if !IsMatch(f.SampleTokens, tokens, matchThreshold) {
			return false
		}
		matched = true
	}
	return matched
}

func matchesAnyFilter(filters []AdaptiveSamplerFilter, msg *message.Message, tokens []Token, matchThreshold float64) bool {
	for _, filter := range filters {
		if filter.matches(msg, tokens, matchThreshold) {
			return true
		}
	}
	return false
}

func (s *AdaptiveSampler) shouldSample(msg *message.Message, tokens []Token) bool {
	if matchesAnyFilter(s.config.Exclude, msg, tokens, s.config.MatchThreshold) {
		return false
	}
	if len(s.config.Include) == 0 && !s.config.IncludeConfigured {
		return true
	}
	return matchesAnyFilter(s.config.Include, msg, tokens, s.config.MatchThreshold)
}

func (s *AdaptiveSampler) appendPatternHashTagIfEnabled(msg *message.Message, tokens []Token) {
	if s.config.TagPatternHash {
		msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, adaptiveSamplerLogHashTag(tokens))
	}
}

// Process applies credit-based rate limiting to the message.
// Returns the message if allowed, nil if dropped.
func (s *AdaptiveSampler) Process(msg *message.Message, tokens []Token) *message.Message {
	if !s.shouldSample(msg, tokens) {
		tlmAdaptiveSamplerKept.Inc(s.source)
		return msg
	}
	if s.config.ProtectImportantLogs && isImportant(tokens) {
		tlmAdaptiveSamplerKept.Inc(s.source)
		tlmAdaptiveSamplerProtected.Inc(s.source)
		return msg
	}
	now := s.now()
	detectionOnly := s.config.DetectionOnly
	limits := s.samplingLimits(msg, tokens, now)

	for i := range s.entries {
		if IsMatch(s.entries[i].tokens, tokens, s.config.MatchThreshold) {
			return s.processMatchedEntry(i, msg, now, detectionOnly, limits)
		}
	}
	return s.trackNewPattern(msg, tokens, now, limits)
}

func (s *AdaptiveSampler) samplingLimits(msg *message.Message, tokens []Token, now time.Time) adaptiveSamplerLimits {
	limits := adaptiveSamplerLimits{
		rateLimit: s.config.RateLimit,
		burstSize: s.config.BurstSize,
	}

	containerID := adaptiveSamplerContainerID(msg)
	if containerID == "" {
		return limits
	}
	patternHash := adaptiveSamplerLogHash(tokens)
	boost, ok, activeBoosts := adaptivesampling.DefaultSamplingBoostStore().LookupWithActiveCount(containerID, patternHash, now)
	if !ok {
		if activeBoosts > 0 {
			s.logBoostMiss(msg, tokens, containerID, patternHash, activeBoosts)
		}
		return limits
	}
	limits.boost = boost
	limits.boosted = true
	limits.rateLimit *= boost.RateMultiplier
	limits.burstSize *= boost.BurstMultiplier
	s.logBoostLookup(msg, tokens, limits)
	return limits
}

// processMatchedEntry handles a log that matched the pattern at index i: it refills
// and spends credits, updates tags and counters, re-sorts the pattern table, and
// returns the message when emitted or nil when dropped.
func (s *AdaptiveSampler) processMatchedEntry(i int, msg *message.Message, now time.Time, detectionOnly bool, limits adaptiveSamplerLimits) *message.Message {
	e := &s.entries[i]
	matchedTokens := e.tokens

	// Refill credits based on time elapsed since last seen.
	elapsed := now.Sub(e.lastSeen).Seconds()
	e.credits += elapsed * limits.rateLimit
	if limits.boosted && limits.boost.ID != e.appliedBoostID {
		e.credits += limits.boost.CreditGrant
		e.appliedBoostID = limits.boost.ID
		s.logBoostApplied(limits)
	}
	if e.credits > limits.burstSize {
		e.credits = limits.burstSize
	}
	e.lastSeen = now
	e.matchCount++

	allow := e.credits >= 1.0
	if allow {
		e.credits--
	}
	if limits.boosted {
		s.logBoostOutcome(limits, allow, detectionOnly, false, e.credits, e.matchCount, e.sampled)
	}

	// Compute tag bytes from the user-originated ParsingExtra.Tags before
	// any sampler-internal annotations are added (e.g. noisy_log:true).
	var tb int
	if !allow {
		tb = message.AppendTagMetadataBytes(s.baseBytesEstimate, msg.ParsingExtra.Tags)
	}

	// All mutations to e must complete before bubbling: bubbling swaps entries by
	// value, so e (= &s.entries[i]) aliases a different entry after the first swap.
	s.updateForMatchedPattern(e, msg, matchedTokens, allow, detectionOnly)

	// Bubble the matched entry toward the front to maintain descending order.
	for i > 0 && s.entries[i-1].matchCount < s.entries[i].matchCount {
		s.entries[i-1], s.entries[i] = s.entries[i], s.entries[i-1]
		i--
	}

	if allow {
		tlmAdaptiveSamplerKept.Inc(s.source)
		return msg
	}
	return s.recordDrop(msg, tb, detectionOnly)
}

// trackNewPattern records a never-before-seen pattern, evicting the
// least-frequently-matched entry when the table is full, and emits the message.
func (s *AdaptiveSampler) trackNewPattern(msg *message.Message, tokens []Token, now time.Time, limits adaptiveSamplerLimits) *message.Message {
	tlmAdaptiveSamplerNewPatterns.Inc(s.source)
	if len(s.entries) >= s.config.MaxPatterns {
		tlmAdaptiveSamplerEvictions.Inc(s.source)
		s.entries = s.entries[:len(s.entries)-1]
	}
	credits := limits.burstSize - 1
	var appliedBoostID uint64
	if limits.boosted && limits.boost.CreditGrant > 0 {
		credits += limits.boost.CreditGrant
		if credits > limits.burstSize {
			credits = limits.burstSize
		}
		appliedBoostID = limits.boost.ID
		s.logBoostApplied(limits)
	}
	if limits.boosted {
		s.logBoostOutcome(limits, true, s.config.DetectionOnly, true, credits, 1, 0)
	}
	// New patterns start with matchCount=1 and belong at the end of the sorted list.
	s.entries = append(s.entries, samplerEntry{
		tokens:         tokens,
		credits:        credits,
		lastSeen:       now,
		matchCount:     1,
		sampled:        0,
		appliedBoostID: appliedBoostID,
	})
	tlmAdaptiveSamplerKept.Inc(s.source)
	return msg
}

func (s *AdaptiveSampler) logBoostApplied(limits adaptiveSamplerLimits) {
	pkglog.Infof("[logs/adaptive-sampling] boost applied source=%s container_id=%s pattern_hash=%s rate=%.2f burst=%.2f credit_grant=%.2f boost_id=%d",
		s.source, limits.boost.ContainerID, limits.boost.PatternHash, limits.rateLimit, limits.burstSize, limits.boost.CreditGrant, limits.boost.ID)
}

func (s *AdaptiveSampler) logBoostLookup(msg *message.Message, tokens []Token, limits adaptiveSamplerLimits) {
	count := s.boostLookupLogs.Add(1)
	if !adaptivesampling.ShouldLogDebugSample(count) {
		return
	}
	pkglog.Infof("%s sampler found active boost count=%d source=%q container_id=%q pattern_hash=%q boost_id=%d rate=%.2f burst=%.2f credit_grant=%.2f detection_only=%t pattern=%q content=%q",
		adaptivesampling.DebugLogPrefix,
		count,
		s.source,
		limits.boost.ContainerID,
		limits.boost.PatternHash,
		limits.boost.ID,
		limits.rateLimit,
		limits.burstSize,
		limits.boost.CreditGrant,
		s.config.DetectionOnly,
		adaptivesampling.TruncateDebugString(TokensToString(tokens), 180),
		adaptivesampling.TruncateDebugString(string(msg.GetContent()), 180))
}

func (s *AdaptiveSampler) logBoostMiss(msg *message.Message, tokens []Token, containerID, patternHash string, activeBoosts int) {
	count := s.boostMissLogs.Add(1)
	if !adaptivesampling.ShouldLogDebugSample(count) {
		return
	}
	pkglog.Infof("%s sampler boost lookup miss count=%d source=%q container_id=%q pattern_hash=%q active_boosts=%d detection_only=%t pattern=%q content=%q",
		adaptivesampling.DebugLogPrefix,
		count,
		s.source,
		containerID,
		patternHash,
		activeBoosts,
		s.config.DetectionOnly,
		adaptivesampling.TruncateDebugString(TokensToString(tokens), 180),
		adaptivesampling.TruncateDebugString(string(msg.GetContent()), 180))
}

func (s *AdaptiveSampler) logBoostOutcome(limits adaptiveSamplerLimits, allowed, detectionOnly, newPattern bool, credits float64, matchCount int64, sampledPending int64) {
	count := s.boostOutcomeLogs.Add(1)
	if !adaptivesampling.ShouldLogDebugSample(count) {
		return
	}
	pkglog.Infof("%s sampler processed boosted log count=%d source=%q container_id=%q pattern_hash=%q boost_id=%d allowed=%t new_pattern=%t detection_only=%t credits_remaining=%.2f match_count=%d sampled_pending=%d rate=%.2f burst=%.2f",
		adaptivesampling.DebugLogPrefix,
		count,
		s.source,
		limits.boost.ContainerID,
		limits.boost.PatternHash,
		limits.boost.ID,
		allowed,
		newPattern,
		detectionOnly,
		credits,
		matchCount,
		sampledPending,
		limits.rateLimit,
		limits.burstSize)
}

// Flush is a no-op — the adaptive sampler does not buffer messages.
func (s *AdaptiveSampler) Flush() *message.Message {
	return nil
}

// updateForMatchedPattern runs after a log matched an existing pattern and credits
// decided allow vs deny. It mutates msg tags and e.sampled:
//   - DetectionOnly: tag lines the credit bucket would reject (noisy_log, optional hash),
//     and reset e.sampled (detection-only never accumulates “suppressed since last emit”).
//   - Otherwise: on allow, attach adaptive_sampler_sampled_count from prior denials and
//     reset e.sampled; on deny, increment e.sampled for a future allowed line.
func (s *AdaptiveSampler) updateForMatchedPattern(e *samplerEntry, msg *message.Message, matchedTokens []Token, allow, detectionOnly bool) {
	if detectionOnly {
		if !allow {
			s.appendPatternHashTagIfEnabled(msg, matchedTokens)
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, adaptiveSamplerNoisyLogTag)
		}
		e.sampled = 0
		return
	}
	if allow {
		if e.sampled > 0 {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, adaptiveSamplerSampledCountTag(e.sampled))
		}
		e.sampled = 0
	} else {
		e.sampled++
	}
}

// recordDrop runs when credits rejected a matched-pattern line
// It records drop projection metrics (bytes + optional tag-byte estimate), with the
// detection_only series tag distinguishing detection-only runs from real drops, then resolves
// outcome: msg when DetectionOnly still forwards the line, nil on real drop.
func (s *AdaptiveSampler) recordDrop(msg *message.Message, tb int, detectionOnly bool) *message.Message {
	detectionTag := strconv.FormatBool(detectionOnly)
	tlmAdaptiveSamplerDropped.Add(1, s.source, detectionTag)
	tlmAdaptiveSamplerBytesDropped.Add(float64(msg.RawDataLen), s.source, detectionTag)
	if tb > 0 {
		tlmAdaptiveSamplerTagBytesDropped.Add(float64(tb), s.source, detectionTag)
	}
	if detectionOnly {
		tlmAdaptiveSamplerKept.Inc(s.source)
		return msg
	}
	return nil
}
