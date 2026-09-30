// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package parquet

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/require"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

func TestSameSecondFlushesPreserveEveryBatch(t *testing.T) {
	dir := t.TempDir()
	instant := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	metric, err := newMetricParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	metric.now = func() time.Time { return instant }
	log, err := newLogParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	log.now = func() time.Time { return instant }

	for _, name := range []string{"first", "second"} {
		require.True(t, metric.WriteMetric(recorder.MetricData{Name: name}))
		metric.flush()
		require.True(t, log.WriteLog(recorder.LogData{Content: []byte(name)}))
		log.flush()
	}
	require.NoError(t, metric.Close())
	require.NoError(t, log.Close())

	for i, suffix := range []string{"", "_000000001"} {
		name := []string{"first", "second"}[i]
		readOneBatch(t, filepath.Join(dir, "observer-metrics-20260102-030405Z"+suffix+".parquet"), func(rec arrow.RecordBatch) {
			require.Equal(t, int64(1), rec.NumRows())
			require.Equal(t, name, rec.Column(2).(*array.String).Value(0))
		})
		readOneBatch(t, filepath.Join(dir, "observer-logs-20260102-030405Z"+suffix+".parquet"), func(rec arrow.RecordBatch) {
			require.Equal(t, int64(1), rec.NumRows())
			require.Equal(t, []byte(name), rec.Column(2).(*array.Binary).Value(0))
		})
	}
}
