// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diskretry

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func testPayload(data string, count int) *message.Payload {
	return makeTestPayload([]byte(data), "gzip", len(data), count)
}

func TestNewDiskRetryManager(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)
	defer m.Stop()

	assert.Equal(t, dir, m.storagePath)
	assert.Equal(t, int64(10*1024*1024), m.maxSizeBytes)
}

func TestNewDiskRetryManagerEnforcesMinSize(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 100, 0.8, 7) // 100 bytes < minSizeBytes
	require.NoError(t, err)
	defer m.Stop()

	assert.Equal(t, int64(minSizeBytes), m.maxSizeBytes)
}

func TestNewDiskRetryManagerCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", "retry")
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)
	defer m.Stop()

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestStoreWritesFile(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)
	defer m.Stop()

	payload := testPayload("test-data", 3)
	err = m.Store(payload)
	require.NoError(t, err)

	m.mu.Lock()
	assert.Equal(t, 1, len(m.filenames))
	assert.True(t, m.currentSize > 0)
	m.mu.Unlock()

	// Verify file exists on disk
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	retryFiles := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == retryFileExtension {
			retryFiles++
		}
	}
	assert.Equal(t, 1, retryFiles)
}

func TestStoreMultiplePayloads(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)
	defer m.Stop()

	for i := 0; i < 5; i++ {
		err = m.Store(testPayload("payload-data", 1))
		require.NoError(t, err)
	}

	m.mu.Lock()
	assert.Equal(t, 5, len(m.filenames))
	m.mu.Unlock()
}

func TestStoreRejectsWhenAtCapacity(t *testing.T) {
	dir := t.TempDir()
	// Small capacity -- use minSizeBytes since that's the minimum enforced
	m, err := NewDiskRetryManager(dir, minSizeBytes, 0.8, 7)
	require.NoError(t, err)
	defer m.Stop()

	// Write a payload that's close to the capacity
	bigData := make([]byte, minSizeBytes-100) // leaves very little room
	payload := makeTestPayload(bigData, "gzip", len(bigData), 1)
	err = m.Store(payload)
	require.NoError(t, err)

	// Next write should fail (exceeds maxSizeBytes)
	err = m.Store(payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disk retry queue full")
}

func TestReloadExistingFiles(t *testing.T) {
	dir := t.TempDir()

	// Write some retry files manually
	payload := testPayload("existing-data", 2)
	for i := 0; i < 3; i++ {
		data, err := SerializePayload(payload)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, filepath.Base(t.TempDir())+retryFileExtension), data, 0600)
		require.NoError(t, err)
	}

	// Create manager -- should reload existing files
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)
	defer m.Stop()

	m.mu.Lock()
	assert.Equal(t, 3, len(m.filenames))
	assert.True(t, m.currentSize > 0)
	m.mu.Unlock()
}

func TestReplayLoopSendsAndDeletes(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)

	// Store a payload
	err = m.Store(testPayload("replay-me", 1))
	require.NoError(t, err)

	// Start replay with a send function that always succeeds
	var received atomic.Int32
	m.StartReplayLoop(func(payload *message.Payload) bool {
		received.Add(1)
		return true
	})

	// Wait for replay
	assert.Eventually(t, func() bool {
		return received.Load() >= 1
	}, 5*time.Second, 100*time.Millisecond)

	m.Stop()

	// File should be deleted
	m.mu.Lock()
	assert.Equal(t, 0, len(m.filenames))
	m.mu.Unlock()

	entries, _ := os.ReadDir(dir)
	retryFiles := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == retryFileExtension {
			retryFiles++
		}
	}
	assert.Equal(t, 0, retryFiles)
}

func TestReplayLoopRetriesOnSendFailure(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)

	err = m.Store(testPayload("retry-me", 1))
	require.NoError(t, err)

	// Send function that fails 3 times then succeeds
	var attempts atomic.Int32
	m.StartReplayLoop(func(payload *message.Payload) bool {
		n := attempts.Add(1)
		return n > 3
	})

	assert.Eventually(t, func() bool {
		return attempts.Load() > 3
	}, 30*time.Second, 100*time.Millisecond)

	m.Stop()

	// File should be deleted after successful send
	m.mu.Lock()
	assert.Equal(t, 0, len(m.filenames))
	m.mu.Unlock()
}

func TestReplayLoopFIFOOrder(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)

	// Store 3 payloads with identifiable data
	for i := 0; i < 3; i++ {
		err = m.Store(testPayload(string(rune('A'+i)), 1))
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // ensure distinct timestamps
	}

	var order []string
	m.StartReplayLoop(func(payload *message.Payload) bool {
		order = append(order, string(payload.Encoded))
		return true
	})

	assert.Eventually(t, func() bool {
		return len(order) >= 3
	}, 5*time.Second, 100*time.Millisecond)

	m.Stop()

	assert.Equal(t, []string{"A", "B", "C"}, order)
}

func TestStopTerminatesReplayLoop(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 7)
	require.NoError(t, err)

	m.StartReplayLoop(func(_ *message.Payload) bool { return true })

	// Stop should return promptly even if no files to replay
	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return in time")
	}
}

func TestNoopRetrierAlwaysFails(t *testing.T) {
	r := NewNoopRetrier()
	err := r.Store(testPayload("data", 1))
	assert.Error(t, err)
	// Stop is safe to call
	r.Stop()
}

func TestExpireTTL(t *testing.T) {
	dir := t.TempDir()
	m, err := NewDiskRetryManager(dir, 10*1024*1024, 0.8, 0) // 0 TTL = no expiry
	require.NoError(t, err)
	defer m.Stop()

	err = m.Store(testPayload("data", 1))
	require.NoError(t, err)

	// With TTL 0, expiry doesn't run
	m.mu.Lock()
	m.expireTTLLocked()
	assert.Equal(t, 1, len(m.filenames))
	m.mu.Unlock()

	// Now set TTL and backdate the file
	m.fileTTLDays = 1
	// Touch the file to the past
	oldTime := time.Now().Add(-48 * time.Hour)
	m.mu.Lock()
	os.Chtimes(m.filenames[0], oldTime, oldTime)
	m.expireTTLLocked()
	assert.Equal(t, 0, len(m.filenames))
	m.mu.Unlock()
}
