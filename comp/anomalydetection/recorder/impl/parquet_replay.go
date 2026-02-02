// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"context"
	"fmt"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ParquetReplayConfig configures the parquet replay behavior.
type ParquetReplayConfig struct {
	ParquetDir string  // directory containing FGM parquet files
	TimeScale  float64 // multiplier for time (1.0 = realtime, 0.25 = 4x faster)
	Loop       bool    // whether to loop the replay after reaching the end
}

// ParquetReplayGenerator replays metrics from FGM parquet files.
type ParquetReplayGenerator struct {
	handle observer.Handle
	reader *ParquetReader
	config ParquetReplayConfig
}

// NewParquetReplayGenerator creates a new parquet replay generator.
func NewParquetReplayGenerator(handle observer.Handle, config ParquetReplayConfig) (*ParquetReplayGenerator, error) {
	// Apply defaults
	if config.TimeScale <= 0 {
		config.TimeScale = 1.0
	}

	reader, err := NewParquetReader(config.ParquetDir)
	if err != nil {
		return nil, fmt.Errorf("creating parquet reader: %w", err)
	}

	fmt.Printf("[parquet-replay] Loaded %d metrics from %s\n", reader.Len(), config.ParquetDir)
	fmt.Printf("[parquet-replay] Time range: %d to %d (duration: %s)\n",
		reader.StartTime(), reader.EndTime(),
		time.Duration(reader.EndTime()-reader.StartTime())*time.Millisecond)
	fmt.Printf("[parquet-replay] TimeScale: %.2fx (%.2f = faster, 1.0 = realtime)\n",
		1.0/config.TimeScale, config.TimeScale)

	return &ParquetReplayGenerator{
		handle: handle,
		reader: reader,
		config: config,
	}, nil
}

// Run replays metrics from parquet files until the context is cancelled.
func (g *ParquetReplayGenerator) Run(ctx context.Context) {
	if g.reader.Len() == 0 {
		fmt.Println("[parquet-replay] No metrics to replay")
		return
	}

	for {
		if err := g.replayOnce(ctx); err != nil {
			if err == context.Canceled {
				return
			}
			fmt.Printf("[parquet-replay] Error: %v\n", err)
			return
		}

		if !g.config.Loop {
			fmt.Println("[parquet-replay] Replay complete")
			return
		}

		fmt.Println("[parquet-replay] Looping replay...")
		g.reader.Reset()
	}
}

// replayOnce replays all metrics once.
func (g *ParquetReplayGenerator) replayOnce(ctx context.Context) error {
	g.reader.Reset()

	startTime := time.Now()
	firstMetric := g.reader.Next()
	if firstMetric == nil {
		return nil
	}

	// Calculate the base offset for timestamps
	baseTimestampMS := firstMetric.Time
	g.reader.Reset() // Reset to replay from the beginning

	// Replay metrics
	var count int
	var lastProgressTime time.Time

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		metric := g.reader.Next()
		if metric == nil {
			break
		}

		// Calculate when this metric should be sent
		metricOffsetMS := metric.Time - baseTimestampMS
		targetTime := startTime.Add(time.Duration(float64(metricOffsetMS)*g.config.TimeScale) * time.Millisecond)

		// Wait until it's time to send this metric
		now := time.Now()
		if now.Before(targetTime) {
			sleepDuration := targetTime.Sub(now)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleepDuration):
			}
		}

		// Send the metric
		sample := &fgmMetricSample{
			metric: metric,
		}
		g.handle.ObserveMetric(sample)
		count++

		// Print progress every second
		if time.Since(lastProgressTime) > time.Second {
			elapsed := time.Since(startTime)
			progress := float64(metric.Time-baseTimestampMS) / float64(g.reader.EndTime()-baseTimestampMS) * 100
			fmt.Printf("[parquet-replay] Progress: %.1f%% (%d metrics, elapsed: %s)\n",
				progress, count, elapsed.Round(time.Second))
			lastProgressTime = time.Now()
		}
	}

	fmt.Printf("[parquet-replay] Replayed %d metrics\n", count)
	return nil
}

// fgmMetricSample implements observer.MetricView for FGM metrics.
type fgmMetricSample struct {
	metric *FGMMetric
}

func (s *fgmMetricSample) GetName() string {
	return s.metric.MetricName
}

func (s *fgmMetricSample) GetValue() float64 {
	if s.metric.ValueFloat != nil {
		return *s.metric.ValueFloat
	}
	if s.metric.ValueInt != nil {
		return float64(*s.metric.ValueInt)
	}
	return 0
}

func (s *fgmMetricSample) GetRawTags() []string {
	// Convert tag map to tag slice in format "key:value"
	tags := make([]string, 0, len(s.metric.Tags))
	for key, value := range s.metric.Tags {
		if value != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", key, value))
		}
	}
	return tags
}

func (s *fgmMetricSample) GetTimestamp() float64 {
	// Convert milliseconds to seconds (with fractional part)
	return float64(s.metric.Time) / 1000.0
}

func (s *fgmMetricSample) GetSampleRate() float64 {
	return 1.0
}
