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

// TestLauncherFileRotationSplitsAggregatedJSON covers the JSONAggregator stage,
// which buffers independently of the multiline aggregator and sits ahead of it
// in the Preprocessor. A pretty-printed JSON object is split by the rotation at
// a line boundary inside the object; the closing half is the first thing in the
// new file.
//
// logs_config.auto_multi_line.enable_json_aggregation is on by default, so this
// is the same bug class as the original report, one stage earlier.
func TestLauncherFileRotationSplitsAggregatedJSON(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.auto_multi_line_detection", true)
	cfg.SetInTest("logs_config.auto_multi_line.enable_json_aggregation", true)

	testDir := t.TempDir()
	path := testDir + "/test.log"
	rotatedPath := path + ".1"

	f, err := os.Create(path)
	assert.Nil(t, err)

	source := sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: path,
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

	// The object is left open: the JSON aggregator is holding an incomplete
	// parse and waiting for the rest.
	_, err = f.WriteString("{\"message\": \"this is a\",\n")
	assert.Nil(t, err)
	assert.Nil(t, f.Sync())

	// Rotate mid-object. The closing half lands first in the new file,
	// synchronously, i.e. well within logs_config.aggregation_timeout.
	err = os.Rename(path, rotatedPath)
	assert.Nil(t, err)
	newFile, err := os.Create(path)
	assert.Nil(t, err)
	_, err = newFile.WriteString("\"level\": \"info\"}\n")
	assert.Nil(t, err)
	assert.Nil(t, newFile.Sync())

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	var messages []string
	timeout := time.After(5 * time.Second)
collect:
	for {
		select {
		case msg := <-outputChan:
			messages = append(messages, string(msg.GetContent()))
			select {
			case msg := <-outputChan:
				messages = append(messages, string(msg.GetContent()))
			case <-time.After(500 * time.Millisecond):
				break collect
			}
		case <-timeout:
			break collect
		}
	}

	t.Logf("received messages: %#v", messages)

	// Compacted into the single object the aggregator would have produced had
	// the rotation not happened.
	assert.Contains(t, messages, `{"message":"this is a","level":"info"}`,
		"a JSON object spanning the rotation boundary should be aggregated into a single message")

	launcher.cleanup()
}
