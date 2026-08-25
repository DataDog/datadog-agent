// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"context"
	"os"
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
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/testutils"
)

func setupSequentialHandoffLauncher(t *testing.T) (*Launcher, *sources.LogSource, string, string, chan *message.Message) {
	t.Helper()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.close_timeout", 2)

	testDir := t.TempDir()
	testPath := testDir + "/launcher.log"
	testRotatedPath := testPath + ".1"

	testFile, err := os.Create(testPath)
	require.NoError(t, err)
	_, err = testFile.WriteString("seed\n")
	require.NoError(t, err)
	require.NoError(t, testFile.Sync())
	require.NoError(t, testFile.Close())
	_, err = os.Create(testRotatedPath)
	require.NoError(t, err)

	quiet := 2
	maxDrain := 30
	fingerprintConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            256,
	}
	source := sources.NewLogSource("", &config.LogsConfig{
		Type:                          config.FileType,
		Path:                          testPath,
		RotationHandoffMode:           string(types.RotationHandoffModeSequential),
		SequentialRotationQuietPeriod: &quiet,
		SequentialRotationMaxDrain:    &maxDrain,
		FingerprintConfig:             fingerprintConfig,
	})

	pipelineProvider := mock.NewMockProvider()
	outputChan := pipelineProvider.NextPipelineChan()
	s := createLauncher(t, launcherTestOptions{fingerprintConfig: fingerprintConfig})
	s.forceSequentialHandoffForTest = true
	s.globalHandoffSettings = types.RotationHandoffSettings{
		Mode:               types.RotationHandoffModeSequential,
		QuietPeriodSeconds: quiet,
		MaxDrainSeconds:    maxDrain,
		QuietPeriod:        time.Duration(quiet) * time.Second,
		MaxDrain:           time.Duration(maxDrain) * time.Second,
	}
	s.pipelineProvider = pipelineProvider
	s.registry = auditorMock.NewMockRegistry()
	s.activeSources = []*sources.LogSource{source}
	status.Clear()
	status.InitStatus(mockConfig, testutils.CreateSources([]*sources.LogSource{source}))
	t.Cleanup(status.Clear)
	t.Cleanup(s.cleanup)

	return s, source, testPath, testRotatedPath, outputChan
}

func TestSequentialHandoffWaitsForDrainingTailerBeforeReplacement(t *testing.T) {
	s, source, testPath, testRotatedPath, outputChan := setupSequentialHandoffLauncher(t)

	testFile, err := os.OpenFile(testPath, os.O_RDWR, 0o644)
	require.NoError(t, err)
	_, err = testFile.WriteString("before rotation\n")
	require.NoError(t, err)
	require.NoError(t, testFile.Sync())

	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)
	require.Equal(t, "before rotation", string((<-outputChan).GetContent()))

	scanKey := getScanKey(testPath, source)
	oldTailer, found := s.tailers.Get(scanKey)
	require.True(t, found)
	require.NoError(t, testFile.Close())

	require.NoError(t, os.Rename(testPath, testRotatedPath))
	replacementFile, err := os.Create(testPath)
	require.NoError(t, err)
	_, err = replacementFile.WriteString("after rotation\n")
	require.NoError(t, err)
	require.NoError(t, replacementFile.Sync())
	require.NoError(t, replacementFile.Close())

	files = s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	require.Equal(t, 0, s.tailers.Count())
	require.Len(t, s.rotatedTailers, 1)
	state := s.pathHandoffStateForTest(testPath)
	require.Equal(t, 1, state.DrainingCount)
	require.True(t, state.ReplacementRequested)

	s.resolveActiveTailers(files)
	require.Equal(t, 0, s.tailers.Count())

	oldTailer.Stop()
	require.True(t, oldTailer.IsFinished())
	s.resolveActiveTailers(files)

	require.Equal(t, 1, s.tailers.Count())
	newTailer, found := s.tailers.Get(scanKey)
	require.True(t, found)
	require.NotSame(t, oldTailer, newTailer)
	require.Equal(t, "after rotation", string((<-outputChan).GetContent()))
	state = s.pathHandoffStateForTest(testPath)
	require.False(t, state.ReplacementRequested)
}

func TestSequentialHandoffSourceReloadDoesNotOpenDuringDrain(t *testing.T) {
	s, source, testPath, testRotatedPath, outputChan := setupSequentialHandoffLauncher(t)

	testFile, err := os.OpenFile(testPath, os.O_RDWR, 0o644)
	require.NoError(t, err)
	_, err = testFile.WriteString("line\n")
	require.NoError(t, err)
	require.NoError(t, testFile.Sync())
	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)
	<-outputChan

	oldTailer, _ := s.tailers.Get(getScanKey(testPath, source))
	require.NotNil(t, oldTailer)
	require.NoError(t, testFile.Close())

	require.NoError(t, os.Rename(testPath, testRotatedPath))
	replacementFile, err := os.Create(testPath)
	require.NoError(t, err)
	_, err = replacementFile.WriteString("new\n")
	require.NoError(t, err)
	require.NoError(t, replacementFile.Sync())
	require.NoError(t, replacementFile.Close())

	files = s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)
	require.Equal(t, 0, s.tailers.Count())

	s.removeSource(source)
	quiet := 2
	maxDrain := 30
	reloaded := sources.NewLogSource("", &config.LogsConfig{
		Type:                          config.FileType,
		Path:                          testPath,
		RotationHandoffMode:           string(types.RotationHandoffModeSequential),
		SequentialRotationQuietPeriod: &quiet,
		SequentialRotationMaxDrain:    &maxDrain,
		FingerprintConfig:             source.Config.FingerprintConfig,
	})
	s.addSource(reloaded)
	require.Equal(t, 0, s.tailers.Count())
	state := s.pathHandoffStateForTest(testPath)
	require.Equal(t, 1, state.DrainingCount)
	require.True(t, state.ReplacementRequested)
}

func sequentialHandoffSource(t *testing.T, testPath string, identifier string, fingerprintConfig *types.FingerprintConfig) *sources.LogSource {
	t.Helper()
	quiet := 2
	maxDrain := 30
	return sources.NewLogSource(identifier, &config.LogsConfig{
		Type:                          config.FileType,
		Path:                          testPath,
		Identifier:                    identifier,
		RotationHandoffMode:           string(types.RotationHandoffModeSequential),
		SequentialRotationQuietPeriod: &quiet,
		SequentialRotationMaxDrain:    &maxDrain,
		FingerprintConfig:             fingerprintConfig,
	})
}

func findRotatedTailer(rotated []*tailer.Tailer, scanKey string) *tailer.Tailer {
	for _, t := range rotated {
		if t.GetID() == scanKey {
			return t
		}
	}
	return nil
}

func TestSequentialHandoffBlocksUntilAllDrainingReadersClose(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.close_timeout", 2)

	testDir := t.TempDir()
	testPath := testDir + "/shared.log"
	testRotatedPath := testPath + ".1"

	testFile, err := os.Create(testPath)
	require.NoError(t, err)
	_, err = testFile.WriteString("seed\n")
	require.NoError(t, err)
	require.NoError(t, testFile.Sync())
	require.NoError(t, testFile.Close())
	_, err = os.Create(testRotatedPath)
	require.NoError(t, err)

	quiet := 2
	maxDrain := 30
	fingerprintConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            256,
	}
	sourceA := sequentialHandoffSource(t, testPath, "container-a", fingerprintConfig)
	sourceB := sequentialHandoffSource(t, testPath, "container-b", fingerprintConfig)

	pipelineProvider := mock.NewMockProvider()
	outputChan := pipelineProvider.NextPipelineChan()
	s := createLauncher(t, launcherTestOptions{fingerprintConfig: fingerprintConfig})
	s.forceSequentialHandoffForTest = true
	s.globalHandoffSettings = types.RotationHandoffSettings{
		Mode:               types.RotationHandoffModeSequential,
		QuietPeriodSeconds: quiet,
		MaxDrainSeconds:    maxDrain,
		QuietPeriod:        time.Duration(quiet) * time.Second,
		MaxDrain:           time.Duration(maxDrain) * time.Second,
	}
	s.pipelineProvider = pipelineProvider
	s.registry = auditorMock.NewMockRegistry()
	s.activeSources = []*sources.LogSource{sourceA, sourceB}
	status.Clear()
	status.InitStatus(mockConfig, testutils.CreateSources([]*sources.LogSource{sourceA, sourceB}))
	t.Cleanup(status.Clear)
	t.Cleanup(s.cleanup)

	testFile, err = os.OpenFile(testPath, os.O_RDWR, 0o644)
	require.NoError(t, err)
	_, err = testFile.WriteString("before rotation\n")
	require.NoError(t, err)
	require.NoError(t, testFile.Sync())

	files := s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)
	<-outputChan
	<-outputChan

	scanKeyA := getScanKey(testPath, sourceA)
	scanKeyB := getScanKey(testPath, sourceB)
	require.Equal(t, 2, s.tailers.Count())
	require.NoError(t, testFile.Close())

	require.NoError(t, os.Rename(testPath, testRotatedPath))
	replacementFile, err := os.Create(testPath)
	require.NoError(t, err)
	_, err = replacementFile.WriteString("after rotation\n")
	require.NoError(t, err)
	require.NoError(t, replacementFile.Sync())
	require.NoError(t, replacementFile.Close())

	files = s.fileProvider.FilesToTail(context.Background(), s.validatePodContainerID, s.activeSources, s.registry)
	s.resolveActiveTailers(files)

	require.Equal(t, 0, s.tailers.Count())
	require.Len(t, s.rotatedTailers, 2)
	state := s.pathHandoffStateForTest(testPath)
	require.Equal(t, 2, state.DrainingCount)
	require.True(t, state.ReplacementRequested)

	oldA := findRotatedTailer(s.rotatedTailers, scanKeyA)
	oldB := findRotatedTailer(s.rotatedTailers, scanKeyB)
	require.NotNil(t, oldA)
	require.NotNil(t, oldB)

	oldA.Stop()
	require.True(t, oldA.IsReaderClosed())
	s.resolveActiveTailers(files)
	require.Equal(t, 0, s.tailers.Count(), "replacement must stay blocked while another draining reader is open")

	oldB.Stop()
	require.True(t, oldB.IsReaderClosed())
	s.resolveActiveTailers(files)

	require.Equal(t, 2, s.tailers.Count())
	newA, found := s.tailers.Get(scanKeyA)
	require.True(t, found)
	newB, found := s.tailers.Get(scanKeyB)
	require.True(t, found)
	require.NotSame(t, oldA, newA)
	require.NotSame(t, oldB, newB)
	<-outputChan
	<-outputChan
}
