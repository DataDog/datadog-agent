// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/testutils"
)

func setupLauncherWithMockFingerprinter(
	t *testing.T,
	handoffMode types.RotationHandoffMode,
	fingerprintConfig *types.FingerprintConfig,
) (*Launcher, *sources.LogSource, string, chan *message.Message, *filetailer.FingerprinterMock) {
	t.Helper()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.close_timeout", 2)

	testDir := t.TempDir()
	testPath := testDir + "/launcher.log"

	testFile, err := os.Create(testPath)
	require.NoError(t, err)
	_, err = testFile.WriteString("seed\n")
	require.NoError(t, err)
	require.NoError(t, testFile.Sync())
	require.NoError(t, testFile.Close())

	quiet := 2
	maxDrain := 30
	source := sources.NewLogSource("", &config.LogsConfig{
		Type:                          config.FileType,
		Path:                          testPath,
		RotationHandoffMode:           string(handoffMode),
		SequentialRotationQuietPeriod: &quiet,
		SequentialRotationMaxDrain:    &maxDrain,
		FingerprintConfig:             fingerprintConfig,
	})

	pipelineProvider := mock.NewMockProvider()
	outputChan := pipelineProvider.NextPipelineChan()
	drainDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-outputChan:
			case <-drainDone:
				return
			}
		}
	}()
	t.Cleanup(func() { close(drainDone) })

	mockFP := filetailer.NewFingerprinterMock()
	s := createLauncher(t, launcherTestOptions{fingerprintConfig: fingerprintConfig})
	s.fingerprinter = mockFP
	if handoffMode == types.RotationHandoffModeSequential {
		s.forceSequentialHandoffForTest = true
		s.globalHandoffSettings = types.RotationHandoffSettings{
			Mode:               types.RotationHandoffModeSequential,
			QuietPeriodSeconds: quiet,
			MaxDrainSeconds:    maxDrain,
			QuietPeriod:        time.Duration(quiet) * time.Second,
			MaxDrain:           time.Duration(maxDrain) * time.Second,
		}
	}
	s.pipelineProvider = pipelineProvider
	s.registry = auditorMock.NewMockRegistry()
	s.activeSources = []*sources.LogSource{source}
	status.Clear()
	status.InitStatus(mockConfig, testutils.CreateSources([]*sources.LogSource{source}))
	t.Cleanup(status.Clear)
	t.Cleanup(s.cleanup)

	return s, source, testPath, outputChan, mockFP
}

func sequentialDirectCfg() *types.FingerprintConfig {
	return &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            256,
		OpenFlags:           []types.FileOpenFlag{types.FileOpenFlagDirect},
	}
}

func startSequentialTailer(
	t *testing.T,
	s *Launcher,
	source *sources.LogSource,
	testPath string,
	mockFP *filetailer.FingerprinterMock,
	initialFP *types.Fingerprint,
	directCfg *types.FingerprintConfig,
) (string, *filetailer.Tailer) {
	t.Helper()
	mockFP.SetFingerprintWithAppliedFlags(testPath, initialFP, directCfg, []types.FileOpenFlag{types.FileOpenFlagDirect})
	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)
	require.Equal(t, 1, s.tailers.Count())
	scanKey := getScanKey(testPath, source)
	tailer, found := s.tailers.Get(scanKey)
	require.True(t, found)
	return scanKey, tailer
}

func triggerSequentialHandoff(
	t *testing.T,
	s *Launcher,
	testPath string,
	mockFP *filetailer.FingerprinterMock,
	candidateFP *types.Fingerprint,
	directCfg *types.FingerprintConfig,
) {
	t.Helper()
	mockFP.SetFingerprintWithAppliedFlags(testPath, candidateFP, directCfg, []types.FileOpenFlag{types.FileOpenFlagDirect})
	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)
	require.Equal(t, 0, s.tailers.Count())
	require.Len(t, s.rotatedTailers, 1)
}

func finishSequentialDrain(t *testing.T, s *Launcher) {
	t.Helper()
	require.NotEmpty(t, s.rotatedTailers)
	s.rotatedTailers[0].Stop()
}

func absPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	return abs
}

func withStaleFileHandleTestHook(t *testing.T) {
	t.Helper()
	cleanup := filetailer.SetStaleFileHandleHookForTest(func(err error) bool {
		return errors.Is(err, filetailer.ErrStaleFileHandleTest)
	})
	t.Cleanup(cleanup)
}

func TestParallelDirectFingerprintRotationUnchanged(t *testing.T) {
	directCfg := sequentialDirectCfg()
	s, source, testPath, _, mockFP := setupLauncherWithMockFingerprinter(t, types.RotationHandoffModeParallel, directCfg)

	initialFP := &types.Fingerprint{Value: 0xaaaa, Config: directCfg}
	scanKey, oldTailer := startSequentialTailer(t, s, source, testPath, mockFP, initialFP, directCfg)

	newFP := &types.Fingerprint{Value: 0xbbbb, Config: directCfg}
	mockFP.SetFingerprintWithAppliedFlags(testPath, newFP, directCfg, nil)

	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	newTailer, found := s.tailers.Get(scanKey)
	require.True(t, found)
	require.NotSame(t, oldTailer, newTailer, "parallel mode must replace the tailer on fingerprint mismatch")
	require.Len(t, s.rotatedTailers, 1)
	require.Same(t, oldTailer, s.rotatedTailers[0])

	state := s.pathHandoffStateForTest(testPath)
	require.Equal(t, 0, state.DrainingCount, "parallel mode must not use sequential handoff")
	require.False(t, state.ReplacementRequested)
}

func TestSequentialDirectBufferedProbeRejectedBlocksHandoff(t *testing.T) {
	directCfg := sequentialDirectCfg()
	s, source, testPath, _, mockFP := setupLauncherWithMockFingerprinter(t, types.RotationHandoffModeSequential, directCfg)

	initialFP := &types.Fingerprint{Value: 0xaaaa, Config: directCfg}
	scanKey, _ := startSequentialTailer(t, s, source, testPath, mockFP, initialFP, directCfg)

	newFP := &types.Fingerprint{Value: 0xbbbb, Config: directCfg}
	mockFP.SetFingerprintWithAppliedFlags(testPath, newFP, directCfg, nil)

	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	require.Equal(t, 1, s.tailers.Count(), "sequential handoff must not start when direct pathname probe falls back to buffered")
	_, found := s.tailers.Get(scanKey)
	require.True(t, found)
	require.Empty(t, s.rotatedTailers)

	probeStatus, _ := s.activeProbeStateForTest(scanKey)
	require.Equal(t, probeStatusBufferedProbeRejected, probeStatus)
}

func TestSequentialVerifiedReplacementStarts(t *testing.T) {
	directCfg := sequentialDirectCfg()
	s, source, testPath, _, mockFP := setupLauncherWithMockFingerprinter(t, types.RotationHandoffModeSequential, directCfg)

	initialFP := &types.Fingerprint{Value: 0xaaaa, Config: directCfg}
	scanKey, _ := startSequentialTailer(t, s, source, testPath, mockFP, initialFP, directCfg)

	candidateFP := &types.Fingerprint{Value: 0xbbbb, Config: directCfg}
	triggerSequentialHandoff(t, s, testPath, mockFP, candidateFP, directCfg)

	mockFP.ResetCallCounts()
	mockFP.SetHandleFingerprint(absPath(t, testPath), candidateFP)
	finishSequentialDrain(t, s)

	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	require.Equal(t, 1, s.tailers.Count())
	_, found := s.tailers.Get(scanKey)
	require.True(t, found)
	state := s.pathHandoffStateForTest(testPath)
	require.False(t, state.ReplacementRequested)
	require.Equal(t, 0, mockFP.ComputeResultCallCount(), "pass-2 must not re-probe pathname when pass-1 candidate is authoritative")
	require.Equal(t, 1, mockFP.ComputeFromHandleCallCount(), "verify should fingerprint the buffered fd once")
	require.Equal(t, 0, mockFP.ComputeFromConfigCallCount(), "verified replacement must not call Position()/ComputeFingerprintFromConfig")
}

func TestSequentialDescriptorMismatchStallsReplacement(t *testing.T) {
	directCfg := sequentialDirectCfg()
	s, source, testPath, _, mockFP := setupLauncherWithMockFingerprinter(t, types.RotationHandoffModeSequential, directCfg)

	initialFP := &types.Fingerprint{Value: 0xaaaa, Config: directCfg}
	scanKey, _ := startSequentialTailer(t, s, source, testPath, mockFP, initialFP, directCfg)

	candidateB := &types.Fingerprint{Value: 0xbbbb, Config: directCfg}
	triggerSequentialHandoff(t, s, testPath, mockFP, candidateB, directCfg)

	candidateC := &types.Fingerprint{Value: 0xcccc, Config: directCfg}
	mockFP.ResetCallCounts()
	mockFP.SetHandleFingerprint(absPath(t, testPath), candidateC)
	finishSequentialDrain(t, s)

	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	require.Equal(t, 0, s.tailers.Count(), "descriptor mismatch must not start replacement tailer")
	_, found := s.tailers.Get(scanKey)
	require.False(t, found)
	state := s.pathHandoffStateForTest(testPath)
	require.True(t, state.ReplacementRequested)
	require.Equal(t, probeStatusDescriptorMismatch, s.replacementProbeStatusForTest(scanKey, testPath))
	require.Equal(t, 0, mockFP.ComputeResultCallCount(), "pass-2 must retain pass-1 candidate without pathname re-probe")
}

func TestSequentialDirectEstaleRecordsActiveProbeState(t *testing.T) {
	withStaleFileHandleTestHook(t)

	directCfg := sequentialDirectCfg()
	s, source, testPath, _, mockFP := setupLauncherWithMockFingerprinter(t, types.RotationHandoffModeSequential, directCfg)

	initialFP := &types.Fingerprint{Value: 0xaaaa, Config: directCfg}
	scanKey, _ := startSequentialTailer(t, s, source, testPath, mockFP, initialFP, directCfg)

	mockFP.SetFingerprintError(testPath, filetailer.ErrStaleFileHandleTest)
	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	require.Equal(t, 1, s.tailers.Count(), "ESTALE alone must not start sequential handoff")
	require.Empty(t, s.rotatedTailers)
	probeStatus, estaleCount := s.activeProbeStateForTest(scanKey)
	require.Equal(t, probeStatusEstaleDegraded, probeStatus)
	require.Equal(t, 1, estaleCount)
}

func TestSequentialDirectEstaleTransfersToReplacementIntent(t *testing.T) {
	withStaleFileHandleTestHook(t)

	directCfg := sequentialDirectCfg()
	s, source, testPath, _, mockFP := setupLauncherWithMockFingerprinter(t, types.RotationHandoffModeSequential, directCfg)

	initialFP := &types.Fingerprint{Value: 0xaaaa, Config: directCfg}
	scanKey, _ := startSequentialTailer(t, s, source, testPath, mockFP, initialFP, directCfg)

	mockFP.SetFingerprintError(testPath, filetailer.ErrStaleFileHandleTest)
	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)
	probeStatus, estaleCount := s.activeProbeStateForTest(scanKey)
	require.Equal(t, probeStatusEstaleDegraded, probeStatus)
	require.Equal(t, 1, estaleCount)

	candidateFP := &types.Fingerprint{Value: 0xbbbb, Config: directCfg}
	mockFP.SetFingerprintError(testPath, nil)
	triggerSequentialHandoff(t, s, testPath, mockFP, candidateFP, directCfg)

	_, estaleAfterTransfer := s.activeProbeStateForTest(scanKey)
	require.Equal(t, 0, estaleAfterTransfer, "pass-1 probe state must move to replacement intent")
	require.Equal(t, probeStatusEstaleDegraded, s.replacementProbeStatusForTest(scanKey, testPath))
}

func TestSequentialReplacementReprobesWhenFingerprintConfigChanges(t *testing.T) {
	directCfg := sequentialDirectCfg()
	s, source, testPath, _, mockFP := setupLauncherWithMockFingerprinter(t, types.RotationHandoffModeSequential, directCfg)

	initialFP := &types.Fingerprint{Value: 0xaaaa, Config: directCfg}
	scanKey, _ := startSequentialTailer(t, s, source, testPath, mockFP, initialFP, directCfg)

	candidateFP := &types.Fingerprint{Value: 0xbbbb, Config: directCfg}
	triggerSequentialHandoff(t, s, testPath, mockFP, candidateFP, directCfg)
	finishSequentialDrain(t, s)

	changedCfg := *directCfg
	changedCfg.Count = 2
	reprobeFP := &types.Fingerprint{Value: 0xcccc, Config: &changedCfg}
	mockFP.ResetCallCounts()
	mockFP.SetFingerprintWithAppliedFlags(testPath, reprobeFP, &changedCfg, []types.FileOpenFlag{types.FileOpenFlagDirect})
	mockFP.SetHandleFingerprint(absPath(t, testPath), reprobeFP)

	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	require.Equal(t, 1, s.tailers.Count())
	_, found := s.tailers.Get(scanKey)
	require.True(t, found)
	require.Equal(t, 1, mockFP.ComputeResultCallCount(), "pass-2 must re-probe when fingerprint config changed during handoff")
	require.Equal(t, 1, mockFP.ComputeFromHandleCallCount())
}
