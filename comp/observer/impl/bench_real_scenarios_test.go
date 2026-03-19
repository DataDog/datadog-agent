// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	recorderimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/impl"
)

// scenariosDir is the path to local-only recorded scenario data.
// Relative to the package directory (comp/observer/impl/).
const scenariosDir = "../../../comp/observer/scenarios"

// loadScenarioStorage loads a scenario's parquet metrics into a fresh storage.
// Returns nil if the scenario directory does not exist (for CI skip).
func loadScenarioStorage(b *testing.B, scenarioName string) *timeSeriesStorage {
	b.Helper()
	parquetDir := filepath.Join(scenariosDir, scenarioName, "parquet")
	if _, err := os.Stat(parquetDir); os.IsNotExist(err) {
		b.Skipf("scenario data not found at %s (local-only, not committed)", parquetDir)
		return nil
	}

	metrics, err := recorderimpl.ReadMetricsFromDir(parquetDir)
	if err != nil {
		b.Fatalf("ReadMetricsFromDir(%s): %v", parquetDir, err)
	}

	storage := newTimeSeriesStorage()
	for _, m := range metrics {
		// Strip aggregation suffix from metric name (e.g., ":avg", ":count").
		name := m.Name
		if idx := strings.LastIndex(name, ":"); idx != -1 {
			suffix := name[idx+1:]
			if suffix == "avg" || suffix == "count" || suffix == "sum" || suffix == "min" || suffix == "max" {
				name = name[:idx]
			}
		}
		storage.Add("parquet", name, m.Value, m.Timestamp, m.Tags)
	}
	return storage
}

// BenchmarkRealScenario_Detection runs BOCPD + RRCF + cross_signal on real
// production scenario data. Each iteration replays the full stored dataset.
//
// Requires local scenario data in comp/observer/scenarios/; skipped in CI.
//
// Run with:
//
//	dda inv test --targets=./comp/observer/impl/ -- -run=^$ -bench=BenchmarkRealScenario_Detection -benchmem
func BenchmarkRealScenario_Detection(b *testing.B) {
	scenarios := []string{
		"213_pagerduty",
		"353_postmark",
		"food_delivery_redis",
	}

	for _, scenario := range scenarios {
		scenario := scenario
		b.Run(scenario, func(b *testing.B) {
			// Setup outside timer: load parquet data into storage.
			storage := loadScenarioStorage(b, scenario)

			detectors, correlators, _, _ := defaultCatalog().Instantiate(ComponentSettings{})
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				e.resetFull()
				b.StartTimer()
				e.ReplayStoredData()
			}
		})
	}
}
