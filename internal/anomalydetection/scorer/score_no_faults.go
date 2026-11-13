// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/DataDog/datadog-agent/internal/anomalydetection/scorer/scoring"
)

func scoreNoFaults(outputPath, scenariosDir string) (*scoring.NoFaultResult, error) {
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, err
	}
	var output ObserverOutput
	if err = json.Unmarshal(data, &output); err != nil {
		return nil, err
	}
	if !slices.Contains(output.Metadata.CorrelatorsEnabled, "anomaly_scorer") {
		return nil, fmt.Errorf("no-fault evaluation requires anomaly_scorer")
	}
	var stats struct {
		InputMetricsCount int64 `json:"input_metrics_count"`
		InputLogsCount    int64 `json:"input_logs_count"`
	}
	if err := json.Unmarshal(output.Metadata.Stats, &stats); err != nil {
		return nil, fmt.Errorf("invalid replay input statistics: %w", err)
	}
	if stats.InputMetricsCount <= 0 && stats.InputLogsCount <= 0 {
		return nil, fmt.Errorf("replay consumed no input")
	}
	if output.AnomalyPeriods == nil {
		return nil, fmt.Errorf("replay output is missing anomaly_periods")
	}

	scenario := output.Metadata.Scenario
	if scenario == "" || filepath.Base(scenario) != scenario || scenario == ".." {
		return nil, fmt.Errorf("invalid metadata.scenario")
	}
	data, err = os.ReadFile(filepath.Join(scenariosDir, scenario, "episode.json"))
	if err != nil {
		return nil, err
	}
	var episode scoring.HealthyEpisode
	if err = json.Unmarshal(data, &episode); err != nil {
		return nil, err
	}
	periods := make([]scoring.Period, 0, len(output.AnomalyPeriods))
	for _, period := range output.AnomalyPeriods {
		periods = append(periods, scoring.Period{Pattern: period.Pattern, Start: float64(period.PeriodStart), End: float64(period.PeriodEnd)})
	}
	return scoring.ScoreNoFaults(periods, episode)
}
