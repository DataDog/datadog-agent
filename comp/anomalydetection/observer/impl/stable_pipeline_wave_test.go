// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

func stablePipelineInnerGate(p *MetricPipeline) *relativeChangeGate {
	return p.registrations[0].detector.(*StablePipeline).gate
}

func TestStablePipelineSquareWavesWithDefaults(t *testing.T) {
	s := newDetectorTestStorage()
	p := newScanMWPipeline(t, DefaultScanMWConfig())
	e := mustNewEngine(engineConfig{
		storage:                    s,
		detectors:                  []detectorRegistration{{detector: p}},
		trackDetectorOutputHistory: true,
	})

	addStableWavePoint := func(key uint64, name string, value float64, ts int64) observer.SeriesRef {
		return s.AddWithKeyAndHostComposite("work", name, "", value, ts,
			tagset.CompositeTags{}, key, metrics.GaugeType).Ref
	}
	smallRef := addStableWavePoint(1, "small", 100, 1)
	largeRef := addStableWavePoint(2, "large", 100, 1)

	for ts := int64(2); ts <= 35; ts++ {
		addStableWavePoint(1, "small", 100, ts)
		addStableWavePoint(2, "large", 100, ts)
	}
	e.Advance(35)
	for _, ref := range []observer.SeriesRef{smallRef, largeRef} {
		kind, _ := s.SeriesKind(ref)
		require.Equal(t, observer.STABLE, kind, "series %s", s.GetSeriesMeta(ref).Name)
	}

	var smallAccepted, largeAccepted int
	for ts := int64(36); ts <= 240; ts++ {
		high := (ts-36)/60%2 == 0
		smallValue, largeValue := 100.0, 100.0
		if high {
			smallValue, largeValue = 104, 140
		}
		if ts != 80 && ts != 170 {
			addStableWavePoint(1, "small", smallValue, ts)
		}
		addStableWavePoint(2, "large", largeValue, ts)
		for _, anomaly := range e.Advance(ts).anomalies {
			switch anomaly.Source.Name {
			case "small":
				smallAccepted++
			case "large":
				largeAccepted++
			default:
				t.Fatalf("unexpected accepted anomaly source %q", anomaly.Source.Name)
			}
		}
	}

	require.Positive(t, largeAccepted, "the large wave must be accepted")
	require.GreaterOrEqual(t, largeAccepted, 3, "the large wave must expose repeated edges")
	require.Zero(t, smallAccepted, "the small wave must remain gated")
	require.Positive(t, stablePipelineInnerGate(p).rejected, "the small wave must exercise the relative gate")
	for _, ref := range []observer.SeriesRef{smallRef, largeRef} {
		kind, _ := s.SeriesKind(ref)
		require.Equal(t, observer.STABLE, kind, "series %s", s.GetSeriesMeta(ref).Name)
	}
	require.Len(t, e.DetectorOutputAnomalies(), largeAccepted,
		"only post-gate candidates must become detector outputs")
}

func TestStablePipelineDetectsLaterEdgeAfterDeferredAdvance(t *testing.T) {
	s := newDetectorTestStorage()
	p := newScanMWPipeline(t, DefaultScanMWConfig())
	e := mustNewEngine(engineConfig{
		storage:                    s,
		detectors:                  []detectorRegistration{{detector: p}},
		trackDetectorOutputHistory: true,
	})

	var ref observer.SeriesRef
	addPoint := func(value float64, ts int64) {
		ref = s.AddWithKeyAndHostComposite("work", "metric", "", value, ts,
			tagset.CompositeTags{}, 1, metrics.GaugeType).Ref
	}
	for ts := int64(1); ts <= 45; ts++ {
		addPoint(100, ts)
	}
	e.Advance(45)
	kind, _ := s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)

	// Do not advance through the first edge. Its start has fallen outside
	// ScanMW's trailing window by the time analysis finally runs.
	for ts := int64(46); ts <= 160; ts++ {
		addPoint(140, ts)
	}
	for ts := int64(161); ts <= 220; ts++ {
		addPoint(100, ts)
	}
	accepted := e.Advance(220).anomalies
	require.NotEmpty(t, accepted, "a later edge must remain detectable")
	require.Greater(t, accepted[0].Timestamp, int64(160), "must detect the later edge")
	kind, _ = s.SeriesKind(ref)
	require.Equal(t, observer.STABLE, kind)
}
