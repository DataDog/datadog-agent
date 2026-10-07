// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package bench

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	recorderparquet "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/impl/parquet"
)

func TestRecorderParquetV1LoadsInBothTestbenchModes(t *testing.T) {
	dir := t.TempDir()
	cfg := recorderdef.WriterConfig{OutputDir: dir, FlushInterval: time.Hour}
	metricWriter, err := (recorderparquet.Factory{}).NewMetricWriter(cfg)
	require.NoError(t, err)
	logWriter, err := (recorderparquet.Factory{}).NewLogWriter(cfg)
	require.NoError(t, err)
	require.True(t, metricWriter.WriteMetric(recorderdef.MetricData{
		Source: "check", Name: "system.cpu", Value: 2.5, Timestamp: 1000,
		Tags: []string{"host:test", "env:dev"},
	}))
	require.True(t, metricWriter.WriteMetric(recorderdef.MetricData{
		Source: "check", Name: "system.cpu", Value: 3.5, Timestamp: 1001,
		Tags: []string{"host:test"}, Dropped: true,
	}))
	require.True(t, logWriter.WriteLog(recorderdef.LogData{
		Source: "logs", TimestampMs: 1000500, Content: []byte("first"),
		Status: "info", Hostname: "test-host", Tags: []string{"service:api"},
	}))
	require.True(t, logWriter.WriteLog(recorderdef.LogData{
		Source: "logs", TimestampMs: 1001500, Content: []byte("second"),
		Status: "error", Hostname: "test-host", Tags: []string{"service:api"},
	}))
	require.NoError(t, metricWriter.Close())
	require.NoError(t, logWriter.Close())
	require.Equal(t, FormatV1, detectParquetFormat(dir))

	metrics, err := readAllMetrics(dir)
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Equal(t, "check", metrics[0].Source)
	require.Equal(t, "system.cpu", metrics[0].Name)
	require.Equal(t, 2.5, metrics[0].Value)
	require.Equal(t, int64(1000), metrics[0].Timestamp)
	require.False(t, metrics[0].Dropped)
	require.ElementsMatch(t, []string{"host:test", "env:dev"}, metrics[0].Tags)
	require.Equal(t, "check", metrics[1].Source)
	require.Equal(t, "system.cpu", metrics[1].Name)
	require.Equal(t, 3.5, metrics[1].Value)
	require.Equal(t, int64(1001), metrics[1].Timestamp)
	require.Equal(t, []string{"host:test"}, metrics[1].Tags)
	require.True(t, metrics[1].Dropped)

	logs, err := readAllLogs(dir)
	require.NoError(t, err)
	require.Equal(t, []recorderdef.LogData{
		{Source: "logs", TimestampMs: 1000500, Content: []byte("first"), Status: "info", Hostname: "test-host", Tags: []string{"service:api"}},
		{Source: "logs", TimestampMs: 1001500, Content: []byte("second"), Status: "error", Hostname: "test-host", Tags: []string{"service:api"}},
	}, logs)

	var streamedMetrics []recorderdef.MetricData
	var streamedLogs []recorderdef.LogData
	metricCount, logCount, err := streamOrderedObservations(dir, FormatV1, false, func(observation parquetObservation) error {
		if observation.metric != nil {
			streamedMetrics = append(streamedMetrics, *observation.metric)
		} else {
			streamedLogs = append(streamedLogs, *observation.log)
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, metricCount)
	require.Equal(t, 2, logCount)
	require.Len(t, streamedMetrics, 2)
	for i := range metrics {
		require.Equal(t, metrics[i].Source, streamedMetrics[i].Source)
		require.Equal(t, metrics[i].Name, streamedMetrics[i].Name)
		require.Equal(t, metrics[i].Value, streamedMetrics[i].Value)
		require.Equal(t, metrics[i].Timestamp, streamedMetrics[i].Timestamp)
		require.Equal(t, metrics[i].Dropped, streamedMetrics[i].Dropped)
		require.ElementsMatch(t, metrics[i].Tags, streamedMetrics[i].Tags)
	}
	require.Equal(t, logs, streamedLogs)
}
