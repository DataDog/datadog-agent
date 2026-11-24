// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

type duplicateSourcesTest struct {
	launcher *Launcher
	path     string
	scanKey  string
	config1  *sources.LogSource
	config2  *sources.LogSource
}

// newDuplicateSourcesTest mirrors two configs in one container's logs annotation: both match the
// same file of the same container, and config2 is added last.
func newDuplicateSourcesTest(t *testing.T, opts launcherTestOptions) *duplicateSourcesTest {
	path := filepath.Join(t.TempDir(), "app.log")
	require.NoError(t, os.WriteFile(path, []byte("first line\n"), 0o644))

	launcher := createLauncher(t, opts)
	launcher.pipelineProvider = mock.NewMockProvider()
	launcher.registry = auditorMock.NewMockRegistry()
	// Drain the pipeline so the tailers never block on it and can stop.
	done := make(chan struct{})
	go func() {
		output := launcher.pipelineProvider.NextPipelineChan()
		for {
			select {
			case <-output:
			case <-done:
				return
			}
		}
	}()
	t.Cleanup(func() {
		launcher.cleanup()
		close(done)
	})

	newSource := func(name string) *sources.LogSource {
		return sources.NewLogSource(name, &config.LogsConfig{Type: config.FileType, Identifier: "container-id", Path: path, Source: name})
	}
	test := &duplicateSourcesTest{
		launcher: launcher,
		path:     path,
		scanKey:  path + "/container-id",
		config1:  newSource("config1"),
		config2:  newSource("config2"),
	}
	launcher.addSource(test.config1)
	launcher.addSource(test.config2)
	return test
}

func (d *duplicateSourcesTest) scan(inputSources []*sources.LogSource) {
	d.launcher.resolveActiveTailers(d.launcher.fileProvider.FilesToTail(context.Background(), d.launcher.validatePodContainerID, inputSources, d.launcher.registry))
}

// stopTailers stops the tailers addSource started, so that the next scan starts them, as it does
// for a file that appears after its sources were added.
func (d *duplicateSourcesTest) stopTailers() {
	d.launcher.cleanup()
	// addSource also queued its files for the next scan, and they still point at the stopped tailer's
	// source, which has since been handed to config2.
	d.launcher.filesTailedBetweenScans = d.launcher.filesTailedBetweenScans[:0]
}

func (d *duplicateSourcesTest) tailedSource(t *testing.T) *sources.LogSource {
	tailer, isTailed := d.launcher.tailers.Get(d.scanKey)
	require.True(t, isTailed)
	return tailer.Source()
}

func (d *duplicateSourcesTest) rotate(t *testing.T) {
	require.NoError(t, os.Rename(d.path, d.path+".1"))
	require.NoError(t, os.WriteFile(d.path, []byte("after rotation\n"), 0o644))
}

func duplicateSourceMessages(source *sources.LogSource) []string {
	var messages []string
	for _, message := range source.Messages.GetMessages() {
		if strings.Contains(message, "is also matched by another log config") {
			messages = append(messages, message)
		}
	}
	return messages
}

func TestDuplicateSourcesKeepLastAddedAcrossRotation(t *testing.T) {
	for name, fingerprintConfig := range map[string]*types.FingerprintConfig{
		"without fingerprinting": nil,
		"with fingerprinting": {
			FingerprintStrategy: types.FingerprintStrategyLineChecksum,
			Count:               1,
			MaxBytes:            10000,
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := newDuplicateSourcesTest(t, launcherTestOptions{fingerprintConfig: fingerprintConfig})
			assert.Same(t, d.config2, d.tailedSource(t))

			d.scan(d.launcher.activeSources)
			assert.Same(t, d.config2, d.tailedSource(t))

			d.rotate(t)
			d.scan(d.launcher.activeSources)
			if fingerprintConfig != nil {
				// The fingerprinted rotation stops the old tailer on one scan and starts the
				// replacement on the next.
				d.scan(d.launcher.activeSources)
			}
			assert.Same(t, d.config2, d.tailedSource(t))
		})
	}
}

func TestDuplicateSourcesScanStartsTailerWithLastAdded(t *testing.T) {
	d := newDuplicateSourcesTest(t, launcherTestOptions{})
	// Let the scan, rather than addSource, start the tailer.
	d.stopTailers()

	d.scan(d.launcher.activeSources)
	assert.Same(t, d.config2, d.tailedSource(t))
}

func TestDuplicateSourcesStaleScanKeepsNewerSource(t *testing.T) {
	d := newDuplicateSourcesTest(t, launcherTestOptions{})

	// A scan that took its copy of activeSources before config2 was added only lists config1.
	d.scan([]*sources.LogSource{d.config1})
	assert.Same(t, d.config2, d.tailedSource(t))

	d.rotate(t)
	d.scan([]*sources.LogSource{d.config1})
	assert.Same(t, d.config2, d.tailedSource(t))
}

func TestDuplicateSourcesAtFileLimitKeepLastAdded(t *testing.T) {
	// With a single slot, the tailer config1 opened takes it, and the scans only list config1's
	// copy of the file.
	d := newDuplicateSourcesTest(t, launcherTestOptions{openFilesLimit: 1})
	assert.Same(t, d.config2, d.tailedSource(t))

	d.scan(d.launcher.activeSources)
	assert.Same(t, d.config2, d.tailedSource(t))

	d.rotate(t)
	d.scan(d.launcher.activeSources)
	assert.Same(t, d.config2, d.tailedSource(t))
}

func TestDuplicateSourcesAtFileLimitBeforeFileExists(t *testing.T) {
	d := newDuplicateSourcesTest(t, launcherTestOptions{openFilesLimit: 1})
	// Both sources are added before the file appears, so the first scan starts the tailer.
	d.stopTailers()

	d.scan(d.launcher.activeSources)
	assert.Same(t, d.config2, d.tailedSource(t))
	assert.Len(t, duplicateSourceMessages(d.config1), 1)
}

func TestDuplicateSourcesRemovingOwnerHandsFileToRemainingSource(t *testing.T) {
	d := newDuplicateSourcesTest(t, launcherTestOptions{})
	d.scan(d.launcher.activeSources)
	require.Same(t, d.config2, d.tailedSource(t))

	d.launcher.removeSource(d.config2)
	d.scan(d.launcher.activeSources)
	assert.Same(t, d.config1, d.tailedSource(t))
}

func TestDuplicateSourcesReportedOnIgnoredSource(t *testing.T) {
	d := newDuplicateSourcesTest(t, launcherTestOptions{})
	d.scan(d.launcher.activeSources)

	messages := duplicateSourceMessages(d.config1)
	require.Len(t, messages, 1)
	assert.Contains(t, messages[0], d.path)
	assert.Contains(t, messages[0], "source: config2")
	assert.Empty(t, duplicateSourceMessages(d.config2))

	// The message stays while the conflict lasts...
	d.scan(d.launcher.activeSources)
	assert.Len(t, duplicateSourceMessages(d.config1), 1)

	// ...and goes away once the other config is removed.
	d.launcher.removeSource(d.config2)
	d.scan(d.launcher.activeSources)
	assert.Empty(t, duplicateSourceMessages(d.config1))
}

func TestDuplicateSourcesNotReportedForSingleSource(t *testing.T) {
	d := newDuplicateSourcesTest(t, launcherTestOptions{})
	d.launcher.removeSource(d.config1)
	d.scan(d.launcher.activeSources)

	assert.Empty(t, duplicateSourceMessages(d.config1))
	assert.Empty(t, duplicateSourceMessages(d.config2))
}

func TestDescribeSourceTellsApartProcessingRules(t *testing.T) {
	newSource := func(rules ...*config.ProcessingRule) *sources.LogSource {
		return sources.NewLogSource("container", &config.LogsConfig{Source: "app", Service: "web", ProcessingRules: rules})
	}
	keepAll := newSource()
	dropDebug := newSource(&config.ProcessingRule{Type: config.ExcludeAtMatch, Name: "drop_debug"})

	assert.Equal(t, "source: app, service: web", describeSource(keepAll))
	assert.Equal(t, "source: app, service: web, log_processing_rules: exclude_at_match drop_debug", describeSource(dropDebug))
}
