// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"context"
	"os"
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

// TestLauncherFileRotationSplitsCRIPartialLine covers the container case: a CRI
// log line long enough to be chunked by the runtime is written as a sequence of
// `P` (partial) records terminated by an `F` (full) record, and containerd
// rotates the log file in the middle of that sequence.
//
// The `P` chunks are buffered by the decoder's MultiLineParser stage (selected
// because the kubernetes parser reports SupportsPartialLine). Without a handoff
// the old tailer's decoder flushes those chunks as their own truncated message
// and the `F` chunk in the new file is emitted as a separate line, so one
// application log line is delivered as two broken ones.
//
// The `F` chunk is written synchronously right after the rotation, i.e. well
// within logs_config.aggregation_timeout. That is deliberate: the handoff only
// bridges a continuation that lands promptly, a later one is still expected to
// be flushed on its own by the regular timeout.
func TestLauncherFileRotationSplitsCRIPartialLine(t *testing.T) {
	cfg := configmock.New(t)
	testDir := t.TempDir()
	path := testDir + "/0.log"
	rotatedPath := path + ".20260101-000000"

	f, err := os.Create(path)
	assert.Nil(t, err)

	source := sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: path,
	})
	// Makes the decoder pick the kubernetes (CRI) parser, and therefore the
	// MultiLineParser that reassembles P/F chunk sequences.
	source.SetSourceType(sources.KubernetesSourceType)

	launcher := createLauncher(t, launcherTestOptions{})
	pipelineProvider := mock.NewMockProvider()
	launcher.pipelineProvider = pipelineProvider
	launcher.registry = auditorMock.NewMockRegistry()
	launcher.activeSources = append(launcher.activeSources, source)
	status.InitStatus(cfg, testutils.CreateSources([]*sources.LogSource{source}))
	defer status.Clear()
	outputChan := pipelineProvider.NextPipelineChan()

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	// A complete line, then the opening chunks of a partial line. The chunk
	// sequence is left unterminated: no `F` record has been seen yet.
	_, err = f.WriteString("2026-01-01T00:00:00.000000000Z stdout F complete line\n" +
		"2026-01-01T00:00:01.000000000Z stdout P upstream request timeout: \n" +
		"2026-01-01T00:00:01.100000000Z stdout P cluster=envoy-ingress \n")
	assert.Nil(t, err)

	// Rotate mid-sequence: the terminating `F` chunk is the very first record
	// written to the new file.
	err = os.Rename(path, rotatedPath)
	assert.Nil(t, err)
	newFile, err := os.Create(path)
	assert.Nil(t, err)
	_, err = newFile.WriteString("2026-01-01T00:00:01.200000000Z stdout F host=10.0.0.1\n" +
		"2026-01-01T00:00:02.000000000Z stdout F line after rotation\n")
	assert.Nil(t, err)

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	var messages []string
	timeout := time.After(2 * time.Second)
collect:
	for {
		select {
		case msg := <-outputChan:
			messages = append(messages, string(msg.GetContent()))
			if len(messages) >= 3 {
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

	assert.Contains(t, messages, "complete line")
	// The P chunks buffered before the rotation must be completed by the F chunk
	// from the new file, reassembling the original line. MultiLineParser
	// concatenates chunks verbatim, so the trailing spaces in the P records are
	// what separate the fragments.
	assert.Contains(t, messages, "upstream request timeout: cluster=envoy-ingress host=10.0.0.1",
		"the CRI partial line spanning the rotation boundary should be reassembled into a single message")
	assert.Contains(t, messages, "line after rotation")

	launcher.cleanup()
}
