// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContextFile(t *testing.T) *contextFile {
	t.Helper()
	cf, err := newContextFile(filepath.Join(t.TempDir(), "contexts.bin"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cf.close() })
	return cf
}

func TestContextFileMaybeWriteIdempotent(t *testing.T) {
	cf := newTestContextFile(t)

	require.NoError(t, cf.maybeWrite(1, "foo", []string{"env:prod"}))
	require.NoError(t, cf.maybeWrite(1, "foo", []string{"env:prod"})) // second call: bloom hit
	require.NoError(t, cf.maybeWrite(1, "foo", []string{"env:prod"})) // third call: bloom hit

	// Only one entry should be on disk.
	entries, err := cf.scan("foo", nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "foo", entries[1].name)
}

func TestContextFileScanNilTagsMatchAll(t *testing.T) {
	cf := newTestContextFile(t)

	require.NoError(t, cf.maybeWrite(1, "m", []string{"env:prod"}))
	require.NoError(t, cf.maybeWrite(2, "m", []string{"env:staging"}))
	require.NoError(t, cf.maybeWrite(3, "other", []string{"env:prod"}))

	entries, err := cf.scan("m", nil)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	_, ok1 := entries[1]
	_, ok2 := entries[2]
	assert.True(t, ok1)
	assert.True(t, ok2)
}

func TestContextFileScanTagFilter(t *testing.T) {
	cf := newTestContextFile(t)

	require.NoError(t, cf.maybeWrite(1, "m", []string{"env:prod", "region:us"}))
	require.NoError(t, cf.maybeWrite(2, "m", []string{"env:staging"}))

	entries, err := cf.scan("m", []string{"env:prod"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	e, ok := entries[1]
	require.True(t, ok)
	assert.Equal(t, "m", e.name)
}

func TestContextFileRoundTripAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contexts.bin")

	// Write two entries and close.
	cf, err := newContextFile(path)
	require.NoError(t, err)
	require.NoError(t, cf.maybeWrite(10, "metric.a", []string{"host:h1"}))
	require.NoError(t, cf.maybeWrite(20, "metric.b", nil))
	require.NoError(t, cf.close())

	// Reopen: bloom should be repopulated from file.
	cf2, err := newContextFile(path)
	require.NoError(t, err)
	defer cf2.close()

	// Known keys should not be re-written (bloom already set).
	assert.True(t, cf2.bloom.IsKnown(10))
	assert.True(t, cf2.bloom.IsKnown(20))

	// Scan should return both entries.
	entries, err := cf2.scan("metric.a", nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, []string{"host:h1"}, entries[10].tags)
}

func TestContextFileConcurrentWrites(t *testing.T) {
	cf := newTestContextFile(t)

	const n = 100
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := uint64(i)
			_ = cf.maybeWrite(key, "concurrent", []string{})
		}(i)
	}
	wg.Wait()

	entries, err := cf.scan("concurrent", nil)
	require.NoError(t, err)
	assert.Len(t, entries, n)
}

func TestContextFileSyntheticKeyConsistency(t *testing.T) {
	k1 := syntheticKey("foo", sortedTagsCopy([]string{"b", "a"}))
	k2 := syntheticKey("foo", sortedTagsCopy([]string{"a", "b"}))
	assert.Equal(t, k1, k2, "synthetic key must be tag-order independent")
	assert.NotZero(t, k1)
}
