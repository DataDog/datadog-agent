// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tempfile

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewWithContent_WritesContentToFile verifies that a file created with NewWithContent
// contains the exact bytes that were provided.
func TestNewWithContent_WritesContentToFile(t *testing.T) {
	content := []byte(`{"key":"value"}`)
	tf, err := NewWithContent("test-*.json", content)
	require.NoError(t, err)
	defer tf.CloseSafely()

	// Seek to start before reading — the file cursor is at the end after writing.
	_, err = tf.File.Seek(0, 0)
	require.NoError(t, err)

	data, err := tf.ReadBytes()
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

// TestReadJSON_ParsesValidJSON verifies that ReadJSON correctly decodes a JSON object.
func TestReadJSON_ParsesValidJSON(t *testing.T) {
	tf, err := NewWithContent("test-*.json", []byte(`{"name":"runner","count":42}`))
	require.NoError(t, err)
	defer tf.CloseSafely()

	_, err = tf.File.Seek(0, 0)
	require.NoError(t, err)

	result, err := tf.ReadJSON()
	require.NoError(t, err)

	m, ok := result.(map[string]interface{})
	require.True(t, ok, "ReadJSON must return a map for a JSON object")
	assert.Equal(t, "runner", m["name"])
	assert.Equal(t, float64(42), m["count"])
}

// TestReadJSON_InvalidJSON verifies that ReadJSON returns an error for malformed JSON.
func TestReadJSON_InvalidJSON(t *testing.T) {
	tf, err := NewWithContent("test-*.txt", []byte(`not { json`))
	require.NoError(t, err)
	defer tf.CloseSafely()

	_, err = tf.File.Seek(0, 0)
	require.NoError(t, err)

	_, err = tf.ReadJSON()
	assert.Error(t, err, "malformed JSON must return an error")
}

// TestReadBytes_ReturnsAllBytes verifies that ReadBytes returns every byte written.
func TestReadBytes_ReturnsAllBytes(t *testing.T) {
	payload := []byte("binary\x00data\xff")
	tf, err := NewWithContent("test-*.bin", payload)
	require.NoError(t, err)
	defer tf.CloseSafely()

	_, err = tf.File.Seek(0, 0)
	require.NoError(t, err)

	data, err := tf.ReadBytes()
	require.NoError(t, err)
	assert.Equal(t, payload, data)
}

// TestClose_DeletesFileFromDisk verifies that calling Close removes the underlying file.
// This is the key cleanup guarantee: no temp files should linger after the caller is done.
func TestClose_DeletesFileFromDisk(t *testing.T) {
	tf, err := New("test-*.tmp")
	require.NoError(t, err)
	name := tf.File.Name()

	// File must exist before Close.
	_, statErr := os.Stat(name)
	require.NoError(t, statErr, "file must exist before Close")

	require.NoError(t, tf.Close())

	_, statErr = os.Stat(name)
	assert.True(t, os.IsNotExist(statErr), "file must not exist after Close")
}

// TestCloseSafely_DoesNotPanicOnValidFile verifies that calling CloseSafely on an open
// file succeeds silently and removes the file from disk.
func TestCloseSafely_DoesNotPanicOnValidFile(t *testing.T) {
	tf, err := New("test-*.tmp")
	require.NoError(t, err)
	name := tf.File.Name()

	// Must not panic.
	assert.NotPanics(t, func() { tf.CloseSafely() })

	_, statErr := os.Stat(name)
	assert.True(t, os.IsNotExist(statErr), "file must be deleted by CloseSafely")
}

// TestCloseSafely_DoesNotPanicAfterAlreadyClosed verifies that calling CloseSafely on an
// already-closed/removed file does not panic. CloseSafely is intended to be safe to call
// even in error paths and double-close scenarios.
func TestCloseSafely_DoesNotPanicAfterAlreadyClosed(t *testing.T) {
	tf, err := New("test-*.tmp")
	require.NoError(t, err)

	tf.CloseSafely()

	// Second call must not panic even though the file is gone.
	assert.NotPanics(t, func() { tf.CloseSafely() })
}
