// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package detectors

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
	"github.com/stretchr/testify/require"
)

// GroundTruth defines expected anomalies for a snapshot.
// The key is a metric name prefix that should (or should not) have anomalies.
type GroundTruth struct {
	// ExpectedAnomalies maps metric name prefixes to expected anomaly counts.
	// A positive count means we expect at least that many anomalies.
	// Use -1 to indicate "any number > 0".
	ExpectedAnomalies map[string]int

	// ExpectedStable lists metric name prefixes that should NOT trigger anomalies.
	ExpectedStable []string
}

// HarnessResult captures the results of running a detector against a snapshot.
type HarnessResult struct {
	DetectorName    string
	SnapshotPath    string
	TotalSeries     int
	SeriesAnalyzed  int
	AnomaliesFound  int
	AnomaliesByType map[string]int
	// AnomaliesBySeries maps metric names to anomaly counts
	AnomaliesBySeries map[string]int
	// Errors during evaluation
	Errors []string
}

// Summary returns a formatted summary of the results.
func (r *HarnessResult) Summary() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== %s on %s ===\n", r.DetectorName, filepath.Base(r.SnapshotPath)))
	sb.WriteString(fmt.Sprintf("Series: %d total, %d analyzed\n", r.TotalSeries, r.SeriesAnalyzed))
	sb.WriteString(fmt.Sprintf("Anomalies: %d total\n", r.AnomaliesFound))

	if len(r.AnomaliesByType) > 0 {
		sb.WriteString("  By type:\n")
		for t, count := range r.AnomaliesByType {
			sb.WriteString(fmt.Sprintf("    %s: %d\n", t, count))
		}
	}

	if len(r.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("Errors: %d\n", len(r.Errors)))
		for _, e := range r.Errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	return sb.String()
}

// TopAnomalies returns the metric names with most anomalies (up to n).
func (r *HarnessResult) TopAnomalies(n int) []string {
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range r.AnomaliesBySeries {
		sorted = append(sorted, kv{k, v})
	}
	// Simple bubble sort for small N
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	var result []string
	for i := 0; i < n && i < len(sorted); i++ {
		result = append(result, fmt.Sprintf("%s: %d", sorted[i].k, sorted[i].v))
	}
	return result
}

// RunDetectorOnSnapshot loads a snapshot and runs the detector against all series.
func RunDetectorOnSnapshot(detector mh.Detector, snapshotPath string) (*HarnessResult, error) {
	cache, err := mh.LoadSnapshot(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}

	result := &HarnessResult{
		DetectorName:      detector.Name(),
		SnapshotPath:      snapshotPath,
		AnomaliesByType:   make(map[string]int),
		AnomaliesBySeries: make(map[string]int),
	}

	cache.Scan(func(key mh.SeriesKey, history *mh.MetricHistory) bool {
		result.TotalSeries++

		// Only analyze series with enough data
		if history.Recent.Len() < 5 {
			return true
		}
		result.SeriesAnalyzed++

		anomalies := detector.Analyze(key, history)
		if len(anomalies) > 0 {
			result.AnomaliesFound += len(anomalies)
			result.AnomaliesBySeries[key.Name] += len(anomalies)
			for _, a := range anomalies {
				result.AnomaliesByType[a.Type]++
			}
		}
		return true
	})

	return result, nil
}

// EvaluateAgainstGroundTruth checks results against expected anomalies.
func EvaluateAgainstGroundTruth(result *HarnessResult, truth *GroundTruth) (truePositives, falsePositives, falseNegatives int, errors []string) {
	// Check expected anomalies
	for prefix, expectedCount := range truth.ExpectedAnomalies {
		found := 0
		for metric, count := range result.AnomaliesBySeries {
			if strings.HasPrefix(metric, prefix) {
				found += count
			}
		}
		if expectedCount == -1 {
			// Expect at least one
			if found > 0 {
				truePositives += found
			} else {
				falseNegatives++
				errors = append(errors, fmt.Sprintf("expected anomalies for %q but found none", prefix))
			}
		} else {
			if found >= expectedCount {
				truePositives += found
			} else {
				falseNegatives += expectedCount - found
				errors = append(errors, fmt.Sprintf("expected %d anomalies for %q but found %d", expectedCount, prefix, found))
			}
		}
	}

	// Check that stable metrics didn't trigger
	for _, prefix := range truth.ExpectedStable {
		for metric, count := range result.AnomaliesBySeries {
			if strings.HasPrefix(metric, prefix) {
				falsePositives += count
				errors = append(errors, fmt.Sprintf("expected %q to be stable but found %d anomalies", metric, count))
			}
		}
	}

	return
}

// loadTestSnapshots finds all snapshot files in the testdata directory.
func loadTestSnapshots(t *testing.T) []string {
	t.Helper()

	// Look for snapshots in common locations
	locations := []string{
		"testdata",
		"../testdata",
		"../../../../tmp/metric-history-snapshots",
	}

	for _, loc := range locations {
		pattern := filepath.Join(loc, "snapshot_*.json")
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			return matches
		}
	}

	return nil
}

// TestDetectorHarness_Discovery verifies we can find and load snapshots.
func TestDetectorHarness_Discovery(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found - run generate-snapshot-testdata.sh first")
	}

	t.Logf("Found %d snapshot files", len(snapshots))
	for _, s := range snapshots {
		t.Logf("  - %s", s)
	}
}

// TestDetectorHarness_RobustZScore runs the robust Z-score detector against snapshots.
func TestDetectorHarness_RobustZScore(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found - run generate-snapshot-testdata.sh first")
	}

	detector := NewRobustZScoreDetector()

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			result, err := RunDetectorOnSnapshot(detector, snapshotPath)
			require.NoError(t, err)

			t.Log(result.Summary())

			if len(result.TopAnomalies(10)) > 0 {
				t.Log("Top anomalies:")
				for _, a := range result.TopAnomalies(10) {
					t.Logf("  %s", a)
				}
			}
		})
	}
}

// TestDetectorHarness_MeanChange runs the mean change detector against snapshots.
func TestDetectorHarness_MeanChange(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found - run generate-snapshot-testdata.sh first")
	}

	detector := NewMeanChangeDetector()

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			result, err := RunDetectorOnSnapshot(detector, snapshotPath)
			require.NoError(t, err)

			t.Log(result.Summary())

			if len(result.TopAnomalies(10)) > 0 {
				t.Log("Top anomalies:")
				for _, a := range result.TopAnomalies(10) {
					t.Logf("  %s", a)
				}
			}
		})
	}
}

// TestDetectorHarness_CompareDetectors runs multiple detectors and compares results.
func TestDetectorHarness_CompareDetectors(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found - run generate-snapshot-testdata.sh first")
	}

	detectors := []mh.Detector{
		NewRobustZScoreDetector(),
		NewMeanChangeDetector(),
	}

	// Use first snapshot for comparison
	snapshotPath := snapshots[0]

	t.Logf("Comparing detectors on %s", filepath.Base(snapshotPath))

	var results []*HarnessResult
	for _, detector := range detectors {
		result, err := RunDetectorOnSnapshot(detector, snapshotPath)
		require.NoError(t, err)
		results = append(results, result)
	}

	// Print comparison table
	t.Log("\n=== Detector Comparison ===")
	t.Logf("%-20s %10s %10s %10s", "Detector", "Analyzed", "Anomalies", "Spikes")
	t.Log(strings.Repeat("-", 55))

	for _, r := range results {
		spikes := r.AnomaliesByType["spike"] + r.AnomaliesByType["drop"]
		t.Logf("%-20s %10d %10d %10d", r.DetectorName, r.SeriesAnalyzed, r.AnomaliesFound, spikes)
	}
}

// TestDetectorHarness_WithGroundTruth evaluates detectors against expected results.
// This is the main test for validating detector accuracy.
func TestDetectorHarness_WithGroundTruth(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found - run generate-snapshot-testdata.sh first")
	}

	// Ground truth for CPU spike snapshots
	// The generate script creates CPU spikes via `dd if=/dev/zero of=/dev/null`
	// This causes:
	// - system.cpu.user to spike (the CPU work)
	// - system.cpu.idle to drop (inverse of user)
	// - system.io.* may spike from disk activity during snapshot collection
	cpuSpikeGroundTruth := &GroundTruth{
		ExpectedAnomalies: map[string]int{
			"system.cpu.user": -1, // expect at least one anomaly
			"system.cpu.idle": -1, // idle should drop when user spikes
		},
		ExpectedStable: []string{
			"system.disk.total", // disk capacity shouldn't change
			// Note: system.fs.inodes.total CAN change during disk activity
		},
	}

	detector := NewRobustZScoreDetector()

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			result, err := RunDetectorOnSnapshot(detector, snapshotPath)
			require.NoError(t, err)

			tp, fp, fn, evalErrors := EvaluateAgainstGroundTruth(result, cpuSpikeGroundTruth)

			t.Logf("Evaluation: TP=%d, FP=%d, FN=%d", tp, fp, fn)

			// Log any evaluation issues (but don't fail - this is for iteration)
			for _, e := range evalErrors {
				t.Logf("  [eval] %s", e)
			}

			// Log anomalies found
			t.Log("Anomalies found:")
			for metric, count := range result.AnomaliesBySeries {
				t.Logf("  %s: %d", metric, count)
			}
		})
	}
}

// BenchmarkDetector_RobustZScore benchmarks detector performance on snapshots.
func BenchmarkDetector_RobustZScore(b *testing.B) {
	// Find a snapshot
	pattern := "../../../../tmp/metric-history-snapshots/snapshot_01.json"
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		b.Skip("No snapshot file found")
	}

	cache, err := mh.LoadSnapshot(matches[0])
	if err != nil {
		b.Fatalf("Failed to load snapshot: %v", err)
	}

	detector := NewRobustZScoreDetector()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Scan(func(key mh.SeriesKey, history *mh.MetricHistory) bool {
			detector.Analyze(key, history)
			return true
		})
	}
}

// TestDetectorHarness_ListAllMetrics helps explore what's in a snapshot.
func TestDetectorHarness_ListAllMetrics(t *testing.T) {
	if os.Getenv("HARNESS_EXPLORE") == "" {
		t.Skip("Set HARNESS_EXPLORE=1 to run this exploratory test")
	}

	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	cache, err := mh.LoadSnapshot(snapshots[0])
	require.NoError(t, err)

	metricCounts := make(map[string]int)
	cache.Scan(func(key mh.SeriesKey, history *mh.MetricHistory) bool {
		metricCounts[key.Name]++
		return true
	})

	t.Logf("Metrics in %s:", filepath.Base(snapshots[0]))
	for name, count := range metricCounts {
		t.Logf("  %s: %d series", name, count)
	}
}

// DetailedAnomalyResult contains full anomaly information for debugging.
type DetailedAnomalyResult struct {
	Anomaly     mh.Anomaly
	SeriesName  string
	Tags        []string
	RecentCount int
}

// RunDetectorDetailed returns detailed anomaly information including messages.
func RunDetectorDetailed(detector mh.Detector, snapshotPath string) ([]DetailedAnomalyResult, error) {
	cache, err := mh.LoadSnapshot(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}

	var results []DetailedAnomalyResult

	cache.Scan(func(key mh.SeriesKey, history *mh.MetricHistory) bool {
		if history.Recent.Len() < 5 {
			return true
		}

		anomalies := detector.Analyze(key, history)
		for _, a := range anomalies {
			results = append(results, DetailedAnomalyResult{
				Anomaly:     a,
				SeriesName:  key.Name,
				Tags:        key.Tags,
				RecentCount: history.Recent.Len(),
			})
		}
		return true
	})

	return results, nil
}

// TestDetectorHarness_DetailedAnomalies shows full anomaly details for debugging.
func TestDetectorHarness_DetailedAnomalies(t *testing.T) {
	if os.Getenv("HARNESS_DETAILED") == "" {
		t.Skip("Set HARNESS_DETAILED=1 to run this detailed test")
	}

	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	detector := NewRobustZScoreDetector()

	// Only use first snapshot for detailed output
	results, err := RunDetectorDetailed(detector, snapshots[0])
	require.NoError(t, err)

	t.Logf("Detailed anomalies from %s:", filepath.Base(snapshots[0]))
	t.Log("")

	for _, r := range results {
		t.Logf("Metric: %s", r.SeriesName)
		if len(r.Tags) > 0 {
			t.Logf("  Tags: %v", r.Tags)
		}
		t.Logf("  Type: %s, Severity: %.2f", r.Anomaly.Type, r.Anomaly.Severity)
		t.Logf("  Message: %s", r.Anomaly.Message)
		t.Logf("  Timestamp: %d", r.Anomaly.Timestamp)
		t.Log("")
	}
}

// TestDetectorHarness_FilteredAnomalies shows anomalies for specific metric patterns.
func TestDetectorHarness_FilteredAnomalies(t *testing.T) {
	if os.Getenv("HARNESS_FILTER") == "" {
		t.Skip("Set HARNESS_FILTER=<pattern> to run this filtered test (e.g., HARNESS_FILTER=cpu)")
	}

	filter := os.Getenv("HARNESS_FILTER")
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	detector := NewRobustZScoreDetector()

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			results, err := RunDetectorDetailed(detector, snapshotPath)
			require.NoError(t, err)

			t.Logf("Anomalies matching '%s':", filter)
			found := 0
			for _, r := range results {
				if strings.Contains(r.SeriesName, filter) {
					found++
					t.Logf("  %s [%s] severity=%.2f", r.SeriesName, r.Anomaly.Type, r.Anomaly.Severity)
					t.Logf("    %s", r.Anomaly.Message)
				}
			}
			if found == 0 {
				t.Log("  (none)")
			}
		})
	}
}

// TestDetectorHarness_ParameterSweep tests different detector configurations.
func TestDetectorHarness_ParameterSweep(t *testing.T) {
	if os.Getenv("HARNESS_SWEEP") == "" {
		t.Skip("Set HARNESS_SWEEP=1 to run parameter sweep")
	}

	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	// Test different threshold values
	thresholds := []float64{2.0, 2.5, 3.0, 3.5, 4.0, 5.0}

	t.Log("=== RobustZScore Threshold Sweep ===")
	t.Logf("%-10s %10s %10s %10s", "Threshold", "Anomalies", "Spikes", "Drops")
	t.Log(strings.Repeat("-", 45))

	for _, threshold := range thresholds {
		detector := &RobustZScoreDetector{
			Threshold:     threshold,
			MinDataPoints: 10,
		}

		totalAnomalies := 0
		totalSpikes := 0
		totalDrops := 0

		for _, snapshotPath := range snapshots {
			result, err := RunDetectorOnSnapshot(detector, snapshotPath)
			require.NoError(t, err)

			totalAnomalies += result.AnomaliesFound
			totalSpikes += result.AnomaliesByType["spike"]
			totalDrops += result.AnomaliesByType["drop"]
		}

		t.Logf("%-10.1f %10d %10d %10d", threshold, totalAnomalies, totalSpikes, totalDrops)
	}
}

// TestDetectorHarness_DeltaVsRaw compares delta-first vs raw value analysis.
// This helps evaluate whether delta-first is still the right approach.
func TestDetectorHarness_DeltaVsRaw(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	// Use first snapshot
	snapshotPath := snapshots[0]
	cache, err := mh.LoadSnapshot(snapshotPath)
	require.NoError(t, err)

	t.Log("=== Delta-first vs Raw Value Analysis ===")
	t.Log("Examining all points to find where anomalies occur")
	t.Log("")

	// Analyze a few interesting metrics - mix of counters and gauges
	interestingMetrics := []string{
		"system.cpu.user.total", // cumulative counter
		"system.cpu.idle.total", // cumulative counter
		"system.uptime",         // monotonic counter - should NOT alert
		"system.load.1",         // gauge - oscillates around a value
		"system.mem.used",       // gauge - memory usage
	}

	for _, metricName := range interestingMetrics {
		var history *mh.MetricHistory
		cache.Scan(func(key mh.SeriesKey, h *mh.MetricHistory) bool {
			if key.Name == metricName {
				history = h
				return false
			}
			return true
		})

		if history == nil {
			t.Logf("%s: not found in snapshot", metricName)
			continue
		}

		points := history.Recent.ToSlice()
		if len(points) < 10 {
			t.Logf("%s: insufficient data (%d points)", metricName, len(points))
			continue
		}

		// Extract raw values
		values := make([]float64, len(points))
		for i, p := range points {
			values[i] = p.Stats.Mean()
		}

		// Compute deltas
		deltas := make([]float64, len(values)-1)
		for i := 0; i < len(values)-1; i++ {
			deltas[i] = values[i+1] - values[i]
		}

		// Stats on raw values
		rawMedian := computeMedian(values)
		rawMAD := computeMAD(values, rawMedian)

		// Stats on deltas
		deltaMedian := computeMedian(deltas)
		deltaMAD := computeMAD(deltas, deltaMedian)

		t.Logf("%s (%d points):", metricName, len(points))
		t.Logf("  Raw baseline: median=%.2f, MAD=%.2f", rawMedian, rawMAD)
		t.Logf("  Delta baseline: median=%.2f, MAD=%.2f", deltaMedian, deltaMAD)

		// Check each point for anomalies
		rawAnomalies := 0
		deltaAnomalies := 0

		for i, val := range values {
			rawMScore := 0.0
			if rawMAD > 1e-10 {
				rawMScore = 0.6745 * (val - rawMedian) / rawMAD
			}
			if math.Abs(rawMScore) >= 3.5 {
				rawAnomalies++
				t.Logf("  [RAW ANOMALY] point %d: value=%.2f, M-score=%.1f", i, val, rawMScore)
			}
		}

		for i, delta := range deltas {
			deltaMScore := 0.0
			if deltaMAD > 1e-10 {
				deltaMScore = 0.6745 * (delta - deltaMedian) / deltaMAD
			}
			if math.Abs(deltaMScore) >= 3.5 {
				deltaAnomalies++
				t.Logf("  [DELTA ANOMALY] point %d->%d: delta=%.2f, M-score=%.1f", i, i+1, delta, deltaMScore)
			}
		}

		t.Logf("  Summary: %d raw anomalies, %d delta anomalies", rawAnomalies, deltaAnomalies)
		t.Log("")
	}
}

// TestDetectorHarness_FilteredResults simulates the effect of demo config filtering.
// Shows what anomalies would remain after applying various filter strategies.
func TestDetectorHarness_FilteredResults(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	detector := NewRobustZScoreDetector()

	// Demo config filters:
	// 1. Only CPU/load/mem metrics (include_metrics)
	// 2. Exclude per-core metrics (exclude_metrics)
	// 3. Minimum severity 0.5 (min_severity)
	includePrefixes := []string{"system.cpu.", "system.load.", "system.mem."}
	excludePrefixes := []string{
		"system.cpu.user",   // per-core (not .total)
		"system.cpu.idle",   // per-core
		"system.cpu.system", // per-core
		"system.cpu.iowait",
		"system.cpu.stolen",
		"system.cpu.guest",
	}
	minSeverity := 0.5

	t.Log("=== Filtered Results (simulating demo config) ===")
	t.Log("Include: system.cpu., system.load., system.mem.")
	t.Log("Exclude: per-core CPU metrics (system.cpu.user, .idle, .system without .total)")
	t.Logf("MinSeverity: %.1f", minSeverity)
	t.Log("")

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			results, err := RunDetectorDetailed(detector, snapshotPath)
			require.NoError(t, err)

			// Apply filters
			var filtered []DetailedAnomalyResult
			for _, r := range results {
				// Check include prefixes
				matched := false
				for _, prefix := range includePrefixes {
					if strings.HasPrefix(r.SeriesName, prefix) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}

				// Check exclude prefixes
				excluded := false
				for _, prefix := range excludePrefixes {
					// Exclude if it matches the prefix but NOT if it ends with .total
					if strings.HasPrefix(r.SeriesName, prefix) && !strings.HasSuffix(r.SeriesName, ".total") {
						excluded = true
						break
					}
				}
				if excluded {
					continue
				}

				// Check severity
				if r.Anomaly.Severity < minSeverity {
					continue
				}

				filtered = append(filtered, r)
			}

			t.Logf("Anomalies: %d total -> %d after filtering", len(results), len(filtered))
			for _, r := range filtered {
				t.Logf("  %s [%s] severity=%.2f", r.SeriesName, r.Anomaly.Type, r.Anomaly.Severity)
			}
		})
	}
}

// TestDetectorHarness_BayesianChangepoint runs the Bayesian changepoint detector against snapshots.
func TestDetectorHarness_BayesianChangepoint(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found - run generate-snapshot-testdata.sh first")
	}

	detector := NewBayesianChangepointDetector()
	detector.ReportWindow = 100 // check all points for snapshot analysis

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			result, err := RunDetectorOnSnapshot(detector, snapshotPath)
			require.NoError(t, err)

			t.Log(result.Summary())

			if len(result.TopAnomalies(10)) > 0 {
				t.Log("Top anomalies:")
				for _, a := range result.TopAnomalies(10) {
					t.Logf("  %s", a)
				}
			}
		})
	}
}

// TestDetectorHarness_CompareBayesianVsZScore compares Bayesian vs Z-score detectors.
func TestDetectorHarness_CompareBayesianVsZScore(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found - run generate-snapshot-testdata.sh first")
	}

	bayesian := NewBayesianChangepointDetector()
	bayesian.ReportWindow = 100 // check all points for snapshot analysis

	detectors := []mh.Detector{
		NewRobustZScoreDetector(),
		bayesian,
	}

	t.Log("=== Bayesian Changepoint vs Robust Z-Score Comparison ===")
	t.Log("")

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			t.Logf("Snapshot: %s", filepath.Base(snapshotPath))
			t.Log("")

			var results []*HarnessResult
			for _, detector := range detectors {
				result, err := RunDetectorOnSnapshot(detector, snapshotPath)
				require.NoError(t, err)
				results = append(results, result)
			}

			// Print comparison table
			t.Logf("%-25s %10s %10s %15s", "Detector", "Analyzed", "Anomalies", "Unique Metrics")
			t.Log(strings.Repeat("-", 65))

			for _, r := range results {
				t.Logf("%-25s %10d %10d %15d",
					r.DetectorName, r.SeriesAnalyzed, r.AnomaliesFound, len(r.AnomaliesBySeries))
			}

			// Show overlap in detected metrics
			zscoreMetrics := make(map[string]bool)
			bayesMetrics := make(map[string]bool)

			for metric := range results[0].AnomaliesBySeries {
				zscoreMetrics[metric] = true
			}
			for metric := range results[1].AnomaliesBySeries {
				bayesMetrics[metric] = true
			}

			// Metrics detected by both
			var both []string
			var onlyZscore []string
			var onlyBayes []string

			for metric := range zscoreMetrics {
				if bayesMetrics[metric] {
					both = append(both, metric)
				} else {
					onlyZscore = append(onlyZscore, metric)
				}
			}
			for metric := range bayesMetrics {
				if !zscoreMetrics[metric] {
					onlyBayes = append(onlyBayes, metric)
				}
			}

			t.Log("")
			t.Logf("Overlap: %d metrics detected by both", len(both))
			t.Logf("Only Z-Score: %d metrics", len(onlyZscore))
			t.Logf("Only Bayesian: %d metrics", len(onlyBayes))

			if len(both) > 0 {
				t.Log("")
				t.Log("Metrics detected by both:")
				for _, m := range both {
					t.Logf("  %s (zscore=%d, bayes=%d)",
						m, results[0].AnomaliesBySeries[m], results[1].AnomaliesBySeries[m])
				}
			}

			if len(onlyZscore) > 0 && len(onlyZscore) <= 10 {
				t.Log("")
				t.Log("Metrics only detected by Z-Score:")
				for _, m := range onlyZscore {
					t.Logf("  %s (%d)", m, results[0].AnomaliesBySeries[m])
				}
			}

			if len(onlyBayes) > 0 && len(onlyBayes) <= 10 {
				t.Log("")
				t.Log("Metrics only detected by Bayesian:")
				for _, m := range onlyBayes {
					t.Logf("  %s (%d)", m, results[1].AnomaliesBySeries[m])
				}
			}
		})
	}
}

// TestDetectorHarness_BayesianParameterSweep tests different Bayesian detector configurations.
func TestDetectorHarness_BayesianParameterSweep(t *testing.T) {
	if os.Getenv("HARNESS_SWEEP") == "" {
		t.Skip("Set HARNESS_SWEEP=1 to run parameter sweep")
	}

	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	// Test different hazard/threshold combinations
	hazards := []float64{0.005, 0.01, 0.02, 0.05}
	thresholds := []float64{0.3, 0.5, 0.7, 0.8}

	t.Log("=== Bayesian Changepoint Parameter Sweep ===")
	t.Log("(hazard = expected changepoint frequency, threshold = detection confidence)")
	t.Log("")
	t.Logf("%-10s %-10s %10s %10s %10s", "Hazard", "Threshold", "Anomalies", "Up", "Down")
	t.Log(strings.Repeat("-", 55))

	for _, hazard := range hazards {
		for _, threshold := range thresholds {
			detector := &BayesianChangepointDetector{
				Hazard:        hazard,
				Threshold:     threshold,
				MinDataPoints: 15,
				ReportWindow:  100, // check all points for snapshot analysis
				PriorKappa:    0.1,
				PriorAlpha:    1.0,
				PriorBeta:     1.0,
			}

			totalAnomalies := 0
			totalUp := 0
			totalDown := 0

			for _, snapshotPath := range snapshots {
				result, err := RunDetectorOnSnapshot(detector, snapshotPath)
				require.NoError(t, err)

				totalAnomalies += result.AnomaliesFound
				totalUp += result.AnomaliesByType["changepoint_up"]
				totalDown += result.AnomaliesByType["changepoint_down"]
			}

			t.Logf("%-10.3f %-10.1f %10d %10d %10d",
				hazard, threshold, totalAnomalies, totalUp, totalDown)
		}
	}
}

// TestDetectorHarness_BayesianDetailed shows detailed Bayesian anomalies for debugging.
func TestDetectorHarness_BayesianDetailed(t *testing.T) {
	if os.Getenv("HARNESS_DETAILED") == "" {
		t.Skip("Set HARNESS_DETAILED=1 to run this detailed test")
	}

	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	detector := NewBayesianChangepointDetector()
	detector.ReportWindow = 100 // check all points for snapshot analysis

	// Only use first snapshot for detailed output
	results, err := RunDetectorDetailed(detector, snapshots[0])
	require.NoError(t, err)

	t.Logf("Detailed Bayesian anomalies from %s:", filepath.Base(snapshots[0]))
	t.Log("")

	for _, r := range results {
		t.Logf("Metric: %s", r.SeriesName)
		if len(r.Tags) > 0 {
			t.Logf("  Tags: %v", r.Tags)
		}
		t.Logf("  Type: %s, Severity: %.2f", r.Anomaly.Type, r.Anomaly.Severity)
		t.Logf("  Message: %s", r.Anomaly.Message)
		t.Logf("  Timestamp: %d", r.Anomaly.Timestamp)
		t.Log("")
	}
}

// TestDetectorHarness_BayesianDiagnostic diagnoses why Bayesian misses CPU spikes.
func TestDetectorHarness_BayesianDiagnostic(t *testing.T) {
	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	cache, err := mh.LoadSnapshot(snapshots[0])
	require.NoError(t, err)

	// Find CPU user.total series
	var cpuHistory *mh.MetricHistory
	var cpuKey mh.SeriesKey
	cache.Scan(func(key mh.SeriesKey, h *mh.MetricHistory) bool {
		if key.Name == "system.cpu.user.total" {
			cpuHistory = h
			cpuKey = key
			return false
		}
		return true
	})

	if cpuHistory == nil {
		t.Fatal("system.cpu.user.total not found in snapshot")
	}

	points := cpuHistory.Recent.ToSlice()
	t.Logf("system.cpu.user.total has %d points", len(points))

	// Extract values and deltas
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Stats.Mean()
	}

	deltas := make([]float64, len(values)-1)
	for i := 0; i < len(values)-1; i++ {
		deltas[i] = values[i+1] - values[i]
	}

	// Print the deltas to see the spike
	t.Log("")
	t.Log("Delta values (rate of change):")
	for i, d := range deltas {
		marker := ""
		if math.Abs(d-computeMedian(deltas)) > 5 {
			marker = " <-- SPIKE"
		}
		t.Logf("  [%2d] delta=%.2f%s", i, d, marker)
	}

	// Run both detectors and compare
	t.Log("")
	t.Log("=== Z-Score Analysis ===")
	zscore := NewRobustZScoreDetector()
	zAnomalies := zscore.Analyze(cpuKey, cpuHistory)
	if len(zAnomalies) == 0 {
		t.Log("No anomalies detected by Z-Score")
	} else {
		for _, a := range zAnomalies {
			t.Logf("  %s: %s", a.Type, a.Message)
		}
	}

	t.Log("")
	t.Log("=== Bayesian Analysis ===")
	bayes := NewBayesianChangepointDetector()
	bayes.ReportWindow = 100 // check all points for snapshot analysis
	bAnomalies := bayes.Analyze(cpuKey, cpuHistory)
	if len(bAnomalies) == 0 {
		t.Log("No anomalies detected by Bayesian")
	} else {
		for _, a := range bAnomalies {
			t.Logf("  %s: %s", a.Type, a.Message)
		}
	}

	// Run BOCPD and show all changepoint probabilities
	t.Log("")
	t.Log("=== Bayesian Changepoint Probabilities ===")
	probs := bayes.RunBOCPDExposed(deltas)
	for i, p := range probs {
		marker := ""
		if p > 0.3 {
			marker = " <-- HIGH"
		}
		t.Logf("  [%2d] prob=%.4f%s", i, p, marker)
	}
}

// TestDetectorHarness_BayesianFilteredAnomalies shows Bayesian anomalies for filtered metrics.
func TestDetectorHarness_BayesianFilteredAnomalies(t *testing.T) {
	filter := os.Getenv("HARNESS_FILTER")
	if filter == "" {
		t.Skip("Set HARNESS_FILTER=<pattern> to run this filtered test (e.g., HARNESS_FILTER=cpu)")
	}

	snapshots := loadTestSnapshots(t)
	if len(snapshots) == 0 {
		t.Skip("No snapshot files found")
	}

	detector := NewBayesianChangepointDetector()
	detector.ReportWindow = 100 // check all points for snapshot analysis

	for _, snapshotPath := range snapshots {
		t.Run(filepath.Base(snapshotPath), func(t *testing.T) {
			results, err := RunDetectorDetailed(detector, snapshotPath)
			require.NoError(t, err)

			t.Logf("Bayesian anomalies matching '%s':", filter)
			found := 0
			for _, r := range results {
				if strings.Contains(r.SeriesName, filter) {
					found++
					t.Logf("  %s [%s] severity=%.2f", r.SeriesName, r.Anomaly.Type, r.Anomaly.Severity)
					t.Logf("    %s", r.Anomaly.Message)
				}
			}
			if found == 0 {
				t.Log("  (none)")
			}
		})
	}
}
