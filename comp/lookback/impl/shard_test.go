// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestShard(t *testing.T) *shard {
	t.Helper()
	dir := t.TempDir()
	s, err := newShard(dir, 1000, 64*1024)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.stop() })
	return s
}

func TestShardWriteRotateRead(t *testing.T) {
	s := newTestShard(t)

	const n = 100
	s.mu.Lock()
	for i := range n {
		err := s.writeRecord(record{contextKey: 1, tsUs: int64(i) * 1_000_000, value: float64(i)})
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
		assert.Equal(t, int64(i)*1_000_000, r.tsUs)
	}
}

func TestShardBufferFlushOnFull(t *testing.T) {
	dir := t.TempDir()
	s, err := newShard(dir, 1000, recordSize*3)
	require.NoError(t, err)
	defer s.stop()

	s.mu.Lock()
	for range 4 {
		require.NoError(t, s.writeRecord(record{contextKey: 1, tsUs: 100, value: 1.0}))
	}
	s.mu.Unlock()

	info, err := os.Stat(filepath.Join(dir, "1000.wal"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, info.Size(), int64(3*recordSize))
}

func TestShardConcurrentWriteRotate(t *testing.T) {
	dir := t.TempDir()
	s, err := newShard(dir, 1000, 64*1024)
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
				_ = s.writeRecord(record{contextKey: 1, tsUs: int64(base*1000 + j), value: 1.0})
				s.mu.Unlock()
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.rotate(2000)
	}()
	wg.Wait()

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
