// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func TestCheckRotationBufferedProbeRejectedWhenDirectNotApplied(t *testing.T) {
	directCfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               8,
		OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
	}
	oldFP := &types.Fingerprint{Value: 0x1111, Config: directCfg}
	newFP := &types.Fingerprint{Value: 0x2222, Config: directCfg}

	source := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              "/tmp/rotation-check.log",
		FingerprintConfig: directCfg,
	})
	file := NewFile("/tmp/rotation-check.log", source, false)
	tailer := &Tailer{file: file, fingerprint: oldFP}

	mock := NewFingerprinterMock()
	mock.SetFingerprintWithAppliedFlags("/tmp/rotation-check.log", newFP, directCfg, nil)

	check, err := tailer.CheckRotation(mock, SequentialRotationCheck())
	require.NoError(t, err)
	require.True(t, check.BufferedProbeRejected)
	require.False(t, check.Rotated)
}

func TestCheckRotationParallelOptsRotatesWithoutBufferedProbeReject(t *testing.T) {
	directCfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               8,
		OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
	}
	oldFP := &types.Fingerprint{Value: 0x1111, Config: directCfg}
	newFP := &types.Fingerprint{Value: 0x2222, Config: directCfg}

	source := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              "/tmp/rotation-check-parallel.log",
		FingerprintConfig: directCfg,
	})
	file := NewFile("/tmp/rotation-check-parallel.log", source, false)
	tailer := &Tailer{file: file, fingerprint: oldFP}

	mock := NewFingerprinterMock()
	mock.SetFingerprintWithAppliedFlags("/tmp/rotation-check-parallel.log", newFP, directCfg, nil)

	check, err := tailer.CheckRotation(mock, ParallelRotationCheck())
	require.NoError(t, err)
	require.False(t, check.BufferedProbeRejected)
	require.True(t, check.Rotated)
}

func TestCheckRotationAuthoritativeDirectCandidateRotates(t *testing.T) {
	directCfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               8,
		OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
	}
	oldFP := &types.Fingerprint{Value: 0x1111, Config: directCfg}
	newFP := &types.Fingerprint{Value: 0x2222, Config: directCfg}

	source := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              "/tmp/rotation-check-direct.log",
		FingerprintConfig: directCfg,
	})
	file := NewFile("/tmp/rotation-check-direct.log", source, false)
	tailer := &Tailer{file: file, fingerprint: oldFP}

	mock := NewFingerprinterMock()
	mock.SetFingerprintWithAppliedFlags("/tmp/rotation-check-direct.log", newFP, directCfg, []types.FileOpenFlag{types.FileOpenFlagDirect})

	check, err := tailer.CheckRotation(mock, SequentialRotationCheck())
	require.NoError(t, err)
	require.False(t, check.BufferedProbeRejected)
	require.True(t, check.Rotated)
	require.Equal(t, RotationFingerprintMismatch, check.Evidence.Method)
	require.True(t, check.Evidence.HasAuthoritativeDirectCandidate(directCfg))
}

func TestCheckRotationWithoutDirectStillRotatesOnMismatch(t *testing.T) {
	cfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               8,
	}
	oldFP := &types.Fingerprint{Value: 0x1111, Config: cfg}
	newFP := &types.Fingerprint{Value: 0x2222, Config: cfg}

	source := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              "/tmp/rotation-check-buffered.log",
		FingerprintConfig: cfg,
	})
	file := NewFile("/tmp/rotation-check-buffered.log", source, false)
	tailer := &Tailer{file: file, fingerprint: oldFP}

	mock := NewFingerprinterMock()
	mock.SetFingerprintWithAppliedFlags("/tmp/rotation-check-buffered.log", newFP, cfg, nil)

	check, err := tailer.CheckRotation(mock, SequentialRotationCheck())
	require.NoError(t, err)
	require.False(t, check.BufferedProbeRejected)
	require.True(t, check.Rotated)
}

func TestDidRotateViaFingerprintDelegatesToParallelCheckRotation(t *testing.T) {
	directCfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               8,
		OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
	}
	oldFP := &types.Fingerprint{Value: 0x1111, Config: directCfg}
	newFP := &types.Fingerprint{Value: 0x2222, Config: directCfg}

	source := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              "/tmp/did-rotate-parallel.log",
		FingerprintConfig: directCfg,
	})
	file := NewFile("/tmp/did-rotate-parallel.log", source, false)
	tailer := &Tailer{file: file, fingerprint: oldFP}

	mock := NewFingerprinterMock()
	mock.SetFingerprintWithAppliedFlags("/tmp/did-rotate-parallel.log", newFP, directCfg, nil)

	rotated, err := tailer.DidRotateViaFingerprint(mock)
	require.NoError(t, err)
	require.True(t, rotated, "parallel wrapper must keep rotating on fingerprint mismatch")
}

func TestCheckRotationEstaleError(t *testing.T) {
	withStaleFileHandleTestHook(t)

	directCfg := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyByteChecksum,
		Count:               8,
		OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
	}
	oldFP := &types.Fingerprint{Value: 0x1111, Config: directCfg}

	source := sources.NewLogSource("", &config.LogsConfig{
		Type:              config.FileType,
		Path:              "/tmp/rotation-check-estale.log",
		FingerprintConfig: directCfg,
	})
	file := NewFile("/tmp/rotation-check-estale.log", source, false)
	tailer := &Tailer{file: file, fingerprint: oldFP}

	mock := NewFingerprinterMock()
	mock.SetFingerprintError("/tmp/rotation-check-estale.log", ErrStaleFileHandleTest)

	_, err := tailer.CheckRotation(mock, SequentialRotationCheck())
	require.ErrorIs(t, err, ErrStaleFileHandleTest)
}

func withStaleFileHandleTestHook(t *testing.T) {
	t.Helper()
	cleanup := SetStaleFileHandleHookForTest(func(err error) bool {
		return errors.Is(err, ErrStaleFileHandleTest)
	})
	t.Cleanup(cleanup)
}
