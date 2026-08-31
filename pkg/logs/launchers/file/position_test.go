// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

type positionFingerprinter struct {
	file.Fingerprinter
	config *types.FingerprintConfig
	reads  int
	err    error
}

func (f *positionFingerprinter) ComputeFingerprintFromConfig(path string, config *types.FingerprintConfig) (*types.Fingerprint, error) {
	f.reads++
	if config != nil {
		captured := *config
		captured.OpenFlags = append([]types.FileOpenFlag(nil), config.OpenFlags...)
		f.config = &captured
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.Fingerprinter.ComputeFingerprintFromConfig(path, config)
}

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

	offset, whence, err = Position(registry, "", config.End, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	offset, whence, err = Position(registry, "", config.Beginning, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.End, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(123456789), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.Beginning, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(987654321), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "foo")
	offset, whence, err = Position(registry, "test", config.End, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	registry.SetOffset("test", "bar")
	offset, whence, err = Position(registry, "test", config.Beginning, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "123456789")
	offset, whence, err = Position(registry, "test", config.ForceBeginning, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("test", "987654321")
	offset, whence, err = Position(registry, "test", config.ForceEnd, mockFingerprinter, opener.NewFileOpener(), nil)
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)
}

func TestPositionUsesCurrentOpenFlagsWithStoredFingerprintConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	require.NoError(t, os.WriteFile(path, make([]byte, 512), 0o600))
	identifier := "file:" + path

	storedConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               7,
		CountToSkip:         3,
		MaxBytes:            99,
		Source:              types.FingerprintConfigSourceGlobal,
	}
	storedFingerprint := &types.Fingerprint{Value: 12345, Config: storedConfig}
	currentFingerprint := &types.Fingerprint{
		Value: 12345,
		Config: &types.FingerprintConfig{
			FingerprintStrategy: types.FingerprintStrategyByteChecksum,
			Count:               2048,
			OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
			Source:              types.FingerprintConfigSourcePerSource,
		},
	}

	registry := auditorMock.NewMockRegistry()
	registry.SetOffset(identifier, "128")
	registry.SetFingerprint(storedFingerprint)
	mockFingerprinter := file.NewFingerprinterMock()
	mockFingerprinter.SetFingerprint(path, &types.Fingerprint{Value: storedFingerprint.Value, Config: storedConfig})
	fingerprinter := &positionFingerprinter{Fingerprinter: mockFingerprinter}

	offset, whence, err := Position(registry, identifier, config.Beginning, fingerprinter, opener.NewFileOpener(), currentFingerprint)
	require.NoError(t, err)
	require.EqualValues(t, 128, offset)
	require.Equal(t, io.SeekStart, whence)

	usedConfig := fingerprinter.config
	require.NotNil(t, usedConfig)
	require.Equal(t, storedConfig.FingerprintStrategy, usedConfig.FingerprintStrategy)
	require.Equal(t, storedConfig.Count, usedConfig.Count)
	require.Equal(t, storedConfig.CountToSkip, usedConfig.CountToSkip)
	require.Equal(t, storedConfig.MaxBytes, usedConfig.MaxBytes)
	require.Equal(t, storedConfig.Source, usedConfig.Source)
	require.Equal(t, []types.FileOpenFlag{types.FileOpenFlagDirect}, usedConfig.OpenFlags)
	require.Empty(t, registry.GetFingerprint(identifier).Config.OpenFlags, "the persisted config must not be mutated")

	if runtime.GOOS != "linux" {
		t.Skip("open_flags fingerprint failures use the dedicated error path on Linux only")
	}

	fingerprintErr := errors.New("direct I/O rejected")
	fingerprinter.err = fingerprintErr
	_, _, err = Position(registry, identifier, config.Beginning, fingerprinter, opener.NewFileOpener(), currentFingerprint)
	require.ErrorIs(t, err, fingerprintErr)
}

// TestPositionReusesLauncherFingerprint covers the read the launcher has already
// paid for before it asks where to start. Repeating it costs more than a syscall:
// it is a second chance to catch the file mid-rotation, and under O_DIRECT a
// second uncached read of the head of every file on every restart.
func TestPositionReusesLauncherFingerprint(t *testing.T) {
	storedConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               7,
		CountToSkip:         3,
		MaxBytes:            99,
		Source:              types.FingerprintConfigSourceGlobal,
	}

	// activeConfig hashes exactly the same bytes as storedConfig; only the fields
	// that cannot change the value differ. That is the steady state on every
	// restart once open_flags is configured, so it is the case that has to avoid
	// the second read.
	activeConfig := *storedConfig
	activeConfig.OpenFlags = []types.FileOpenFlag{types.FileOpenFlagDirect}
	activeConfig.Source = types.FingerprintConfigSourcePerSource

	differentConfig := activeConfig
	differentConfig.Count = 9

	tests := []struct {
		name      string
		current   *types.Fingerprint
		wantReads int
	}{
		{
			name:      "same checksum parameters reuse the launcher value",
			current:   &types.Fingerprint{Value: 12345, Config: &activeConfig},
			wantReads: 0,
		},
		{
			// Value 0 means no read happened and only the configuration was
			// filled in for the status page. Reusing it would compare zero
			// against the stored value and report a rotation.
			name:      "invalid launcher value is read again",
			current:   &types.Fingerprint{Value: types.InvalidFingerprintValue, Config: &activeConfig},
			wantReads: 1,
		},
		{
			name:      "different checksum parameters are read again",
			current:   &types.Fingerprint{Value: 12345, Config: &differentConfig},
			wantReads: 1,
		},
		{
			name:      "no launcher fingerprint is read again",
			current:   nil,
			wantReads: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.log")
			require.NoError(t, os.WriteFile(path, make([]byte, 512), 0o600))
			identifier := "file:" + path

			registry := auditorMock.NewMockRegistry()
			registry.SetOffset(identifier, "128")
			registry.SetFingerprint(&types.Fingerprint{Value: 12345, Config: storedConfig})

			mockFingerprinter := file.NewFingerprinterMock()
			mockFingerprinter.SetFingerprint(path, &types.Fingerprint{Value: 12345, Config: storedConfig})
			fingerprinter := &positionFingerprinter{Fingerprinter: mockFingerprinter}

			offset, whence, err := Position(registry, identifier, config.Beginning, fingerprinter, opener.NewFileOpener(), tt.current)
			require.NoError(t, err)
			require.Equal(t, tt.wantReads, fingerprinter.reads)

			// Every case above describes the same unrotated file, so the stored
			// offset has to survive whichever path was taken to confirm it.
			require.EqualValues(t, 128, offset)
			require.Equal(t, io.SeekStart, whence)
		})
	}
}

// TestPositionOffsetBeyondFileSize covers recovery from a stored offset that points past the end of
// the file, which happens when a file is rotated or truncated while no tailer is watching it (for
// example across an Agent restart). Reusing such an offset makes the tailer seek past EOF and read
// nothing, leaving the source permanently at "Bytes Read: 0".
//
// A running tailer is protected by the fileSize < lastReadOffset check in DidRotate(), but a
// brand-new tailer is not. The gap is unrecoverable when fingerprinting is enabled, because the
// launcher then consults DidRotateViaFingerprint() instead of DidRotate(), and that reports "no
// rotation" for a file whose head content is unchanged.
func TestPositionOffsetBeyondFileSize(t *testing.T) {
	// Fingerprinter with the shipped default (fingerprinting disabled globally).
	fingerprinter := file.NewFingerprinter(
		types.FingerprintConfig{FingerprintStrategy: types.FingerprintStrategyDisabled},
		opener.NewFileOpener(),
	)

	// disabledFingerprint is the entry shape observed in the field: a real offset paired with a
	// fingerprint that can only ever recompute to Value 0, so Equals() trivially reports "aligned".
	disabledFingerprint := func() *types.Fingerprint {
		return &types.Fingerprint{
			Value: 0,
			Config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyDisabled,
				Count:               1,
				MaxBytes:            2048,
			},
		}
	}

	// setup writes a file of the given size and returns its registry identifier.
	setup := func(t *testing.T, size int) (identifier, path string) {
		t.Helper()
		path = filepath.Join(t.TempDir(), "app.log")
		content := make([]byte, size)
		for i := range content {
			content[i] = 'a'
		}
		require.NoError(t, os.WriteFile(path, content, 0o644))
		return "file:" + path, path
	}

	tests := []struct {
		name           string
		fileSize       int
		storedOffset   int64
		storedFP       *types.Fingerprint
		mode           config.TailingMode
		expectedOffset int64
		expectedWhence int
	}{
		{
			// The reported failure: the file rotated to a much smaller one while the Agent was
			// stopped, so the committed offset is now past EOF.
			name:           "offset past EOF with no stored fingerprint restarts from beginning",
			fileSize:       78,
			storedOffset:   9600,
			storedFP:       nil,
			mode:           config.End,
			expectedOffset: 0,
			expectedWhence: io.SeekStart,
		},
		{
			// Same, but with the weak stored fingerprint written before fingerprint_config was added.
			// This is the case that never self-heals, since the fingerprint rotation check reports
			// "no rotation" for a file whose head is unchanged.
			name:           "offset past EOF with disabled stored fingerprint restarts from beginning",
			fileSize:       78,
			storedOffset:   9600,
			storedFP:       disabledFingerprint(),
			mode:           config.End,
			expectedOffset: 0,
			expectedWhence: io.SeekStart,
		},
		{
			// start_position: beginning must not be able to preserve an out-of-range offset either.
			name:           "offset past EOF with beginning mode restarts from beginning",
			fileSize:       78,
			storedOffset:   9600,
			storedFP:       disabledFingerprint(),
			mode:           config.Beginning,
			expectedOffset: 0,
			expectedWhence: io.SeekStart,
		},
		{
			// A tailer that read the file to EOF has offset == size. That is a valid resume point:
			// resetting here would re-send the whole file on every Agent restart.
			name:           "offset exactly at EOF resumes from the stored offset",
			fileSize:       500,
			storedOffset:   500,
			storedFP:       disabledFingerprint(),
			mode:           config.End,
			expectedOffset: 500,
			expectedWhence: io.SeekStart,
		},
		{
			// Normal steady state: unread bytes remain in the file.
			name:           "offset within the file resumes from the stored offset",
			fileSize:       500,
			storedOffset:   250,
			storedFP:       disabledFingerprint(),
			mode:           config.End,
			expectedOffset: 250,
			expectedWhence: io.SeekStart,
		},
		{
			// An explicit override still wins over the out-of-range offset handling.
			name:           "force end wins over offset past EOF",
			fileSize:       78,
			storedOffset:   9600,
			storedFP:       disabledFingerprint(),
			mode:           config.ForceEnd,
			expectedOffset: 0,
			expectedWhence: io.SeekEnd,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			identifier, _ := setup(t, tc.fileSize)

			registry := auditorMock.NewMockRegistry()
			registry.SetOffset(identifier, strconv.FormatInt(tc.storedOffset, 10))
			if tc.storedFP != nil {
				registry.SetFingerprint(tc.storedFP)
			}

			offset, whence, err := Position(registry, identifier, tc.mode, fingerprinter, opener.NewFileOpener(), nil)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOffset, offset)
			assert.Equal(t, tc.expectedWhence, whence)
		})
	}
}
