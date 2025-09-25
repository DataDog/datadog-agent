// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"io"
	"path/filepath"
	"testing"
	"os"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func TestPosition(t *testing.T) {
	registry := auditorMock.NewMockRegistry()

	var err error
	var offset int64
	var whence int
	maxLines := 1
	maxBytes := 2048
	toSkip := 0
	fingerprintConfig := &types.FingerprintConfig{
		MaxBytes:            maxBytes,
		Count:               maxLines,
		CountToSkip:         toSkip,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	// Create a mock fingerprinter
	mockFingerprinter := file.NewFingerprinter(*fingerprintConfig)

	// Set a fingerprint in the registry
	fingerprint := &types.Fingerprint{
		Value:  12345,
		Config: fingerprintConfig,
	}
	registry.SetFingerprint(fingerprint)

	offset, whence, err = Position(registry, "", config.End, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	offset, whence, err = Position(registry, "", config.Beginning, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.End, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(123456789), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.Beginning, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(987654321), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "foo")
	offset, whence, err = Position(registry, "test", config.End, *mockFingerprinter)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	registry.SetOffset("test", "bar")
	offset, whence, err = Position(registry, "test", config.Beginning, *mockFingerprinter)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.ForceBeginning, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.ForceEnd, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)
}

// TestPositionFingerprintRotation tests position logic when fingerprints don't align (rotation scenario)
func TestPositionFingerprintRotation(t *testing.T) {
	registry := auditorMock.NewMockRegistry()

	fingerprintConfig := &types.FingerprintConfig{
		MaxBytes:            2048,
		Count:               1,
		CountToSkip:         0,
		FingerprintStrategy: types.FingerprintStrategyDisabled, // Simplify for testing
	}

	// Create a mock fingerprinter
	mockFingerprinter := file.NewFingerprinter(*fingerprintConfig)

	// Test when there's a stored offset but no fingerprint logic to worry about
	identifier := "file:/path/to/rotated.log"
	registry.SetOffset(identifier, "1000") // Had read 1000 bytes from original file

	// Test with End mode - should use stored offset since fingerprinting is disabled
	offset, whence, err := Position(registry, identifier, config.End, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(1000), offset)
	assert.Equal(t, io.SeekStart, whence)

	// Test with Beginning mode
	offset, whence, err = Position(registry, identifier, config.Beginning, *mockFingerprinter)
	assert.Nil(t, err)
	// Should still use stored offset
	assert.Equal(t, int64(1000), offset)
	assert.Equal(t, io.SeekStart, whence)
}

// When fingerprints don't align (rotation detected), should start from beginning
func TestPositionFingerprintMismatch(t *testing.T) {
	registry := auditorMock.NewMockRegistry()

	fingerprintConfig := &types.FingerprintConfig{
		MaxBytes:            2048,
		Count:               1,
		CountToSkip:         0,
		FingerprintStrategy: types.FingerprintStrategyByteChecksum, // Enable fingerprinting
	}

	// Create temporary files with different content to generate different fingerprints
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.log")

	// Create first file with content
	err := os.WriteFile(testFile, []byte("original content"), 0644)
	assert.Nil(t, err)

	mockFingerprinter := file.NewFingerprinter(*fingerprintConfig)

	// 1. File has stored offset (was being tailed)
	identifier := "file:" + testFile
	registry.SetOffset(identifier, "500")

	// 2. Set a previous fingerprint (simulating old file content)
	prevFingerprint := &types.Fingerprint{
		Value:  12345, // Different from what the new file will generate
		Config: fingerprintConfig,
	}
	registry.SetFingerprint(prevFingerprint)

	// 3. Overwrite file with different content (simulating rotation)
	err = os.WriteFile(testFile, []byte("rotated content - completely different"), 0644)
	assert.Nil(t, err)

	offset, whence, _ := Position(registry, identifier, config.End, *mockFingerprinter)

	// Fingerprints don't align and there's an offset, should start from beginning (rotation handling)
	assert.Equal(t, int64(0), offset, "Should start from offset 0")
	assert.Equal(t, io.SeekStart, whence, "Should seek to start (beginning) when rotation detected")

	// Test with Beginning mode too - should also work
	offset, whence, _ = Position(registry, identifier, config.Beginning, *mockFingerprinter)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)
}

// TestPositionFingerprintMatch tests position logic when fingerprints DO match (no rotation)
func TestPositionFingerprintMatch(t *testing.T) {
	registry := auditorMock.NewMockRegistry()

	fingerprintConfig := &types.FingerprintConfig{
		MaxBytes:            2048,
		Count:               1,
		CountToSkip:         0,
		FingerprintStrategy: types.FingerprintStrategyDisabled, // Disable fingerprinting
	}

	mockFingerprinter := file.NewFingerprinter(*fingerprintConfig)

	// When fingerprinting is disabled, fingerprintsAlign should be true
	identifier := "file:/any/path.log"
	registry.SetOffset(identifier, "1500")

	offset, whence, err := Position(registry, identifier, config.End, *mockFingerprinter)
	assert.Nil(t, err)

	// Should use stored offset since fingerprints "align"
	assert.Equal(t, int64(1500), offset)
	assert.Equal(t, io.SeekStart, whence)
}

// TestPositionNoStoredOffset tests behavior when there's no stored offset
func TestPositionNoStoredOffset(t *testing.T) {
	registry := auditorMock.NewMockRegistry()

	fingerprintConfig := &types.FingerprintConfig{
		MaxBytes:            2048,
		Count:               1,
		CountToSkip:         0,
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
	}

	mockFingerprinter := file.NewFingerprinter(*fingerprintConfig)

	// No stored offset for this identifier
	identifier := "file:/new/file.log"

	// Test different modes when no offset is stored
	offset, whence, err := Position(registry, identifier, config.End, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	offset, whence, err = Position(registry, identifier, config.Beginning, *mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)
}
