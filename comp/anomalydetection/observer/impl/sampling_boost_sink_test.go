// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
)

func TestSamplingBoostEventSinkEmitsBoostForHighLogPatternAnomaly(t *testing.T) {
	store := adaptivesampling.NewSamplingBoostStore()
	now := time.Unix(100, 0)
	score := 20.0 // holt_residual high threshold.
	sink := &samplingBoostEventSink{
		store:     store,
		scorerCfg: DefaultScorerConfig(),
		now:       func() time.Time { return now },
	}

	sink.onEngineEvent(engineEvent{
		kind: eventAnomalyCreated,
		anomalyCreated: &anomalyCreatedEvent{anomaly: observerdef.Anomaly{
			Source: observerdef.SeriesDescriptor{
				Namespace: LogMetricsExtractorName,
				Name:      "log.pattern.abc123.count",
			},
			DetectorName: "holt_residual",
			Score:        &score,
			Context: &observerdef.MetricContext{
				ContainerID: "container-a",
				PatternHash: "abc123",
			},
		}},
	})

	boost, ok := store.Lookup("container-a", "abc123", now)
	require.True(t, ok)
	assert.Equal(t, samplingBoostRateMultiplier, boost.RateMultiplier)
	assert.Equal(t, samplingBoostBurstMultiplier, boost.BurstMultiplier)
	assert.Equal(t, samplingBoostCreditGrant, boost.CreditGrant)
	assert.Equal(t, now.Add(samplingBoostTTL), boost.ExpiresAt)
}

func TestSamplingBoostEventSinkSkipsMediumLogPatternAnomaly(t *testing.T) {
	store := adaptivesampling.NewSamplingBoostStore()
	now := time.Unix(100, 0)
	score := 12.0 // holt_residual medium threshold.
	sink := &samplingBoostEventSink{
		store:     store,
		scorerCfg: DefaultScorerConfig(),
		now:       func() time.Time { return now },
	}

	sink.onEngineEvent(engineEvent{
		kind: eventAnomalyCreated,
		anomalyCreated: &anomalyCreatedEvent{anomaly: observerdef.Anomaly{
			Source: observerdef.SeriesDescriptor{
				Namespace: LogMetricsExtractorName,
				Name:      "log.pattern.abc123.count",
			},
			DetectorName: "holt_residual",
			Score:        &score,
			Context: &observerdef.MetricContext{
				ContainerID: "container-a",
				PatternHash: "abc123",
			},
		}},
	})

	_, ok := store.Lookup("container-a", "abc123", now)
	assert.False(t, ok)
}
