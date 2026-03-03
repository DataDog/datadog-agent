// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBenchForOutput creates a minimal TestBench with injected state for testing
// observer output. No scenarios dir or registry needed.
func newTestBenchForOutput() *TestBench {
	return &TestBench{
		storage:           newTimeSeriesStorage(),
		components:        make(map[string]*registeredComponent),
		metricsAnomalies:  []observerdef.Anomaly{},
		metricsByDetector: make(map[string][]observerdef.Anomaly),
	}
}

// testCorrelation returns a test correlation with known values.
func testCorrelation() observerdef.ActiveCorrelation {
	return observerdef.ActiveCorrelation{
		Pattern:         "cluster_1000",
		Title:           "Correlated anomalies at 1000",
		MemberSeriesIDs: []observerdef.SeriesID{"parquet|cpu.user:avg|host:a", "parquet|mem.used:avg|host:a"},
		FirstSeen:       1000,
		LastUpdated:     1500,
		Anomalies: []observerdef.Anomaly{
			{
				Timestamp:      1000,
				Source:         "cpu.user:avg",
				SourceSeriesID: "parquet|cpu.user:avg|host:a",
				DetectorName:   "cusum",
				Description:    "CUSUM detected shift in cpu.user:avg",
			},
			{
				Timestamp:      1200,
				Source:         "mem.used:avg",
				SourceSeriesID: "parquet|mem.used:avg|host:a",
				DetectorName:   "cusum",
				Description:    "CUSUM detected shift in mem.used:avg",
			},
		},
	}
}

func TestWriteObserverOutput_EmptyScenario(t *testing.T) {
	tb := newTestBenchForOutput()

	outPath := filepath.Join(t.TempDir(), "results.json")
	err := tb.WriteObserverOutput(outPath, false)
	require.NoError(t, err)

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)

	var output ObserverOutput
	require.NoError(t, json.Unmarshal(data, &output))

	assert.Equal(t, "", output.Metadata.Scenario)
	assert.Equal(t, int64(0), output.Metadata.TimelineStart)
	assert.Equal(t, int64(0), output.Metadata.TimelineEnd)
	assert.Equal(t, 0, output.Metadata.TotalAnomalyPeriods)
	assert.Empty(t, output.AnomalyPeriods)
	assert.Empty(t, output.Metadata.DetectorsEnabled)
	assert.Empty(t, output.Metadata.CorrelatorsEnabled)
}

func TestWriteObserverOutput_NonVerbose(t *testing.T) {
	tb := newTestBenchForOutput()
	tb.loadedScenario = "test_scenario"
	tb.storage.Add("parquet", "cpu.user", 1.0, 1000, []string{"host:a"})
	tb.storage.Add("parquet", "cpu.user", 2.0, 2000, []string{"host:a"})
	tb.correlations = []observerdef.ActiveCorrelation{testCorrelation()}
	tb.components["cusum"] = &registeredComponent{
		Registration: ComponentRegistration{Name: "cusum", Category: "detector"},
		Enabled:      true,
	}

	outPath := filepath.Join(t.TempDir(), "results.json")
	require.NoError(t, tb.WriteObserverOutput(outPath, false))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)

	var output ObserverOutput
	require.NoError(t, json.Unmarshal(data, &output))

	require.Len(t, output.AnomalyPeriods, 1)
	corr := output.AnomalyPeriods[0]

	// Time span always present
	assert.Equal(t, "cluster_1000", corr.Pattern)
	assert.Equal(t, int64(1000), corr.PeriodStart)
	assert.Equal(t, int64(1500), corr.PeriodEnd)

	// Verbose-only fields omitted
	assert.Empty(t, corr.Title)
	assert.Empty(t, corr.MemberSeries)
	assert.Empty(t, corr.Anomalies)
}

func TestWriteObserverOutput_Verbose(t *testing.T) {
	tb := newTestBenchForOutput()
	tb.loadedScenario = "test_scenario"
	tb.storage.Add("parquet", "cpu.user", 1.0, 1000, []string{"host:a"})
	tb.storage.Add("parquet", "cpu.user", 2.0, 2000, []string{"host:a"})
	tb.correlations = []observerdef.ActiveCorrelation{testCorrelation()}
	tb.components["cusum"] = &registeredComponent{
		Registration: ComponentRegistration{Name: "cusum", Category: "detector"},
		Enabled:      true,
	}
	tb.components["time_cluster"] = &registeredComponent{
		Registration: ComponentRegistration{Name: "time_cluster", Category: "correlator"},
		Enabled:      true,
	}
	tb.components["bocpd"] = &registeredComponent{
		Registration: ComponentRegistration{Name: "bocpd", Category: "detector"},
		Enabled:      false, // disabled — should not appear in metadata
	}

	outPath := filepath.Join(t.TempDir(), "results.json")
	require.NoError(t, tb.WriteObserverOutput(outPath, true))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)

	var output ObserverOutput
	require.NoError(t, json.Unmarshal(data, &output))

	// Metadata
	assert.Equal(t, "test_scenario", output.Metadata.Scenario)
	assert.Equal(t, int64(1000), output.Metadata.TimelineStart)
	assert.Equal(t, int64(2000), output.Metadata.TimelineEnd)
	assert.Equal(t, []string{"cusum"}, output.Metadata.DetectorsEnabled)
	assert.Equal(t, []string{"time_cluster"}, output.Metadata.CorrelatorsEnabled)
	assert.Equal(t, 1, output.Metadata.TotalAnomalyPeriods)

	// Correlation with full detail
	require.Len(t, output.AnomalyPeriods, 1)
	corr := output.AnomalyPeriods[0]
	assert.Equal(t, "cluster_1000", corr.Pattern)
	assert.Equal(t, "Correlated anomalies at 1000", corr.Title)
	assert.Equal(t, int64(1000), corr.PeriodStart)
	assert.Equal(t, int64(1500), corr.PeriodEnd)
	assert.Equal(t, []string{"parquet|cpu.user:avg|host:a", "parquet|mem.used:avg|host:a"}, corr.MemberSeries)

	// Nested anomalies
	require.Len(t, corr.Anomalies, 2)
	assert.Equal(t, int64(1000), corr.Anomalies[0].Timestamp)
	assert.Equal(t, "cpu.user:avg", corr.Anomalies[0].Source)
	assert.Equal(t, "parquet|cpu.user:avg|host:a", corr.Anomalies[0].SourceSeriesID)
	assert.Equal(t, "cusum", corr.Anomalies[0].Detector)

	assert.Equal(t, int64(1200), corr.Anomalies[1].Timestamp)
	assert.Equal(t, "mem.used:avg", corr.Anomalies[1].Source)
	assert.Equal(t, "cusum", corr.Anomalies[1].Detector)
}

func TestWriteObserverOutput_TimelineBoundsFromStorage(t *testing.T) {
	tb := newTestBenchForOutput()
	tb.storage.Add("parquet", "disk.io", 10.0, 5000, []string{"device:sda"})
	tb.storage.Add("parquet", "disk.io", 20.0, 9000, []string{"device:sda"})

	outPath := filepath.Join(t.TempDir(), "results.json")
	require.NoError(t, tb.WriteObserverOutput(outPath, false))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)

	var output ObserverOutput
	require.NoError(t, json.Unmarshal(data, &output))

	assert.Equal(t, int64(5000), output.Metadata.TimelineStart)
	assert.Equal(t, int64(9000), output.Metadata.TimelineEnd)
}

func TestWriteObserverOutput_ValidJSON(t *testing.T) {
	tb := newTestBenchForOutput()
	tb.loadedScenario = "json_validity_check"
	tb.correlations = []observerdef.ActiveCorrelation{
		{
			Pattern:         "p1",
			Title:           "Title with \"quotes\" and\nnewlines",
			MemberSeriesIDs: []observerdef.SeriesID{"s1"},
			FirstSeen:       100,
			LastUpdated:     200,
			Anomalies: []observerdef.Anomaly{
				{
					Timestamp:      100,
					Source:         "metric:avg",
					SourceSeriesID: "s1",
					DetectorName:   "cusum",
				},
			},
		},
	}

	// Both modes produce valid JSON
	for _, verbose := range []bool{false, true} {
		outPath := filepath.Join(t.TempDir(), "results.json")
		require.NoError(t, tb.WriteObserverOutput(outPath, verbose))

		data, err := os.ReadFile(outPath)
		require.NoError(t, err)
		assert.True(t, json.Valid(data), "output should be valid JSON (verbose=%v)", verbose)

		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Contains(t, raw, "metadata")
		assert.Contains(t, raw, "anomaly_periods")
	}
}
