// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStorageBackedTailMatchesFullRangeWindow covers the common replacement
// for retained detector rings: selecting a bounded tail at dataTime, then
// filtering by a scan segment boundary. It includes a same-bucket merge.
func TestStorageBackedTailMatchesFullRangeWindow(t *testing.T) {
	storage := newDetectorTestStorage()
	var ref observer.SeriesRef
	for timestamp := int64(1); timestamp <= 10; timestamp++ {
		result := storage.Add("ns", "metric", float64(timestamp), timestamp, nil)
		ref = result.Ref
	}
	storage.Add("ns", "metric", 30, 10, nil) // average in bucket 10 becomes 20

	tests := []struct {
		name         string
		end          int64
		maxPoints    int
		segmentStart int64
		want         []int64
	}{
		{"before end", 6, 3, 0, []int64{4, 5, 6}},
		{"exact max", 10, 4, 0, []int64{7, 8, 9, 10}},
		{"segment boundary", 10, 6, 7, []int64{8, 9, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := collectLastPoints(storage, ref, tt.end, tt.maxPoints, observer.AggregateAverage, nil)
			kept := got[:0]
			for _, point := range got {
				if point.Timestamp > tt.segmentStart {
					kept = append(kept, point)
				}
			}
			var timestamps []int64
			for _, point := range kept {
				timestamps = append(timestamps, point.Timestamp)
			}
			assert.Equal(t, tt.want, timestamps)
			if tt.end == 10 {
				require.NotEmpty(t, kept)
				assert.Equal(t, 20.0, kept[len(kept)-1].Value)
			}
		})
	}
}

// TestStorageBackedScanDetectors_OneShotMatchesIncremental verifies that scan
// detectors observe the same bounded storage window in batch and live replay.
func TestStorageBackedScanDetectors_OneShotMatchesIncremental(t *testing.T) {
	tests := []struct {
		name        string
		newDetector func() observer.Detector
	}{
		{
			name: "scanmw",
			newDetector: func() observer.Detector {
				d := testScanMWDetector()
				d.MinPoints, d.MinSegment, d.MaxPoints = 12, 6, 24
				return d
			},
		},
		{
			name: "scanwelch",
			newDetector: func() observer.Detector {
				d := testScanWelchDetector()
				d.MinPoints, d.MinSegment, d.MaxPoints = 12, 6, 24
				return d
			},
		},
	}

	values := make([]float64, 24)
	for i := range values {
		if i < 12 {
			values[i] = 10
		} else {
			values[i] = 100
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batchStorage := newDetectorTestStorage()
			incrementalStorage := newDetectorTestStorage()
			for index, value := range values {
				batchStorage.Add("ns", tt.name, value, int64(index+1), nil)
			}
			batch := tt.newDetector().Detect(batchStorage, int64(len(values)))

			incrementalDetector := tt.newDetector()
			var incremental []observer.Anomaly
			for index, value := range values {
				timestamp := int64(index + 1)
				incrementalStorage.Add("ns", tt.name, value, timestamp, nil)
				incremental = append(incremental, incrementalDetector.Detect(incrementalStorage, timestamp).Anomalies...)
			}
			assert.Equal(t, anomalyTimestamps(batch.Anomalies), anomalyTimestamps(incremental))
		})
	}
}

// TestHoltResidual_WarmupSeedsFromScalarHalves verifies that scalar warmup
// aggregation preserves the previous two-half initialization, including the
// deliberate omission of the middle point for odd-sized windows.
func TestHoltResidual_WarmupSeedsFromScalarHalves(t *testing.T) {
	tests := []struct {
		name                 string
		warmup               int
		values               []float64
		wantLevel, wantTrend float64
	}{
		{"even", 4, []float64{2, 4, 10, 14}, 3, 4.5},
		{"odd", 5, []float64{2, 4, 100, 10, 14}, 3, 4.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testHoltResidualDetector()
			d.WarmupPoints, d.ResidualWindow = tt.warmup, 1
			storage := newDetectorTestStorage()
			for index, value := range tt.values {
				storage.Add("ns", "metric", value, int64(index+1), nil)
			}
			d.Detect(storage, int64(len(tt.values)))
			state := d.series[holtStateKey{ref: 0, agg: observer.AggregateAverage}]
			require.NotNil(t, state)
			require.True(t, state.warmedUp)
			assert.InDelta(t, tt.wantLevel, state.level, math.SmallestNonzeroFloat64)
			assert.InDelta(t, tt.wantTrend, state.trend, math.SmallestNonzeroFloat64)
			assert.Equal(t, tt.warmup, state.warmupCount)
		})
	}
}
