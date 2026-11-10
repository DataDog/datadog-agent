// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scoring

import (
	"encoding/json"
	"testing"
)

func healthyEpisode() HealthyEpisode {
	return HealthyEpisode{Mode: "healthy-control", Expectation: "healthy", EvaluationWindow: &Window{Start: "2026-01-01T00:05:00.000001Z", End: "2026-01-01T00:35:00.000001Z"}}
}

func TestHealthyEpisodeCounting(t *testing.T) {
	episode := healthyEpisode()
	start, end, err := HealthyWindow(episode)
	if err != nil {
		t.Fatal(err)
	}
	periods := []Period{
		{Pattern: "anomaly_scorer_high:warmup", Start: start - 1, End: start + 20},
		{Pattern: "anomaly_scorer_high:one", Start: start, End: start + 100},
		{Pattern: "anomaly_scorer_high:one", Start: start, End: start + 150},
		{Pattern: "anomaly_scorer_high:one", Start: start + 200, End: start + 250},
		{Pattern: "anomaly_scorer_medium:ignored", Start: start + 30},
		{Pattern: "time_cluster:ignored", Start: start + 40},
		{Pattern: "anomaly_scorer_high:outside", Start: end},
	}
	result, err := ScoreNoFaults(periods, episode)
	if err != nil {
		t.Fatal(err)
	}
	if result.FalsePositives != 2 || len(result.Episodes) != 2 || result.NumFilteredWarmup != 1 || result.NumFilteredOutsideScenario != 1 || result.NumFilteredNonHigh != 2 {
		t.Fatalf("unexpected score: %+v", result)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"f1", "precision", "recall"} {
		if _, found := fields[key]; found {
			t.Fatalf("unexpected %s", key)
		}
	}
}

func TestRejectsAmbiguousHealthyMetadata(t *testing.T) {
	for _, mutate := range []func(*HealthyEpisode){
		func(e *HealthyEpisode) { e.Mode = "" },
		func(e *HealthyEpisode) { e.Expectation = "" },
		func(e *HealthyEpisode) { e.EvaluationWindow = nil },
		func(e *HealthyEpisode) { e.EvaluationWindow.End = e.EvaluationWindow.Start },
		func(e *HealthyEpisode) { e.Disruption = json.RawMessage(`{"start":"2026-01-01T00:10:00Z"}`) },
		func(e *HealthyEpisode) { e.Disruptions = json.RawMessage(`[{"fault":"yes"}]`) },
	} {
		episode := healthyEpisode()
		mutate(&episode)
		if _, err := ScoreNoFaults(nil, episode); err == nil {
			t.Fatalf("accepted invalid episode: %+v", episode)
		}
	}
}

func TestEmptyDisruptionsAllowWhitespace(t *testing.T) {
	episode := healthyEpisode()
	episode.Disruption = json.RawMessage("{ \n }")
	episode.Disruptions = json.RawMessage("[ \n ]")
	if _, _, err := HealthyWindow(episode); err != nil {
		t.Fatal(err)
	}
}
