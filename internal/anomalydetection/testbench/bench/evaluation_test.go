// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"testing"
	"time"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	"github.com/DataDog/datadog-agent/internal/anomalydetection/scorer/scoring"
	"github.com/stretchr/testify/require"
)

func TestScenarioBaselineLearningWindow(t *testing.T) {
	stamp := func(sec int64) string { return time.Unix(sec, 0).UTC().Format(time.RFC3339) }
	b := &Bench{config: Config{BaselineFromEpisode: true}, settings: observerimpl.ComponentSettings{
		Baseline: observerimpl.BaselineConfig{Enabled: true, DurationSec: 120, MuteNoisyMetrics: true}},
		episodeInfo: &EpisodeInfo{Baseline: &EpisodePhase{Start: stamp(100), End: stamp(700)}, Disruption: &EpisodePhase{Start: stamp(700)}}}
	require.NoError(t, b.configureBaselineWindow())
	require.Equal(t, &observerimpl.BaselineWindow{StartSec: 100, EndSec: 300}, b.settings.Baseline.ReplayWindow)
	require.False(t, b.inReplayWindow(99))
	require.True(t, b.inReplayWindow(100))
	require.True(t, b.inReplayWindow(900), "fault and recovery observations still replay")
	require.True(t, b.settings.Baseline.MuteNoisyMetrics)

	// A new scenario replaces the old window; missing labels keep the explicit
	// duration fallback and must not retain a previous scenario's timestamps.
	b.episodeInfo.Baseline.End = stamp(820)
	b.episodeInfo.Disruption.Start = stamp(820)
	require.NoError(t, b.configureBaselineWindow())
	require.Equal(t, int64(340), b.settings.Baseline.ReplayWindow.EndSec)
	b.episodeInfo = nil
	require.NoError(t, b.configureBaselineWindow())
	require.Nil(t, b.settings.Baseline.ReplayWindow)
	require.Equal(t, int64(120), b.settings.Baseline.DurationSec)
}

func TestScenarioBaselineValidationAndOverrides(t *testing.T) {
	for _, tc := range []struct{ name, start, end, onset string }{
		{"invalid start", "bad", "1970-01-01T00:10:00Z", "1970-01-01T00:10:00Z"},
		{"missing end", "1970-01-01T00:00:00Z", "", "1970-01-01T00:10:00Z"},
		{"backwards", "1970-01-01T00:10:00Z", "1970-01-01T00:00:00Z", "1970-01-01T00:10:00Z"},
		{"fault overlap", "1970-01-01T00:00:00Z", "1970-01-01T00:10:00Z", "1970-01-01T00:05:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &Bench{config: Config{BaselineFromEpisode: true}, settings: observerimpl.ComponentSettings{Baseline: observerimpl.BaselineConfig{Enabled: true}},
				episodeInfo: &EpisodeInfo{Baseline: &EpisodePhase{Start: tc.start, End: tc.end}, Disruption: &EpisodePhase{Start: tc.onset}}}
			require.Error(t, b.configureBaselineWindow())
			b.config.BaselineFromEpisode = false // explicit --baseline-duration override
			require.NoError(t, b.configureBaselineWindow())
			require.Nil(t, b.settings.Baseline.ReplayWindow)
			b.config.BaselineFromEpisode = true
			b.settings.Baseline.Enabled = false
			require.NoError(t, b.configureBaselineWindow())
			require.Nil(t, b.settings.Baseline.ReplayWindow)
		})
	}
}

func TestScenarioBaselineRoundsCompletedSeconds(t *testing.T) {
	b := &Bench{config: Config{BaselineFromEpisode: true}, settings: observerimpl.ComponentSettings{Baseline: observerimpl.BaselineConfig{Enabled: true}},
		episodeInfo: &EpisodeInfo{Baseline: &EpisodePhase{Start: "2026-01-01T00:00:00.5Z", End: "2026-01-01T00:10:00.5Z"}}}
	require.NoError(t, b.configureBaselineWindow())
	start, err := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	require.NoError(t, err)
	require.Equal(t, start.Unix()+1, b.settings.Baseline.ReplayWindow.StartSec)
	require.Equal(t, start.Unix()+201, b.settings.Baseline.ReplayWindow.EndSec)
}

func TestHealthyRecordingMetadata(t *testing.T) {
	data := []byte(`{"name":"healthy-recording","scenario":"my-app","mode":"healthy-control","expectation":"healthy","disruptions":[],"evaluation_window":{"start":"2026-01-01T00:05:00Z","end":"2026-01-01T00:35:00Z"}}`)
	var info EpisodeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.Scenario.AppName != "my-app" || info.Mode != "healthy-control" || info.Disruption != nil || info.Cooldown != nil {
		t.Fatalf("lost healthy metadata: %+v", info)
	}
	if _, _, err := scoring.HealthyWindow(info.HealthyEpisode); err != nil {
		t.Fatal(err)
	}
	// The explicit disruption must remain visible to validation even though the
	// testbench also has a typed phase field with the same JSON name.
	bad := []byte(`{"mode":"healthy-control","expectation":"healthy","disruption":{"start":"2026-01-01T00:10:00Z"}}`)
	if err := json.Unmarshal(bad, &info); err != nil {
		t.Fatal(err)
	}
	if _, _, err := scoring.HealthyWindow(info.HealthyEpisode); err == nil {
		t.Fatal("accepted healthy recording containing a fault")
	}
}
