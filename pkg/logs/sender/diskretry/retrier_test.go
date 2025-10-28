// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diskretry

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAndReadPayload(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "diskretry_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create retrier
	config := Config{
		Enabled:      true,
		Path:         tmpDir,
		MaxSizeBytes: 1024 * 1024, // 1MB
	}
	retrier, err := NewRetrier(config)
	require.NoError(t, err)

	// Create test payloads
	payload1 := message.NewPayload(
		[]*message.MessageMetadata{{Hostname: "test1"}},
		[]byte("test payload 1"),
		"",
		14,
	)

	payload2 := message.NewPayload(
		[]*message.MessageMetadata{{Hostname: "test2"}},
		[]byte("test payload 2"),
		"",
		14,
	)

	// Write payloads to disk
	written1, err := retrier.WritePayloadToDisk(payload1)
	require.NoError(t, err)
	assert.True(t, written1)

	// Delay to ensure different modification times
	time.Sleep(100 * time.Millisecond)

	written2, err := retrier.WritePayloadToDisk(payload2)
	require.NoError(t, err)
	assert.True(t, written2)

	// Verify files were created
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 2, "Should have 2 retry files")

	// Read payloads back (should be in order)
	readPayload1 := retrier.readOldestPayload()
	require.NotNil(t, readPayload1)
	assert.Equal(t, "test payload 1", string(readPayload1.Encoded))

	readPayload2 := retrier.readOldestPayload()
	require.NotNil(t, readPayload2)
	assert.Equal(t, "test payload 2", string(readPayload2.Encoded))

	// Verify files were deleted
	files, err = os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 0, "All retry files should be deleted after reading")

	// No more payloads
	readPayload3 := retrier.readOldestPayload()
	assert.Nil(t, readPayload3)
}

func TestMaxSizeLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskretry_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create retrier with small max size - test that it enforces limits
	config := Config{
		Enabled:      true,
		Path:         tmpDir,
		MaxSizeBytes: 500, // Enough for a few small payloads
	}
	retrier, err := NewRetrier(config)
	require.NoError(t, err)

	// Write payloads until we hit the limit
	successCount := 0
	for i := 0; i < 10; i++ {
		payload := message.NewPayload(
			[]*message.MessageMetadata{{Hostname: "test"}},
			[]byte("test data"),
			"",
			9,
		)
		written, err := retrier.WritePayloadToDisk(payload)
		if err != nil {
			// Should eventually hit the limit
			assert.Contains(t, err.Error(), "disk retry space full")
			break
		}
		if written {
			successCount++
		}
	}

	// Should have written at least one but not all
	assert.Greater(t, successCount, 0, "Should write at least one payload")
	assert.Less(t, successCount, 10, "Should hit limit before writing all payloads")
}

func TestDisabledRetrier(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskretry_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create disabled retrier
	config := Config{
		Enabled:      false,
		Path:         tmpDir,
		MaxSizeBytes: 1024 * 1024,
	}
	retrier, err := NewRetrier(config)
	require.NoError(t, err)

	payload := message.NewPayload(
		[]*message.MessageMetadata{{Hostname: "test"}},
		[]byte("test payload"),
		"",
		12,
	)

	// Write should return false but no error
	written, err := retrier.WritePayloadToDisk(payload)
	require.NoError(t, err)
	assert.False(t, written)

	// No files should be created
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 0)
}

func TestAtomicWrites(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diskretry_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := Config{
		Enabled:      true,
		Path:         tmpDir,
		MaxSizeBytes: 1024 * 1024,
	}
	retrier, err := NewRetrier(config)
	require.NoError(t, err)

	payload := message.NewPayload(
		[]*message.MessageMetadata{{Hostname: "test"}},
		[]byte("test payload"),
		"",
		12,
	)

	written, err := retrier.WritePayloadToDisk(payload)
	require.NoError(t, err)
	assert.True(t, written)

	// Verify no .tmp files exist (atomic rename worked)
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.True(t, filepath.Ext(files[0].Name()) == ".json", "Should have .json extension, not .tmp")
}
