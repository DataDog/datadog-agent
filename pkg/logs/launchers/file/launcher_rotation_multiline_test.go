// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/util/testutils"
)

// TestLauncherFileRotationSplitsMultilineGroup covers the plain-file
// `multi_line_pattern` case (as opposed to the CRI P/F container case, see
// launcher_rotation_cri_partial_test.go). A multiline group ("foo 1", "foo 2")
// is still open (no new group-starting line has been seen yet) when the file is
// rotated. The continuation line "foo 3" is written first thing in the new
// file, before the next group-starting line ("bar 1"). It is written
// synchronously right after the rotation, i.e. well within
// logs_config.aggregation_timeout, which is what makes the handoff apply — a
// continuation that shows up later is still expected to be flushed separately.
//
// Expected (correct) behavior: the launcher hands off the old tailer's
// pending multiline buffer to the new tailer, so "foo 1\nfoo 2\nfoo 3" is
// reassembled into a single message.
//
// Actual (buggy) behavior: createRotatedTailer (launcher.go) builds a brand
// new decoder for the new tailer with no knowledge of the old tailer's
// pending buffer. The old tailer's decoder flushes "foo 1\nfoo 2" early as
// its own broken message when stopped, and "foo 3" starts a new group in the
// new file instead of completing the old one.
func TestLauncherFileRotationSplitsMultilineGroup(t *testing.T) {
	cfg := configmock.New(t)
	testDir := t.TempDir()
	path := testDir + "/test.log"
	rotatedPath := path + ".1"

	f, err := os.Create(path)
	assert.Nil(t, err)

	multiLineRule := &config.ProcessingRule{
		Type: config.MultiLine,
		Name: "date_prefix",
		// Mirrors what config.CompileProcessingRules produces for a
		// user-supplied `multi_line_pattern` of `\d{4}-\d{2}-\d{2}`.
		Regex: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`),
	}
	source := sources.NewLogSource("", &config.LogsConfig{
		Type:            config.FileType,
		Path:            path,
		ProcessingRules: []*config.ProcessingRule{multiLineRule},
	})

	launcher := createLauncher(t, launcherTestOptions{})
	pipelineProvider := mock.NewMockProvider()
	launcher.pipelineProvider = pipelineProvider
	launcher.registry = auditorMock.NewMockRegistry()
	launcher.activeSources = append(launcher.activeSources, source)
	status.InitStatus(cfg, testutils.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()
	outputChan := pipelineProvider.NextPipelineChan()

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	// Group is left open: "foo 2" doesn't match the date-prefix regex, so the
	// decoder is still waiting to see whether the *next* line starts a new
	// group or continues this one.
	_, err = f.WriteString("2026-01-01 foo 1\nfoo 2\n")
	assert.Nil(t, err)

	// Rotate mid-group: the continuation line "foo 3" is the very first
	// thing written to the new file, ahead of the next group-starting line.
	err = os.Rename(path, rotatedPath)
	assert.Nil(t, err)
	newFile, err := os.Create(path)
	assert.Nil(t, err)
	_, err = newFile.WriteString("foo 3\n2026-01-01 bar 1\nbar 2\nbar 3\n")
	assert.Nil(t, err)

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	var messages []string
	timeout := time.After(2 * time.Second)
collect:
	for {
		select {
		case msg := <-outputChan:
			messages = append(messages, string(msg.GetContent()))
			if len(messages) >= 2 {
				// Give any further in-flight messages a brief moment to
				// arrive before declaring the collection complete.
				select {
				case msg := <-outputChan:
					messages = append(messages, string(msg.GetContent()))
				case <-time.After(200 * time.Millisecond):
					break collect
				}
			}
		case <-timeout:
			break collect
		}
	}

	t.Logf("received messages: %#v", messages)

	// This is the correct, expected behavior once the bug is fixed: the
	// pending "foo 1\nfoo 2" buffer from the old tailer should be carried
	// over and completed by "foo 3" from the new file, producing a single
	// combined message.
	assert.Contains(t, messages, "2026-01-01 foo 1\\nfoo 2\\nfoo 3",
		"the multiline group spanning the rotation boundary should be reassembled into a single message")
	assert.Contains(t, messages, "2026-01-01 bar 1\\nbar 2\\nbar 3")

	launcher.cleanup()
}

// TestLauncherFileRotationFlushesUncontinuedMultilineGroup is the companion of
// the test above: the rotation handoff must not turn into an open-ended wait.
// Here no continuation is ever written to the new file, so the carried-over
// group has to be emitted on its own once the aggregation timeout elapses,
// exactly as it was before the handoff existed.
func TestLauncherFileRotationFlushesUncontinuedMultilineGroup(t *testing.T) {
	cfg := configmock.New(t)
	testDir := t.TempDir()
	path := testDir + "/test.log"
	rotatedPath := path + ".1"

	f, err := os.Create(path)
	assert.Nil(t, err)

	multiLineRule := &config.ProcessingRule{
		Type:  config.MultiLine,
		Name:  "date_prefix",
		Regex: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`),
	}
	source := sources.NewLogSource("", &config.LogsConfig{
		Type:            config.FileType,
		Path:            path,
		ProcessingRules: []*config.ProcessingRule{multiLineRule},
	})

	launcher := createLauncher(t, launcherTestOptions{})
	pipelineProvider := mock.NewMockProvider()
	launcher.pipelineProvider = pipelineProvider
	launcher.registry = auditorMock.NewMockRegistry()
	launcher.activeSources = append(launcher.activeSources, source)
	status.InitStatus(cfg, testutils.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()
	outputChan := pipelineProvider.NextPipelineChan()

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	_, err = f.WriteString("2026-01-01 foo 1\nfoo 2\n")
	assert.Nil(t, err)

	// Rotate, and leave the new file empty.
	err = os.Rename(path, rotatedPath)
	assert.Nil(t, err)
	_, err = os.Create(path)
	assert.Nil(t, err)

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	var messages []string
	timeout := time.After(5 * time.Second)
collect:
	for {
		select {
		case msg := <-outputChan:
			messages = append(messages, string(msg.GetContent()))
			break collect
		case <-timeout:
			break collect
		}
	}

	t.Logf("received messages: %#v", messages)

	assert.Contains(t, messages, "2026-01-01 foo 1\\nfoo 2",
		"a carried-over group with no continuation must still be flushed on the aggregation timeout")

	launcher.cleanup()
}
