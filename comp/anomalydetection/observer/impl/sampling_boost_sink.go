// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"strings"
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
		return false
	}
	if !isBoostableLogPatternAnomaly(anomaly) {
		return false
	}
	if anomalyLevel(anomaly, scorerCfg) < samplingBoostMinAnomalyLevel {
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
	if anomaly.Source.Namespace != LogMetricsExtractorName {
		return false
	}
	if !strings.HasPrefix(anomaly.Source.Name, "log.pattern.") || !strings.HasSuffix(anomaly.Source.Name, ".count") {
		return false
	}
	return anomaly.Context != nil && anomaly.Context.ContainerID != "" && anomaly.Context.PatternHash != ""
}
