// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Snapshot represents a serializable cache state for testing/debugging.
type Snapshot struct {
	Series []SeriesSnapshot `json:"series"`
}

// SeriesSnapshot represents a single metric series in a snapshot.
type SeriesSnapshot struct {
	Name     string              `json:"name"`
	Tags     []string            `json:"tags"`
	Type     int                 `json:"type"`
	LastSeen int64               `json:"last_seen"`
	Recent   []DataPointSnapshot `json:"recent"`
	Medium   []DataPointSnapshot `json:"medium"`
	Long     []DataPointSnapshot `json:"long"`
}

// DataPointSnapshot represents a single data point in a snapshot.
type DataPointSnapshot struct {
	Timestamp int64   `json:"ts"`
	Count     int64   `json:"count"`
	Sum       float64 `json:"sum"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
}

// CaptureSnapshot creates a snapshot from a cache.
func CaptureSnapshot(cache *MetricHistoryCache) *Snapshot {
	snapshot := &Snapshot{
		Series: make([]SeriesSnapshot, 0, len(cache.series)),
	}

	for _, history := range cache.series {
		series := SeriesSnapshot{
			Name:     history.Key.Name,
			Tags:     history.Key.Tags,
			Type:     int(history.Type),
			LastSeen: history.LastSeen,
			Recent:   convertDataPoints(history.Recent.ToSlice()),
			Medium:   convertDataPoints(history.Medium.ToSlice()),
			Long:     convertDataPoints(history.Long.ToSlice()),
		}
		snapshot.Series = append(snapshot.Series, series)
	}

	return snapshot
}

func convertDataPoints(points []DataPoint) []DataPointSnapshot {
	result := make([]DataPointSnapshot, len(points))
	for i, p := range points {
		result[i] = DataPointSnapshot{
			Timestamp: p.Timestamp,
			Count:     p.Stats.Count,
			Sum:       p.Stats.Sum,
			Min:       p.Stats.Min,
			Max:       p.Stats.Max,
		}
	}
	return result
}

// SaveSnapshot writes a snapshot to a file.
func SaveSnapshot(cache *MetricHistoryCache, path string) error {
	snapshot := CaptureSnapshot(cache)
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadSnapshot reads a snapshot from a file and reconstructs a cache.
func LoadSnapshot(path string) (*MetricHistoryCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}

	return RestoreSnapshot(&snapshot), nil
}

// RestoreSnapshot creates a cache from a snapshot.
func RestoreSnapshot(snapshot *Snapshot) *MetricHistoryCache {
	cache := NewMetricHistoryCache()

	keygen := ckey.NewKeyGenerator()
	for _, s := range snapshot.Series {
		// Generate context key from name and tags
		contextKey := keygen.Generate(s.Name, "", tagset.NewHashingTagsAccumulatorWithTags(s.Tags))

		history := &MetricHistory{
			Key: SeriesKey{
				ContextKey: contextKey,
				Name:       s.Name,
				Tags:       s.Tags,
			},
			Type:     metrics.APIMetricType(s.Type),
			Recent:   restoreRingBuffer(s.Recent, cache.recentCapacity),
			Medium:   restoreRingBuffer(s.Medium, cache.mediumCapacity),
			Long:     restoreRingBuffer(s.Long, cache.longCapacity),
			LastSeen: s.LastSeen,
		}

		cache.series[contextKey] = history
	}

	return cache
}

func restoreRingBuffer(points []DataPointSnapshot, capacity int) *RingBuffer[DataPoint] {
	buffer := NewRingBuffer[DataPoint](capacity)
	for _, p := range points {
		buffer.Push(DataPoint{
			Timestamp: p.Timestamp,
			Stats: SummaryStats{
				Count: p.Count,
				Sum:   p.Sum,
				Min:   p.Min,
				Max:   p.Max,
			},
		})
	}
	return buffer
}

// StartSnapshotServer starts a simple HTTP server for capturing snapshots.
// Intended for local debugging only.
func StartSnapshotServer(cache *MetricHistoryCache, addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/snapshot", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := CaptureSnapshot(cache)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshot)
	})

	server := &http.Server{Addr: addr, Handler: mux}
	go server.ListenAndServe()
	return server
}
