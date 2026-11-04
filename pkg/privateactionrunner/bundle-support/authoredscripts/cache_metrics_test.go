// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
)

func TestCacheSize(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	require.NoError(t, os.Mkdir(staging, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "package"), []byte("abc"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "download"), []byte("12345"), 0o600))
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(outside, []byte("excluded"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))
	size, err := cacheSize(context.Background(), root)
	require.NoError(t, err)
	require.Equal(t, int64(8), size)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = cacheSize(ctx, root)
	require.ErrorIs(t, err, context.Canceled)
}

type cacheMetricsClient struct {
	statsd.NoOpClient
	values []float64
}

func (c *cacheMetricsClient) Gauge(_ string, value float64, _ []string, _ float64) error {
	c.values = append(c.values, value)
	return nil
}

func TestCacheMetricsLifecycle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	synctest.Test(t, func(t *testing.T) {
		client := &cacheMetricsClient{}
		stop := startCacheMetrics(context.Background(), root, client)
		defer stop(context.Background())
		synctest.Wait()
		require.Equal(t, []float64{0}, client.values, "a missing cache reports zero")
		require.NoError(t, os.Mkdir(root, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "package"), []byte("abc"), 0o600))
		time.Sleep(cacheMetricsInterval)
		synctest.Wait()
		require.Equal(t, []float64{0, 3}, client.values)
		require.NoError(t, os.WriteFile(filepath.Join(root, "package"), []byte("abcdef"), 0o600))
		require.NoError(t, stop(context.Background()))
		require.Equal(t, []float64{0, 3, 6}, client.values, "shutdown captures the final cache size")
		time.Sleep(cacheMetricsInterval)
		require.Equal(t, []float64{0, 3, 6}, client.values, "collection stops before the metrics client is closed")
	})
}

func TestCacheMetricsScanFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	synctest.Test(t, func(t *testing.T) {
		client := &cacheMetricsClient{}
		stop := startCacheMetrics(context.Background(), filepath.Join(file, "cache"), client)
		defer stop(context.Background())
		synctest.Wait()
		require.Empty(t, client.values, "a failed scan must not publish a partial size")
	})
}
