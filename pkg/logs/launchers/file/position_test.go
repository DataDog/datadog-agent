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
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
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

	offset, whence, err = Position(registry, "", config.End, mockFingerprinter, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	offset, whence, err = Position(registry, "", config.Beginning, mockFingerprinter, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.End, mockFingerprinter, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(123456789), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.Beginning, mockFingerprinter, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(987654321), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "foo")
	offset, whence, err = Position(registry, "test", config.End, mockFingerprinter, nil)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	registry.SetOffset("test", "bar")
	offset, whence, err = Position(registry, "test", config.Beginning, mockFingerprinter, nil)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.ForceBeginning, mockFingerprinter, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.ForceEnd, mockFingerprinter, nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)
}

// TestPositionFingerprintAlignment covers how Position handles the fingerprint stored in the
// registry, and in particular the AGENT-16624 self-heal path: a registry entry whose stored
// fingerprint cannot produce a real value (Value 0, e.g. it predates the source's
// fingerprint_config or was written with a disabled strategy) must not be trusted to validate a
// saved offset once the source has active fingerprinting, otherwise a stale offset left behind by a
// missed log rotation is reused forever (permanent "Bytes Read: 0").
func TestPositionFingerprintAlignment(t *testing.T) {
	const path = "/var/log/app.log"
	source := sources.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})
	testFile := file.NewFile(path, source, false)
	identifier := testFile.Identifier() // "file:/var/log/app.log"
	const staleOffset = "9444967"        // multi-MB offset left over from before a rotation

	// disabledFingerprint mimics the customer's registry entry: a real offset paired with a
	// fingerprint whose config can only ever recompute to Value 0.
	disabledFingerprint := func() *types.Fingerprint {
		return &types.Fingerprint{
			Value:  0,
			Config: &types.FingerprintConfig{FingerprintStrategy: types.FingerprintStrategyDisabled, Count: 1, MaxBytes: 2048},
		}
	}
	validFingerprint := func(v uint64) *types.Fingerprint {
		return &types.Fingerprint{
			Value:  v,
			Config: &types.FingerprintConfig{FingerprintStrategy: types.FingerprintStrategyLineChecksum, Count: 1, MaxBytes: 2048},
		}
	}

	t.Run("weak stored fingerprint self-heals when the source now fingerprints", func(t *testing.T) {
		registry := auditorMock.NewMockRegistry()
		registry.SetOffset(identifier, staleOffset)
		registry.SetFingerprint(disabledFingerprint())

		fp := file.NewFingerprinterMock()
		fp.SetShouldFileFingerprint(testFile, true) // source gained fingerprint_config

		// Even with mode End, the unverifiable stale offset must be discarded in favor of the
		// beginning of the (rotated) file.
		offset, whence, err := Position(registry, identifier, config.End, fp, testFile)
		assert.Nil(t, err)
		assert.Equal(t, int64(0), offset, "should restart from the beginning, not the stale offset")
		assert.Equal(t, io.SeekStart, whence)
	})

	t.Run("weak stored fingerprint keeps offset when fingerprinting is disabled", func(t *testing.T) {
		registry := auditorMock.NewMockRegistry()
		registry.SetOffset(identifier, staleOffset)
		registry.SetFingerprint(disabledFingerprint())

		fp := file.NewFingerprinterMock()
		fp.SetShouldFileFingerprint(testFile, false) // fingerprinting genuinely off (the default)

		// No fingerprinting means no rotation detection: resume from the stored offset exactly as
		// before, so we do not re-read files from the top on every restart.
		offset, whence, err := Position(registry, identifier, config.End, fp, testFile)
		assert.Nil(t, err)
		assert.Equal(t, int64(9444967), offset)
		assert.Equal(t, io.SeekStart, whence)
	})

	t.Run("valid stored fingerprint that still matches keeps the offset", func(t *testing.T) {
		registry := auditorMock.NewMockRegistry()
		registry.SetOffset(identifier, "500")
		registry.SetFingerprint(validFingerprint(12345))

		fp := file.NewFingerprinterMock()
		fp.SetFingerprint(path, validFingerprint(12345)) // recompute matches the stored value

		offset, whence, err := Position(registry, identifier, config.End, fp, testFile)
		assert.Nil(t, err)
		assert.Equal(t, int64(500), offset)
		assert.Equal(t, io.SeekStart, whence)
	})

	t.Run("valid stored fingerprint that no longer matches restarts from beginning", func(t *testing.T) {
		registry := auditorMock.NewMockRegistry()
		registry.SetOffset(identifier, "500")
		registry.SetFingerprint(validFingerprint(12345))

		fp := file.NewFingerprinterMock()
		fp.SetFingerprint(path, validFingerprint(99999)) // rotation: head content changed

		offset, whence, err := Position(registry, identifier, config.End, fp, testFile)
		assert.Nil(t, err)
		assert.Equal(t, int64(0), offset)
		assert.Equal(t, io.SeekStart, whence)
	})
}
