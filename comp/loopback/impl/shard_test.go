// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loopbackimpl

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestShard(t *testing.T) (*shard, *contextRegistry) {
	t.Helper()
	reg := newContextRegistry()
	reg.registerWithKey(1, "m1", []string{"env:test"})
	dir := t.TempDir()
	s, err := newShard(dir, 1000, 64*1024, reg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.stop() })
	return s, reg
}

func TestShardWriteRotateRead(t *testing.T) {
	s, _ := newTestShard(t)

	const n = 100
	s.mu.Lock()
	for i := range n {
		err := s.writeRecord(record{contextKey: 1, tsNs: int64(i) * 1_000_000_000, value: float64(i)})
		require.NoError(t, err)
	}
	s.mu.Unlock()

	require.NoError(t, s.rotate(2000))

	files, err := s.sealedFiles()
	require.NoError(t, err)
	require.Len(t, files, 1)

	recs, err := readRecordsFromFile(files[0].path)
	require.NoError(t, err)
	require.Len(t, recs, n)
	for i, r := range recs {
		assert.Equal(t, uint64(1), r.contextKey)
		assert.Equal(t, int64(i)*1_000_000_000, r.tsNs)
	}
}

func TestShardBufferFlushOnFull(t *testing.T) {
	reg := newContextRegistry()
	dir := t.TempDir()
	// Buffer fits exactly 3 records.
	s, err := newShard(dir, 1000, recordSize*3, reg)
	require.NoError(t, err)
	defer s.stop()

	s.mu.Lock()
	for range 4 {
		require.NoError(t, s.writeRecord(record{contextKey: 1, tsNs: 100, value: 1.0}))
	}
	s.mu.Unlock()

	info, err := os.Stat(filepath.Join(dir, "1000.wal"))
	require.NoError(t, err)
	// At least 3 records have been flushed to disk.
	assert.GreaterOrEqual(t, info.Size(), int64(3*recordSize))
}

func TestShardConcurrentWriteRotate(t *testing.T) {
	reg := newContextRegistry()
	dir := t.TempDir()
	s, err := newShard(dir, 1000, 64*1024, reg)
	require.NoError(t, err)
	defer s.stop()

	const goroutines = 50
	const recordsEach = 40

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := range recordsEach {
				s.mu.Lock()
				_ = s.writeRecord(record{contextKey: 1, tsNs: int64(base*1000 + j), value: 1.0})
				s.mu.Unlock()
			}
		}(i)
	}

	// Trigger a rotation mid-flight.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.rotate(2000)
	}()
	wg.Wait()

	// Final rotation to seal the second window.
	require.NoError(t, s.rotate(3000))

	files, err := s.sealedFiles()
	require.NoError(t, err)

	total := 0
	for _, f := range files {
		recs, err := readRecordsFromFile(f.path)
		require.NoError(t, err)
		total += len(recs)
	}
	assert.Equal(t, goroutines*recordsEach, total)
}

func TestParseWALFilename(t *testing.T) {
	ws, err := parseWALFilename("1715000000.wal")
	require.NoError(t, err)
	assert.Equal(t, int64(1715000000), ws)

	_, err = parseWALFilename("not-a-number.wal")
	assert.Error(t, err)
}

func TestShardCatalogPersistReload(t *testing.T) {
	reg := newContextRegistry()
	dir := t.TempDir()
	s, err := newShard(dir, 1000, 64*1024, reg)
	require.NoError(t, err)

	// Register a context key via write path.
	s.mu.Lock()
	require.NoError(t, s.maybeRegisterKey(42, "mymetric", []string{"env:prod"}))
	s.mu.Unlock()
	require.NoError(t, s.stop())

	// Simulate restart: new registry, reopen shard.
	reg2 := newContextRegistry()
	s2, err := newShard(dir, 1000, 64*1024, reg2)
	require.NoError(t, err)
	defer s2.stop()

	// The registry should have been restored from the catalog file.
	keys := reg2.lookupKeys("mymetric", nil)
	require.Len(t, keys, 1)
	assert.Equal(t, uint64(42), keys[0])
}
