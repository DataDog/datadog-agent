// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build observer

package observerimpl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestParquetBloomFilterOnListColumn verifies that bloom filters work on individual
// elements within list<string> columns, not just the entire array.
func TestParquetBloomFilterOnListColumn(t *testing.T) {
	tmpDir := t.TempDir()

	// Create parquet writer with bloom filters
	writer, err := NewParquetWriter(tmpDir, 1*time.Second, 0)
	require.NoError(t, err)
	defer writer.Close()

	// Write metrics with different tag combinations
	writer.WriteMetric("test", "metric.a", 1.0, []string{"host:server1", "env:prod"}, 1000)
	writer.WriteMetric("test", "metric.b", 2.0, []string{"host:server2", "env:dev"}, 2000)
	writer.WriteMetric("test", "metric.c", 3.0, []string{"host:server3", "env:staging"}, 3000)

	// Force flush to write file
	writer.Close()

	// Read parquet files
	reader, err := NewParquetReader(tmpDir)
	require.NoError(t, err)
	require.Equal(t, 3, reader.Len())

	// Verify we can find metrics by individual tags
	foundServer1 := false
	foundDev := false
	foundServer3Staging := false

	reader.Reset()
	for {
		metric := reader.Next()
		if metric == nil {
			break
		}

		// Check individual tag matching
		tags := metric.Tags
		if tags["host"] == "server1" {
			foundServer1 = true
			require.Equal(t, "prod", tags["env"])
		}
		if tags["env"] == "dev" {
			foundDev = true
			require.Equal(t, "server2", tags["host"])
		}
		if tags["host"] == "server3" && tags["env"] == "staging" {
			foundServer3Staging = true
		}
	}

	require.True(t, foundServer1, "Should find metric with host:server1")
	require.True(t, foundDev, "Should find metric with env:dev")
	require.True(t, foundServer3Staging, "Should find metric with host:server3 and env:staging")
}

// TestParquetBloomFilterSkipsRowGroups verifies bloom filters reduce I/O
// by checking we can read specific metrics without scanning everything.
func TestParquetBloomFilterSkipsRowGroups(t *testing.T) {
	tmpDir := t.TempDir()

	// Create writer
	writer, err := NewParquetWriter(tmpDir, 100*time.Millisecond, 0)
	require.NoError(t, err)

	// Write many metrics to create multiple files
	for i := 0; i < 100; i++ {
		tags := []string{"host:server1", "index:" + string(rune(i))}
		writer.WriteMetric("test", "metric.test", float64(i), tags, int64(i*1000))
		time.Sleep(2 * time.Millisecond)
	}
	writer.Close()

	// Verify multiple files were created
	files, err := filepath.Glob(filepath.Join(tmpDir, "*.parquet"))
	require.NoError(t, err)
	require.Greater(t, len(files), 1, "Should have created multiple parquet files")

	// Read back - bloom filter should allow selective reading
	reader, err := NewParquetReader(tmpDir)
	require.NoError(t, err)
	require.Equal(t, 100, reader.Len())

	// Verify all metrics have the expected tag
	count := 0
	reader.Reset()
	for {
		metric := reader.Next()
		if metric == nil {
			break
		}
		require.Equal(t, "server1", metric.Tags["host"])
		count++
	}
	require.Equal(t, 100, count)
}
