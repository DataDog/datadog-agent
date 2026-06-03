// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	lookback "github.com/DataDog/datadog-agent/comp/lookback/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultTestCfg(t *testing.T) storeConfig {
	return storeConfig{
		baseDir:          t.TempDir(),
		numShards:        4,
		rotationInterval: 10 * time.Second,
		maxAge:           24 * time.Hour,
		maxDiskBytes:     100 * 1024 * 1024,
		maxBufSize:       64 * 1024,
	}
}

// buildTestComponent creates a component backed by a temp directory.
func buildTestComponent(t *testing.T, cfg storeConfig) (*component, *walBackend) {
	t.Helper()
	backend, err := newWALBackend(cfg, nil)
	require.NoError(t, err)
	backend.startRotationTimer()
	comp := &component{backend: backend}
	t.Cleanup(func() {
		_ = backend.stop(context.Background())
	})
	return comp, backend
}

func TestNoopComponentReturnsErrDisabled(t *testing.T) {
	n := &noopComponent{}
	_, err := n.Flush(context.Background(), "m", nil, 0, 1, time.Second)
	assert.True(t, errors.Is(err, lookback.ErrDisabled))
}

func TestEndToEndWriteRotateFlush(t *testing.T) {
	cfg := defaultTestCfg(t)
	comp, wb := buildTestComponent(t, cfg)

	const metricName = "latency.p99"
	tags := []string{"env:prod"}
	ck := syntheticKey(metricName, sortedTagsCopy(tags))

	require.NoError(t, wb.ctxFile.write(ck, metricName, tags))

	// Timestamps must fall within the current WAL window.
	baseUs := time.Now().Add(1 * time.Second).UnixMicro()
	for i := range 5 {
		wb.store.write(ck, baseUs+int64(i)*1_000_000, float64(i+1))
	}

	newWindow := time.Now().Unix() + int64(cfg.rotationInterval.Seconds()) + 1
	require.NoError(t, wb.store.rotateAll(newWindow))

	start := time.Now().UnixMicro()
	stop := baseUs + 5*1_000_000 + 1_000_000
	buckets, err := comp.Flush(context.Background(), metricName, tags, start, stop, time.Second)
	require.NoError(t, err)
	require.Len(t, buckets, 5)

	for i, b := range buckets {
		assert.Equal(t, metricName, b.Name)
		assert.Equal(t, int64(1), b.Count)
		assert.InDelta(t, float64(i+1), b.Sum, 1e-9)
	}
}

func TestFlushNilTagsMatchesAll(t *testing.T) {
	cfg := defaultTestCfg(t)
	comp, wb := buildTestComponent(t, cfg)

	baseUs := time.Now().Add(1 * time.Second).UnixMicro()
	ck1 := syntheticKey("counter", sortedTagsCopy([]string{"env:prod"}))
	ck2 := syntheticKey("counter", sortedTagsCopy([]string{"env:staging"}))

	require.NoError(t, wb.ctxFile.write(ck1, "counter", []string{"env:prod"}))
	require.NoError(t, wb.ctxFile.write(ck2, "counter", []string{"env:staging"}))

	wb.store.write(ck1, baseUs, 1.0)
	wb.store.write(ck2, baseUs, 2.0)

	require.NoError(t, wb.store.rotateAll(time.Now().Unix()+int64(cfg.rotationInterval.Seconds())+1))

	start := time.Now().UnixMicro()
	stop := baseUs + 2*1_000_000
	buckets, err := comp.Flush(context.Background(), "counter", nil, start, stop, time.Second)
	require.NoError(t, err)
	require.Len(t, buckets, 2)
}

func TestFlushErrNoDataWhenNoFiles(t *testing.T) {
	cfg := defaultTestCfg(t)
	comp, wb := buildTestComponent(t, cfg)

	require.NoError(t, wb.ctxFile.write(99, "ghost", nil))

	_, err := comp.Flush(context.Background(), "ghost", nil, 0, 1_000_000, time.Second)
	assert.True(t, errors.Is(err, lookback.ErrNoData))
}

func TestRetentionAgeEnforcement(t *testing.T) {
	cfg := defaultTestCfg(t)
	cfg.maxAge = 60 * time.Second

	store, err := newShardedStore(cfg, nil)
	require.NoError(t, err)
	defer store.stop(context.Background())

	pastWindow := int64(1) // 1970 — far older than maxAge=60s
	for _, s := range store.shards {
		s.mu.Lock()
		_ = s.activeF.Close()
		_ = os.Remove(s.activeF.Name())

		s.windowStart = pastWindow
		require.NoError(t, s.openActiveFile())
		require.NoError(t, s.writeRecord(record{contextKey: 1, tsUs: 1_000_000, value: 1.0}))
		require.NoError(t, s.flushLocked())
		require.NoError(t, s.activeF.Sync())
		require.NoError(t, s.activeF.Close())

		s.windowStart = time.Now().Unix()
		require.NoError(t, s.openActiveFile())
		s.mu.Unlock()
	}

	for _, s := range store.shards {
		files, err := s.sealedFiles()
		require.NoError(t, err)
		require.NotEmpty(t, files)
	}

	require.NoError(t, store.enforceRetention())

	for _, s := range store.shards {
		files, err := s.sealedFiles()
		require.NoError(t, err)
		assert.Empty(t, files)
	}
}
