// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"io"
	"testing"

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

	// Create a mock fingerprinter
	mockFingerprinter := file.NewFingerprinterMock()

	// Set a fingerprint in the registry
	fingerprint := &types.Fingerprint{
		Value: 12345,
		Config: &types.FingerprintConfig{
			MaxBytes:            maxBytes,
			Count:               maxLines,
			CountToSkip:         toSkip,
			FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		},
	}
	registry.SetFingerprint(fingerprint)
	mockFingerprinter.SetFingerprint("test", fingerprint)
	mockFingerprinter.SetInvalidFingerprint("")

	offset, whence, err = Position(registry, "", config.End, mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	offset, whence, err = Position(registry, "", config.Beginning, mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.End, mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(123456789), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.Beginning, mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(987654321), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "foo")
	offset, whence, err = Position(registry, "test", config.End, mockFingerprinter)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	registry.SetOffset("test", "bar")
	offset, whence, err = Position(registry, "test", config.Beginning, mockFingerprinter)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.ForceBeginning, mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.ForceEnd, mockFingerprinter)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)
}

// TestPositionFingerprintAlignment covers the fingerprint-comparison branches that TestPosition
// never reaches (its identifiers are too short to yield a non-empty file path). It uses an
// identifier longer than the 5-char prefix so Position derives a file path and consults the
// fingerprinter.
func TestPositionFingerprintAlignment(t *testing.T) {
	const identifier = "file:abc" // len > 5, so filePath = "abc"
	cfg := &types.FingerprintConfig{
		MaxBytes:            2048,
		Count:               1,
		CountToSkip:         0,
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
	}

	t.Run("fingerprints align: stored offset is used", func(t *testing.T) {
		registry := auditorMock.NewMockRegistry()
		fp := file.NewFingerprinterMock()
		registry.SetOffset(identifier, "555")
		registry.SetFingerprint(&types.Fingerprint{Value: 12345, Config: cfg})
		fp.SetFingerprint("abc", &types.Fingerprint{Value: 12345, Config: cfg})

		offset, whence, err := Position(registry, identifier, config.End, fp)
		assert.Nil(t, err)
		assert.Equal(t, int64(555), offset)
		assert.Equal(t, io.SeekStart, whence)
	})

	t.Run("fingerprints differ: restart from beginning despite stored offset", func(t *testing.T) {
		registry := auditorMock.NewMockRegistry()
		fp := file.NewFingerprinterMock()
		registry.SetOffset(identifier, "555")
		registry.SetFingerprint(&types.Fingerprint{Value: 12345, Config: cfg})
		fp.SetFingerprint("abc", &types.Fingerprint{Value: 99999, Config: cfg})

		offset, whence, err := Position(registry, identifier, config.End, fp)
		assert.Nil(t, err)
		assert.Equal(t, int64(0), offset)
		assert.Equal(t, io.SeekStart, whence)
	})

	t.Run("no previous fingerprint: stored offset is used", func(t *testing.T) {
		registry := auditorMock.NewMockRegistry()
		fp := file.NewFingerprinterMock()
		registry.SetOffset(identifier, "777")
		// No registry fingerprint set => GetFingerprint returns nil => alignment is assumed.

		offset, whence, err := Position(registry, identifier, config.End, fp)
		assert.Nil(t, err)
		assert.Equal(t, int64(777), offset)
		assert.Equal(t, io.SeekStart, whence)
	})
}
