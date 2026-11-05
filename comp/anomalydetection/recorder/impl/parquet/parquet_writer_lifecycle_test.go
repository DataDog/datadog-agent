// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package parquet

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/stretchr/testify/require"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

func TestEmptyFlushAndCloseCreateNoFiles(t *testing.T) {
	dir := t.TempDir()
	metric, err := newMetricParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	log, err := newLogParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	metric.flush()
	log.flush()
	require.NoError(t, metric.Close())
	require.NoError(t, log.Close())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestConcurrentWritesAndClose(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		create func(string) (func() bool, func() error, error)
	}{
		{
			name: "metrics", prefix: "observer-metrics",
			create: func(dir string) (func() bool, func() error, error) {
				w, err := newMetricParquetWriter(dir, time.Hour, 0)
				if err != nil {
					return nil, nil, err
				}
				return func() bool { return w.WriteMetric(recorder.MetricData{Name: "test"}) }, w.Close, nil
			},
		},
		{
			name: "logs", prefix: "observer-logs",
			create: func(dir string) (func() bool, func() error, error) {
				w, err := newLogParquetWriter(dir, time.Hour, 0)
				if err != nil {
					return nil, nil, err
				}
				return func() bool { return w.WriteLog(recorder.LogData{Content: []byte("test")}) }, w.Close, nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write, closeWriter, err := tc.create(dir)
			require.NoError(t, err)
			require.True(t, write())
			var accepted atomic.Int64
			accepted.Store(1)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < 64; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					if write() {
						accepted.Add(1)
					}
				}()
			}
			closeResults := make(chan error, 4)
			for i := 0; i < cap(closeResults); i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					closeResults <- closeWriter()
				}()
			}
			close(start)
			wg.Wait()
			close(closeResults)
			for err := range closeResults {
				require.NoError(t, err)
			}
			require.False(t, write())
			require.Equal(t, accepted.Load(), parquetRowCount(t, dir, tc.prefix))
		})
	}
}

func TestFinalFlushErrorIsStableAcrossCloseCalls(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recordings")
	w, err := newMetricParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	require.True(t, w.WriteMetric(recorder.MetricData{Name: "test"}))
	require.NoError(t, os.RemoveAll(dir))
	first := w.Close()
	require.ErrorContains(t, first, "final flush")
	require.EqualError(t, w.Close(), first.Error())
	require.False(t, w.WriteMetric(recorder.MetricData{Name: "late"}))
}

type failingOutput struct {
	*os.File
	err    error
	closed bool
}

func (f *failingOutput) Write([]byte) (int, error) {
	return 0, f.err
}

func (f *failingOutput) Close() error {
	f.closed = true
	return f.File.Close()
}

func TestWriteFailureClosesAndDoesNotPublishParquetFile(t *testing.T) {
	dir := t.TempDir()
	w, err := newMetricParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	failure := errors.New("disk full")
	var output *failingOutput
	w.openFile = func(path string) (io.WriteCloser, error) {
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			return nil, err
		}
		output = &failingOutput{File: file, err: failure}
		return output, nil
	}
	require.True(t, w.WriteMetric(recorder.MetricData{Name: "test"}))
	require.ErrorIs(t, w.Close(), failure)
	require.NotNil(t, output)
	require.True(t, output.closed)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func parquetRowCount(t *testing.T, dir, prefix string) int64 {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, prefix+"-*.parquet"))
	require.NoError(t, err)
	var rows int64
	for _, path := range paths {
		f, err := os.Open(path)
		require.NoError(t, err)
		pf, err := file.NewParquetReader(f)
		require.NoError(t, err)
		rows += pf.MetaData().NumRows
		require.NoError(t, pf.Close())
	}
	return rows
}
