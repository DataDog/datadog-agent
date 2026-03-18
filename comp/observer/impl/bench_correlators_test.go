// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"runtime"
	"testing"
)

// BenchmarkCorrelators_Isolated measures the advance-loop cost with each correlator
// enabled individually, plus a no-correlator baseline and an all-enabled comparison.
// BOCPD is always on to generate anomaly events so correlators have real work to do.
//
// Sub-benchmarks:
//   - none:         BOCPD only, no correlators (baseline)
//   - cross_signal: BOCPD + CrossSignal only
//   - time_cluster: BOCPD + TimeCluster only
//   - lead_lag:     BOCPD + LeadLag only
//   - surprise:     BOCPD + Surprise only
//   - all:          BOCPD + all four correlators
//
// Compare each correlator sub-benchmark to "none" to isolate its cost.
// Compare "all" to "none" to see total correlator overhead.
func BenchmarkCorrelators_Isolated(b *testing.B) {
	variants := []struct {
		name        string
		correlators []string
	}{
		{"none", nil},
		{"cross_signal", []string{"cross_signal"}},
		{"time_cluster", []string{"time_cluster"}},
		{"lead_lag", []string{"lead_lag"}},
		{"surprise", []string{"surprise"}},
		{"all", []string{"cross_signal", "time_cluster", "lead_lag", "surprise"}},
	}

	for _, v := range variants {
		v := v
		b.Run(fmt.Sprintf("correlator=%s", v.name), func(b *testing.B) {
			cat := bocpdOnlyCatalog()
			for _, name := range v.correlators {
				cat = cat.WithDefaultEnabled(name, true)
			}

			const numSeries = 50
			storage := buildSyntheticStorage(numSeries, 600)
			detectors, correlators, _ := cat.Instantiate(nil)
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})

			// Warm up: advance BOCPD cursors to end of history.
			e.ReplayStoredData()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				newSec := int64(600 + i)

				b.StopTimer()
				// Constant high-value data: BOCPD fires anomalies every second,
				// giving correlators a steady stream of events to process.
				for s := 0; s < numSeries; s++ {
					e.Storage().Add("ns", fmt.Sprintf("metric_%d", s), 200.0, newSec, nil)
				}
				runtime.GC()
				b.StartTimer()

				e.Advance(newSec)
			}
		})
	}
}
