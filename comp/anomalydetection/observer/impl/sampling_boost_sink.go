// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"
	"sync/atomic"
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
)

var samplingBoostDecisionDebugCount atomic.Uint64

type samplingBoostEventSink struct {
	store     *adaptivesampling.SamplingBoostStore
	scorerCfg observerdef.ScorerConfig
	now       func() time.Time
}

func newSamplingBoostEventSink(scorerCfg observerdef.ScorerConfig) *samplingBoostEventSink {
	return &samplingBoostEventSink{
		store:     adaptivesampling.DefaultSamplingBoostStore(),
		scorerCfg: scorerCfg,
		now:       time.Now,
	}
}

func (s *samplingBoostEventSink) onEngineEvent(evt engineEvent) {
	if s == nil || evt.kind != eventAnomalyCreated || evt.anomalyCreated == nil {
		return
	}
	MaybeEmitSamplingBoostForAnomaly(evt.anomalyCreated.anomaly, s.store, s.scorerCfg, s.now())
}

// MaybeEmitSamplingBoostForAnomaly converts a high log-pattern anomaly into a
// short-lived adaptive-sampling boost. It returns true when a boost is emitted.
func MaybeEmitSamplingBoostForAnomaly(anomaly observerdef.Anomaly, store *adaptivesampling.SamplingBoostStore, scorerCfg observerdef.ScorerConfig, now time.Time) bool {
	if store == nil {
		logSamplingBoostDecision(anomaly, 0, false, "nil boost store")
		return false
	}
	boostable, reason := boostableLogPatternAnomaly(anomaly)
	level := anomalyLevel(anomaly, scorerCfg)
	logSamplingBoostDecision(anomaly, level, boostable, reason)
	if !boostable {
		return false
	}
	if level < samplingBoostMinAnomalyLevel {
		return false
	}

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

func isBoostableLogPatternAnomaly(anomaly observerdef.Anomaly) bool {
	boostable, _ := boostableLogPatternAnomaly(anomaly)
	return boostable
}

func boostableLogPatternAnomaly(anomaly observerdef.Anomaly) (bool, string) {
	if anomaly.Source.Namespace != LogMetricsExtractorName {
		return false, "namespace is not log_metrics_extractor"
	}
	if !strings.HasPrefix(anomaly.Source.Name, "log.pattern.") || !strings.HasSuffix(anomaly.Source.Name, ".count") {
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

func logSamplingBoostDecision(anomaly observerdef.Anomaly, level int, boostable bool, reason string) {
	count := samplingBoostDecisionDebugCount.Add(1)
	if !adaptivesampling.ShouldLogDebugSample(count) {
		return
	}
	var containerID, patternHash, pattern string
	if anomaly.Context != nil {
		containerID = anomaly.Context.ContainerID
		patternHash = anomaly.Context.PatternHash
		pattern = anomaly.Context.Pattern
	}
	pkglog.Infof("%s boost decision count=%d boostable=%t reason=%q level=%d min_level=%d namespace=%q metric=%q detector=%q score=%s container_id=%q pattern_hash=%q pattern=%q",
		adaptivesampling.DebugLogPrefix,
		count,
		boostable,
		reason,
		level,
		samplingBoostMinAnomalyLevel,
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
