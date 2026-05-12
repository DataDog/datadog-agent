// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loopbackimpl

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	loopback "github.com/DataDog/datadog-agent/comp/loopback/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestStore creates a shardedStore backed by a temp directory.
func buildTestStore(t *testing.T, cfg storeConfig) (*shardedStore, *contextRegistry) {
	t.Helper()
	reg := newContextRegistry()
	store, err := newShardedStore(cfg, reg, nil)
	require.NoError(t, err)
	store.startRotationTimer()
	t.Cleanup(func() { _ = store.stop(context.Background()) })
	return store, reg
}

func defaultTestCfg(t *testing.T) storeConfig {
	return storeConfig{
		baseDir:        t.TempDir(),
		numShards:      4,
		windowDuration: 10 * time.Second,
		maxAge:         24 * time.Hour,
		maxDiskBytes:   100 * 1024 * 1024,
		maxBufSize:     64 * 1024,
	}
}

func TestNoopComponentReturnsErrDisabled(t *testing.T) {
	n := &noopComponent{}
	_, err := n.Flush(context.Background(), "m", nil, 0, 1, time.Second)
	assert.True(t, errors.Is(err, loopback.ErrDisabled))
}

func TestEndToEndWriteRotateFlush(t *testing.T) {
	cfg := defaultTestCfg(t)
	store, reg := buildTestStore(t, cfg)

	const metricName = "latency.p99"
	tags := []string{"env:prod"}
	ck := reg.register(metricName, tags)

	// Timestamps must fall within the current WAL window (opened at ~now).
	// Write 5 samples starting at now+1s so they are clearly within [now, now+windowDuration).
	baseNs := time.Now().Add(1 * time.Second).UnixNano()

	for i := range 5 {
		store.writeWithName(ck, baseNs+int64(i)*int64(time.Second), float64(i+1), metricName, tags)
	}

	// Seal the current window by rotating forward past windowDuration.
	newWindow := time.Now().Unix() + int64(cfg.windowDuration.Seconds()) + 1
	require.NoError(t, store.rotateAll(newWindow))

	comp := &component{store: store, reg: reg}
	// Query from now (before baseNs) to cover all 5 records.
	start := time.Now().UnixNano()
	stop := baseNs + 5*int64(time.Second) + int64(time.Second)
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
	store, reg := buildTestStore(t, cfg)

	baseNs := time.Now().Add(1 * time.Second).UnixNano()
	ck1 := reg.register("counter", []string{"env:prod"})
	ck2 := reg.register("counter", []string{"env:staging"})
	store.writeWithName(ck1, baseNs, 1.0, "counter", []string{"env:prod"})
	store.writeWithName(ck2, baseNs, 2.0, "counter", []string{"env:staging"})

	require.NoError(t, store.rotateAll(time.Now().Unix()+int64(cfg.windowDuration.Seconds())+1))

	comp := &component{store: store, reg: reg}
	start := time.Now().UnixNano()
	stop := baseNs + int64(time.Second)*2
	buckets, err := comp.Flush(context.Background(), "counter", nil, start, stop, time.Second)
	require.NoError(t, err)
	require.Len(t, buckets, 2)
}

func TestFlushErrNoDataWhenNoFiles(t *testing.T) {
	cfg := defaultTestCfg(t)
	store, reg := buildTestStore(t, cfg)
	reg.registerWithKey(99, "ghost", nil)

	comp := &component{store: store, reg: reg}
	_, err := comp.Flush(context.Background(), "ghost", nil, 0, int64(time.Second), time.Second)
	assert.True(t, errors.Is(err, loopback.ErrNoData))
}

func TestRetentionAgeEnforcement(t *testing.T) {
	cfg := defaultTestCfg(t)
	cfg.maxAge = 60 * time.Second

	// Build store without starting the rotation timer so we control rotation manually.
	reg := newContextRegistry()
	store, err := newShardedStore(cfg, reg, nil)
	require.NoError(t, err)
	defer store.stop(context.Background())

	// For each shard: discard the current (empty) active file, create an old-timestamped
	// active file, write a record, then rotate to the current time so the old file is sealed.
	pastWindow := int64(1) // 1970 — far older than maxAge=60s
	for _, s := range store.shards {
		s.mu.Lock()
		// Close and remove the just-opened (empty) active file.
		_ = s.activeF.Close()
		_ = os.Remove(s.activeF.Name())

		// Open an active file at pastWindow.
		s.windowStart = pastWindow
		require.NoError(t, s.openActiveFile())
		require.NoError(t, s.writeRecord(record{contextKey: 1, tsNs: 1_000_000_000, value: 1.0}))
		require.NoError(t, s.flushLocked())
		require.NoError(t, s.activeF.Sync())
		require.NoError(t, s.activeF.Close())

		// Switch to a current active file; the old one is now sealed.
		s.windowStart = time.Now().Unix()
		require.NoError(t, s.openActiveFile())
		s.mu.Unlock()
	}

	// Confirm old sealed files are visible before retention.
	for _, s := range store.shards {
		files, err := s.sealedFiles()
		require.NoError(t, err)
		require.NotEmpty(t, files, "expected old sealed files before retention")
	}

	require.NoError(t, store.enforceRetention())

	// All old files should now be gone.
	for _, s := range store.shards {
		files, err := s.sealedFiles()
		require.NoError(t, err)
		assert.Empty(t, files, "old sealed files should be deleted after retention enforcement")
	}
}
