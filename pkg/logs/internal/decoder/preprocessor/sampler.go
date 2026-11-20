// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"regexp"
	"slices"
	"strconv"
	"time"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Sampler is the final stage of the Preprocessor. It receives one completed log
// message and returns it unchanged or nil if the message should be dropped.
// tokens are the tokenized first line of the message, used to identify its pattern.
type Sampler interface {
	// Process handles a completed log message and returns it, or nil to drop it.
	// tokens are a borrowed view (see BorrowedTokens), valid only for the duration
	// of the call; a sampler that retains them must Clone first.
	Process(msg *message.Message, tokens BorrowedTokens) *message.Message

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
func (s *NoopSampler) Process(msg *message.Message, _ BorrowedTokens) *message.Message {
	return msg
}

// Flush is a no-op since NoopSampler has no buffered state.
func (s *NoopSampler) Flush() *message.Message {
	return nil
}

var tlmAdaptiveSamplerDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "dropped",
	[]string{"log_source", "detection_only"}, "Number of log messages dropped by the adaptive sampler, or that would be dropped when detection_only is true")

var tlmAdaptiveSamplerBytesDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "bytes_dropped",
	[]string{"log_source", "detection_only"}, "Number of bytes dropped by the adaptive sampler, or that would be dropped when detection_only is true")

var tlmAdaptiveSamplerKept = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "kept",
	[]string{"log_source"}, "Number of log messages emitted by the adaptive sampler")

var tlmAdaptiveSamplerOutcomes = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "outcomes",
	[]string{"severity", "outcome"}, "Adaptive sampler keep/drop decisions while an anomaly severity profile is applied")

var tlmAdaptiveSamplerNewPatterns = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "new_patterns",
	[]string{"log_source"}, "Number of new log patterns added to the adaptive sampler pattern table")

var tlmAdaptiveSamplerEvictions = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "evictions",
	[]string{"log_source"}, "Number of pattern table evictions performed by the adaptive sampler")

var tlmAdaptiveSamplerProtected = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "protected",
	[]string{"log_source"}, "Number of important log messages that bypassed adaptive sampling")

var tlmAdaptiveSamplerTagBytesDropped = telemetryimpl.GetCompatComponent().NewCounter("logs_adaptive_sampler", "tag_bytes_dropped",
	[]string{"log_source", "detection_only"}, "Estimated pre-tailer tag metadata bytes for logs dropped by the adaptive sampler, or that would be dropped when detection_only is true")

func adaptiveSamplerSampledCountTag(count int64) string {
	return "adaptive_sampler_sampled_count:" + strconv.FormatInt(count, 10)
}

const (
	adaptiveSamplerNoisyLogTag = "noisy_log:true"

	// FNV-1a 64-bit parameters (identical to hash/fnv.New64a).
	fnvOffset64 = uint64(14695981039346656037)
	fnvPrime64  = uint64(1099511628211)

	// exactIndexMinEntries is the number of hottest entries scanned linearly
	// before falling back to the O(1) exact-match index. Keeping the hottest
	// entries on the direct SWAR scan avoids hashing overhead for top patterns.
	exactIndexMinEntries = 4

	// exactIndexMinTableSize is the table size at which the exact-match index is
	// built. Below it, a linear scan is cheaper than hashing every log.
	exactIndexMinTableSize = 32
)

// hashTokens64 computes the FNV-1a 64-bit hash of tokens without allocations.
func hashTokens64(tokens []Token) uint64 {
	h := fnvOffset64
	for _, t := range tokens {
		h ^= uint64(t)
		h *= fnvPrime64
	}
	return h
}

// hashTokensIndex is a fast word-at-a-time hash used only as an exactIndex key.
// Every index hit is verified with slices.Equal, so collisions only cost speed.
func hashTokensIndex(tokens []Token) uint64 {
	h := fnvOffset64 ^ uint64(len(tokens))
	for len(tokens) >= 8 {
		_ = tokens[7]
		w := uint64(tokens[0]) | uint64(tokens[1])<<8 | uint64(tokens[2])<<16 | uint64(tokens[3])<<24 |
			uint64(tokens[4])<<32 | uint64(tokens[5])<<40 | uint64(tokens[6])<<48 | uint64(tokens[7])<<56
		h = (h ^ w) * 0x9E3779B97F4A7C15
		h ^= h >> 32
		tokens = tokens[8:]
	}
	for _, t := range tokens {
		h = (h ^ uint64(t)) * fnvPrime64
	}
	return h
}

func adaptiveSamplerLogHashTag(tokens []Token) string {
	return "log_hash:" + strconv.FormatUint(hashTokens64(tokens), 16)
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
	// IsSourceDisabled is called per-message to check whether the current source
	// is in the disabled_sources list. When it returns true the sampler passes the
	// message through without rate-limiting. Using a closure lets the check track
	// ReplaceableSource swaps and future Remote Config updates.
	IsSourceDisabled func() bool
	// PassThrough temporarily disables adaptive sampling for matched messages.
	// When it turns back off, the sampler resumes from the current profile's
	// BurstSize after existing entries are reseeded.
	PassThrough bool
	// SmartSeverityProfilesEnabled switches RateLimit/BurstSize based on the level
	// read from SeverityProvider (see Profiles). Profiles[SeverityLow] must match
	// RateLimit/BurstSize above.
	SmartSeverityProfilesEnabled bool
	// Profiles holds the RateLimit/BurstSize pair per SeverityLevel.
	// Only consulted when SmartSeverityProfilesEnabled is true.
	Profiles [severityeventsdef.NumSeverityLevels]SamplerProfile
	// SeverityProvider returns the current anomaly-detection severity level, or false
	// when no reader is registered yet. Only consulted when SmartSeverityProfilesEnabled
	// is true. Left nil in tests that don't exercise smart severity profiles.
	SeverityProvider func() (severityeventsdef.SeverityLevel, bool)
}

// SamplerProfile is a RateLimit/BurstSize pair for one SeverityLevel.
type SamplerProfile struct {
	RateLimit float64
	BurstSize float64
	// PassThrough skips adaptive-sampler drops while this profile is active.
	PassThrough bool
}

// AdaptiveSamplerFilter matches messages by raw-content regex, structural sample,
// or both.
type AdaptiveSamplerFilter struct {
	Regex        *regexp.Regexp
	SampleTokens []Token
}

// samplerEntry tracks the credit-based rate limiting state for a single log pattern.
type samplerEntry struct {
	tokens     []Token
	hash       uint64    // hashTokensIndex(tokens); valid once exactIndex is built
	credits    float64   // remaining log allowance; decremented on each emitted log
	lastSeen   time.Time // used for credit refill
	matchCount int64     // total number of times this pattern has matched; drives sort order
	sampled    int64     // number of dropped matches since the last emitted log
}

// AdaptiveSampler rate-limits logs by structural pattern using per-pattern credit allowances.
// Structurally similar logs share a credit allowance.
// New or returning patterns receive a full burst allowance before being rate-limited.
//
// entries is maintained as a sorted list in descending matchCount order. The most
// frequently matched patterns appear at the front, so the scan exits early for the
// common case where a hot pattern is matched.
//
// Because every entry added via Process failed IsMatch against all entries present
// at insertion time and IsMatch is symmetric, any two entries in the table are
// pairwise non-matching under MatchThreshold (for MatchThreshold <= 1). Thus an
// exact byte-for-byte match in exactIndex is guaranteed to be the unique matching
// entry in the table, preserving exact first-match ordering while avoiding an O(N)
// scan for warm/tail exact hits.
type AdaptiveSampler struct {
	entries           []samplerEntry
	exactIndex        map[uint64]int // hashTokensIndex(tokens) -> index in entries
	indexedCount      int            // tracks len(entries) managed via Process; -1 disables index on direct test mutations
	config            AdaptiveSamplerConfig
	sourceTag         func() string // resolves the log_source telemetry tag
	now               func() time.Time
	baseBytesEstimate int

	// appliedLevel tracks the last severity profile applied by
	// applyProfileIfChanged. appliedLevelInitialized stays false until a real
	// reader is registered, so the sampler does not treat the no-reader case as
	// an implicit Low profile.
	appliedLevel            severityeventsdef.SeverityLevel
	appliedLevelInitialized bool
}

// UnknownSourceTag is the log_source telemetry tag value used when the source
// cannot be resolved, so the series stays readable instead of carrying an empty tag.
const UnknownSourceTag = "unknown"

// NewAdaptiveSampler creates a new AdaptiveSampler.
// sourceTag resolves the value of the log_source telemetry tag. It is called once
// per message rather than captured up front so that source swaps (e.g. a
// ReplaceableSource replaced on file rotation) are picked up. It must return a
// low-cardinality value — see buildSourceTag in the decoder package.
// baseBytesEstimate is the static portion of the ddtags byte count (source config
// tags + sourcecategory), computed once at decoder construction time.
func NewAdaptiveSampler(config AdaptiveSamplerConfig, sourceTag func() string, baseBytesEstimate int) *AdaptiveSampler {
	return &AdaptiveSampler{
		entries:           make([]samplerEntry, 0, config.MaxPatterns),
		config:            config,
		sourceTag:         sourceTag,
		now:               time.Now,
		baseBytesEstimate: baseBytesEstimate,
	}
}

// resolveSourceTag returns the log_source telemetry tag value for the current
// message, normalizing a missing or empty source to UnknownSourceTag.
func (s *AdaptiveSampler) resolveSourceTag() string {
	if s.sourceTag == nil {
		return UnknownSourceTag
	}
	if src := s.sourceTag(); src != "" {
		return src
	}
	return UnknownSourceTag
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

// applyProfileIfChanged switches RateLimit/BurstSize to the currently published
// level, when SmartSeverityProfilesEnabled is set. Escalation grants every
// pattern a fresh burst immediately. The first available Medium/High level is
// also treated as an escalation from the base Low profile. De-escalation leaves
// credits untouched, letting the refill-time clamp in processMatchedEntry
// shrink them naturally.
func (s *AdaptiveSampler) applyProfileIfChanged() {
	if s.config.SeverityProvider == nil {
		return
	}
	level, ok := s.config.SeverityProvider()
	if !ok {
		return
	}
	if s.appliedLevelInitialized && level == s.appliedLevel {
		return
	}

	wasInitialized := s.appliedLevelInitialized
	previousLevel := s.appliedLevel
	profile := s.config.Profiles[level]
	escalation := (wasInitialized && level > previousLevel) ||
		(!wasInitialized && level > severityeventsdef.SeverityLow)
	leavingPassThrough := wasInitialized && s.config.PassThrough && !profile.PassThrough
	s.config.RateLimit = profile.RateLimit
	s.config.BurstSize = profile.BurstSize
	s.config.PassThrough = profile.PassThrough

	if escalation || leavingPassThrough {
		for i := range s.entries {
			s.entries[i].credits = s.config.BurstSize
			if leavingPassThrough {
				s.entries[i].sampled = 0
			}
		}
	}

	s.appliedLevel = level
	s.appliedLevelInitialized = true
}

// recordSmartSeverityOutcome records a decision made under the severity profile
// that is currently applied to this sampler. Until the severity provider has
// supplied a level, the sampler is using its base settings rather than an AAD
// profile, so no collaboration telemetry is emitted.
func (s *AdaptiveSampler) recordSmartSeverityOutcome(outcome string) {
	if !s.config.SmartSeverityProfilesEnabled || !s.appliedLevelInitialized {
		return
	}
	tlmAdaptiveSamplerOutcomes.Inc(s.appliedLevel.String(), outcome)
}

// Process applies credit-based rate limiting to the message.
// Returns the message if allowed, nil if dropped.
func (s *AdaptiveSampler) Process(msg *message.Message, tokens BorrowedTokens) *message.Message {
	// tailers skip no-content messages via HasContent() before the processor, don't use
	// space in the pattern table for them.
	if !msg.HasContent() {
		return msg
	}
	if s.config.SmartSeverityProfilesEnabled {
		s.applyProfileIfChanged()
	}
	if s.config.IsSourceDisabled != nil && s.config.IsSourceDisabled() {
		return msg
	}
	// raw is borrowed: used only for synchronous reads below. trackNewPattern
	// clones before storing.
	raw := tokens.Borrow()
	// Resolve once per message: the closure reads through a lock, and every
	// counter below tags with the same value.
	sourceTag := s.resolveSourceTag()
	if !s.shouldSample(msg, raw) {
		tlmAdaptiveSamplerKept.Inc(sourceTag)
		return msg
	}
	if s.config.ProtectImportantLogs && IsImportant(raw) {
		tlmAdaptiveSamplerKept.Inc(sourceTag)
		tlmAdaptiveSamplerProtected.Inc(sourceTag)
		return msg
	}
	now := s.now()
	detectionOnly := s.config.DetectionOnly
	passThrough := s.config.PassThrough

	thresh := s.config.MatchThreshold
	n := len(s.entries)
	hotEnd := min(n, exactIndexMinEntries)
	for i := range hotEnd {
		if IsMatch(s.entries[i].tokens, raw, thresh) {
			return s.processMatchedEntry(i, msg, now, sourceTag, detectionOnly, passThrough)
		}
	}

	var rawHash uint64
	var hasHash bool
	if n >= exactIndexMinTableSize && thresh <= 1.0 && s.exactIndex != nil && n == s.indexedCount {
		rawHash = hashTokensIndex(raw)
		hasHash = true
		if idx, ok := s.exactIndex[rawHash]; ok && idx >= exactIndexMinEntries && idx < n && slices.Equal(s.entries[idx].tokens, raw) {
			return s.processMatchedEntry(idx, msg, now, sourceTag, detectionOnly, passThrough)
		}
	}

	for i := hotEnd; i < n; i++ {
		if IsMatch(s.entries[i].tokens, raw, thresh) {
			return s.processMatchedEntry(i, msg, now, sourceTag, detectionOnly, passThrough)
		}
	}
	return s.trackNewPattern(msg, tokens, raw, rawHash, hasHash, now, sourceTag)
}

// processMatchedEntry handles a log that matched the pattern at index i: it refills
// and spends credits, updates tags and counters, re-sorts the pattern table, and
// returns the message when emitted or nil when dropped.
func (s *AdaptiveSampler) processMatchedEntry(i int, msg *message.Message, now time.Time, sourceTag string, detectionOnly, passThrough bool) *message.Message {
	e := &s.entries[i]
	matchedTokens := e.tokens

	// Refill credits based on time elapsed since last seen.
	elapsed := now.Sub(e.lastSeen).Seconds()
	e.credits += elapsed * s.config.RateLimit
	if e.credits > s.config.BurstSize {
		e.credits = s.config.BurstSize
	}
	e.lastSeen = now
	e.matchCount++

	allow := passThrough || e.credits >= 1.0
	if allow && !passThrough {
		e.credits--
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
	maintainIndex := s.exactIndex != nil && len(s.entries) == s.indexedCount
	for i > 0 && s.entries[i-1].matchCount < s.entries[i].matchCount {
		s.entries[i-1], s.entries[i] = s.entries[i], s.entries[i-1]
		if maintainIndex {
			hMovedUp := s.entries[i-1].hash // moved i -> i-1
			hMovedDown := s.entries[i].hash // moved i-1 -> i
			if hMovedUp != hMovedDown {
				if cur, ok := s.exactIndex[hMovedUp]; ok && cur == i {
					s.exactIndex[hMovedUp] = i - 1
				}
				if cur, ok := s.exactIndex[hMovedDown]; ok && cur == i-1 {
					s.exactIndex[hMovedDown] = i
				}
			} else if cur, ok := s.exactIndex[hMovedUp]; ok {
				if cur == i {
					s.exactIndex[hMovedUp] = i - 1
				} else if cur == i-1 {
					s.exactIndex[hMovedUp] = i
				}
			}
		}
		i--
	}

	if allow {
		tlmAdaptiveSamplerKept.Inc(sourceTag)
		s.recordSmartSeverityOutcome("kept")
		return msg
	}
	s.recordSmartSeverityOutcome("dropped")
	return s.recordDrop(msg, tb, sourceTag, detectionOnly)
}

// trackNewPattern records a never-before-seen pattern, evicting the
// least-frequently-matched entry when the table is full, and emits the message.
func (s *AdaptiveSampler) trackNewPattern(msg *message.Message, tokens BorrowedTokens, raw []Token, rawHash uint64, hasHash bool, now time.Time, sourceTag string) *message.Message {
	tlmAdaptiveSamplerNewPatterns.Inc(sourceTag)
	indexValid := s.indexedCount == len(s.entries) && s.config.MatchThreshold <= 1.0
	if !indexValid {
		s.exactIndex = nil
		s.indexedCount = -1
	}

	if len(s.entries) >= s.config.MaxPatterns {
		tlmAdaptiveSamplerEvictions.Inc(sourceTag)
		lastIdx := len(s.entries) - 1
		if indexValid && s.exactIndex != nil {
			evictedHash := s.entries[lastIdx].hash
			if cur, ok := s.exactIndex[evictedHash]; ok && cur == lastIdx {
				delete(s.exactIndex, evictedHash)
			}
		}
		s.entries = s.entries[:lastIdx]
	}
	// Only hash when the index is live; small tables never pay for hashing.
	indexed := indexValid && s.exactIndex != nil
	if indexed && !hasHash {
		rawHash = hashTokensIndex(raw)
	}
	// New patterns start with matchCount=1 and belong at the end of the sorted list.
	// The entry outlives this call, so it must own the tokens.
	newIdx := len(s.entries)
	s.entries = append(s.entries, samplerEntry{
		tokens:     tokens.Clone(),
		hash:       rawHash,
		credits:    s.config.BurstSize - 1,
		lastSeen:   now,
		matchCount: 1,
		sampled:    0,
	})
	if indexValid {
		s.indexedCount = len(s.entries)
		if indexed {
			s.exactIndex[rawHash] = newIdx
		} else if len(s.entries) >= exactIndexMinTableSize {
			// The table just grew large enough: build the index once.
			s.exactIndex = make(map[uint64]int, len(s.entries))
			for i := range s.entries {
				h := hashTokensIndex(s.entries[i].tokens)
				s.entries[i].hash = h
				s.exactIndex[h] = i
			}
		}
	}
	tlmAdaptiveSamplerKept.Inc(sourceTag)
	s.recordSmartSeverityOutcome("kept")
	return msg
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
func (s *AdaptiveSampler) recordDrop(msg *message.Message, tb int, sourceTag string, detectionOnly bool) *message.Message {
	detectionTag := strconv.FormatBool(detectionOnly)
	tlmAdaptiveSamplerDropped.Add(1, sourceTag, detectionTag)
	tlmAdaptiveSamplerBytesDropped.Add(float64(msg.RawDataLen), sourceTag, detectionTag)
	if tb > 0 {
		tlmAdaptiveSamplerTagBytesDropped.Add(float64(tb), sourceTag, detectionTag)
	}
	if detectionOnly {
		tlmAdaptiveSamplerKept.Inc(sourceTag)
		return msg
	}
	return nil
}
