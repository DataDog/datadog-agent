// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	samplingBoostMinAnomalyLevel = 3
	samplingBoostTTL             = 2 * time.Minute
	samplingBoostRateMultiplier  = 10.0
	samplingBoostBurstMultiplier = 10.0
	samplingBoostCreditGrant     = 100.0
	samplingBoostScorerHighTTL   = samplingBoostTTL
)

type samplingBoostEventSink struct {
	store     *adaptivesampling.SamplingBoostStore
	scorerCfg observerdef.ScorerConfig
	now       func() time.Time

	mu               sync.Mutex
	scorerHighUntil  time.Time
	recentCandidates map[string]recentBoostCandidate
}

type recentBoostCandidate struct {
	anomaly   observerdef.Anomaly
	expiresAt time.Time
}

func newSamplingBoostEventSink(scorerCfg observerdef.ScorerConfig) *samplingBoostEventSink {
	return &samplingBoostEventSink{
		store:            adaptivesampling.DefaultSamplingBoostStore(),
		scorerCfg:        scorerCfg,
		now:              time.Now,
		recentCandidates: make(map[string]recentBoostCandidate),
	}
}

func (s *samplingBoostEventSink) onEngineEvent(evt engineEvent) {
	if s == nil || evt.kind != eventAnomalyCreated || evt.anomalyCreated == nil {
		return
	}
	s.maybeEmitSamplingBoostForAnomaly(evt.anomalyCreated.anomaly)
}

func (s *samplingBoostEventSink) OnSeverityTransition(evt observerdef.SeverityEvent) {
	if s == nil || evt.Direction != observerdef.ScorerEventEscalation || evt.ToLevel != observerdef.SeverityHigh {
		return
	}
	now := s.now()
	candidates := s.openScorerHighGate(now)
	pkglog.Infof("%s scorer high boost gate opened ttl=%s recent_candidates=%d",
		adaptivesampling.DebugLogPrefix,
		samplingBoostScorerHighTTL,
		len(candidates))
	for _, anomaly := range candidates {
		emitSamplingBoostForAnomaly(anomaly, s.store, now)
	}
}

func (s *samplingBoostEventSink) maybeEmitSamplingBoostForAnomaly(anomaly observerdef.Anomaly) bool {
	if s.store == nil {
		logSamplingBoostDecision(anomaly, 0, false, "nil boost store", false)
		return false
	}
	now := s.now()
	boostable, reason := boostableLogPatternAnomaly(anomaly)
	level := anomalyLevel(anomaly, s.scorerCfg)
	scorerHigh := s.scorerHighActive(now)
	logSamplingBoostDecision(anomaly, level, boostable, reason, scorerHigh)
	if !boostable {
		return false
	}
	s.rememberCandidate(anomaly, now)
	if level < samplingBoostMinAnomalyLevel && !scorerHigh {
		logSamplingBoostSkipLowSeverity(anomaly, level, scorerHigh)
		return false
	}
	return emitSamplingBoostForAnomaly(anomaly, s.store, now)
}

// MaybeEmitSamplingBoostForAnomaly converts a high log-pattern anomaly into a
// short-lived adaptive-sampling boost. It returns true when a boost is emitted.
func MaybeEmitSamplingBoostForAnomaly(anomaly observerdef.Anomaly, store *adaptivesampling.SamplingBoostStore, scorerCfg observerdef.ScorerConfig, now time.Time) bool {
	if store == nil {
		logSamplingBoostDecision(anomaly, 0, false, "nil boost store", false)
		return false
	}
	boostable, reason := boostableLogPatternAnomaly(anomaly)
	level := anomalyLevel(anomaly, scorerCfg)
	logSamplingBoostDecision(anomaly, level, boostable, reason, false)
	if !boostable {
		return false
	}
	if level < samplingBoostMinAnomalyLevel {
		return false
	}
	return emitSamplingBoostForAnomaly(anomaly, store, now)
}

func emitSamplingBoostForAnomaly(anomaly observerdef.Anomaly, store *adaptivesampling.SamplingBoostStore, now time.Time) bool {
	boost := store.Set(adaptivesampling.SamplingBoost{
		ContainerID:     anomaly.Context.ContainerID,
		PatternHash:     anomaly.Context.PatternHash,
		ExpiresAt:       now.Add(samplingBoostTTL),
		RateMultiplier:  samplingBoostRateMultiplier,
		BurstMultiplier: samplingBoostBurstMultiplier,
		CreditGrant:     samplingBoostCreditGrant,
	})
	pkglog.Infof("[logs/adaptive-sampling] boost emitted container_id=%s pattern_hash=%s ttl=%s rate_multiplier=%.2f burst_multiplier=%.2f credit_grant=%.2f boost_id=%d",
		boost.ContainerID, boost.PatternHash, samplingBoostTTL, boost.RateMultiplier, boost.BurstMultiplier, boost.CreditGrant, boost.ID)
	return true
}

func (s *samplingBoostEventSink) rememberCandidate(anomaly observerdef.Anomaly, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recentCandidates == nil {
		s.recentCandidates = make(map[string]recentBoostCandidate)
	}
	s.evictExpiredCandidatesLocked(now)
	s.recentCandidates[boostCandidateKey(anomaly)] = recentBoostCandidate{
		anomaly:   anomaly,
		expiresAt: now.Add(samplingBoostScorerHighTTL),
	}
}

func (s *samplingBoostEventSink) scorerHighActive(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scorerHighUntil.After(now)
}

func (s *samplingBoostEventSink) openScorerHighGate(now time.Time) []observerdef.Anomaly {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scorerHighUntil = now.Add(samplingBoostScorerHighTTL)
	s.evictExpiredCandidatesLocked(now)
	candidates := make([]observerdef.Anomaly, 0, len(s.recentCandidates))
	for _, candidate := range s.recentCandidates {
		candidates = append(candidates, candidate.anomaly)
	}
	return candidates
}

func (s *samplingBoostEventSink) evictExpiredCandidatesLocked(now time.Time) {
	for key, candidate := range s.recentCandidates {
		if !candidate.expiresAt.After(now) {
			delete(s.recentCandidates, key)
		}
	}
}

func boostCandidateKey(anomaly observerdef.Anomaly) string {
	if anomaly.Context == nil {
		return ""
	}
	return anomaly.Context.ContainerID + "\x00" + anomaly.Context.PatternHash
}

func boostableLogPatternAnomaly(anomaly observerdef.Anomaly) (bool, string) {
	if anomaly.Source.Namespace != LogMetricsExtractorName {
		return false, "namespace is not log_metrics_extractor"
	}
	if !isLogPatternCountAnomalySource(anomaly.Source.Namespace, anomaly.Source.Name) {
		return false, "metric is not log.pattern.*.count"
	}
	if anomaly.Context == nil {
		return false, "missing anomaly context"
	}
	if anomaly.Context.ContainerID == "" {
		return false, "missing container_id"
	}
	if anomaly.Context.PatternHash == "" {
		return false, "missing pattern_hash"
	}
	return true, "boostable"
}

func isLogPatternCountAnomalySource(namespace, name string) bool {
	return namespace == LogMetricsExtractorName && strings.HasPrefix(name, "log.pattern.") && strings.HasSuffix(name, ".count")
}

func logSamplingBoostDecision(anomaly observerdef.Anomaly, level int, boostable bool, reason string, scorerHigh bool) {
	var count uint64
	if boostable {
		count = adaptiveSamplingPOCBoostableCandidateLogCount.Add(1)
		if !shouldLogSamplingPOCDebug(count, 50, 100) {
			return
		}
	} else {
		count = adaptiveSamplingPOCNonBoostableDecisionLogCount.Add(1)
		if !adaptivesampling.ShouldLogDebugSample(count) {
			return
		}
	}
	var containerID, patternHash, pattern string
	if anomaly.Context != nil {
		containerID = anomaly.Context.ContainerID
		patternHash = anomaly.Context.PatternHash
		pattern = anomaly.Context.Pattern
	}
	pkglog.Infof("%s boost decision count=%d boostable=%t reason=%q level=%d min_level=%d scorer_high=%t namespace=%q metric=%q detector=%q score=%s container_id=%q pattern_hash=%q pattern=%q",
		adaptivesampling.DebugLogPrefix,
		count,
		boostable,
		reason,
		level,
		samplingBoostMinAnomalyLevel,
		scorerHigh,
		anomaly.Source.Namespace,
		anomaly.Source.Name,
		anomaly.DetectorName,
		scoreForDebug(anomaly.Score),
		containerID,
		patternHash,
		adaptivesampling.TruncateDebugString(pattern, 180))
}

func logSamplingBoostSkipLowSeverity(anomaly observerdef.Anomaly, level int, scorerHigh bool) {
	var containerID, patternHash, pattern string
	if anomaly.Context != nil {
		containerID = anomaly.Context.ContainerID
		patternHash = anomaly.Context.PatternHash
		pattern = anomaly.Context.Pattern
	}
	pkglog.Infof("%s boost skipped reason=%q level=%d min_level=%d scorer_high=%t namespace=%q metric=%q detector=%q score=%s container_id=%q pattern_hash=%q pattern=%q",
		adaptivesampling.DebugLogPrefix,
		"anomaly severity below boost threshold",
		level,
		samplingBoostMinAnomalyLevel,
		scorerHigh,
		anomaly.Source.Namespace,
		anomaly.Source.Name,
		anomaly.DetectorName,
		scoreForDebug(anomaly.Score),
		containerID,
		patternHash,
		adaptivesampling.TruncateDebugString(pattern, 180))
}

func scoreForDebug(score *float64) string {
	if score == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%.4f", *score)
}
