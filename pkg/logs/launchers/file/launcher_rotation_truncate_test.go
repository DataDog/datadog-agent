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

// TestLauncherTruncateRotationDoesNotStitchMultilineGroup pins the one case
// where the rotation handoff must NOT happen. A truncation rewrites the file in
// place, so the bytes between the tailer's read offset and the old size are
// gone for good. Joining the buffered group to the new file's first line would
// produce a single message assembled from two fragments that were never
// adjacent, hiding the loss - worse than the two honest fragments the Agent
// emitted before any of this work.
//
// Fixing the truncation loss itself is out of scope (it is unrecoverable); this
// only asserts the handoff does not make it worse.
func TestLauncherTruncateRotationDoesNotStitchMultilineGroup(t *testing.T) {
	cfg := configmock.New(t)
	testDir := t.TempDir()
	path := testDir + "/test.log"

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

	// Leave a group open, and wait for the tailer to have actually read it so
	// the read offset is past the content the truncation removes.
	_, err = f.WriteString("2026-01-01 foo 1\nfoo 2\n")
	assert.Nil(t, err)
	assert.Nil(t, f.Sync())
	assert.Eventually(t, func() bool {
		tailer, found := launcher.tailers.Get(getScanKey(path, source))
		return found && tailer.Source().BytesRead.Get() >= int64(len("2026-01-01 foo 1\nfoo 2\n"))
	}, 3*time.Second, 20*time.Millisecond, "the tailer should have read the pre-truncation content")

	// Copy-truncate in place: same inode, size drops below the read offset.
	// The replacement content is deliberately shorter than the old read offset,
	// so the old tailer's file handle sees nothing more and only the
	// replacement tailer reads "foo 3".
	assert.Nil(t, f.Truncate(0))
	_, err = f.Seek(0, 0)
	assert.Nil(t, err)
	assert.Nil(t, f.Sync())
	_, err = f.WriteString("foo 3\n")
	assert.Nil(t, err)
	assert.Nil(t, f.Sync())

	launcher.resolveActiveTailers(launcher.fileProvider.FilesToTail(context.Background(), launcher.validatePodContainerID, launcher.activeSources, launcher.registry))

	var messages []string
	timeout := time.After(5 * time.Second)
collect:
	for {
		select {
		case msg := <-outputChan:
			messages = append(messages, string(msg.GetContent()))
			if len(messages) >= 2 {
				break collect
			}
		case <-timeout:
			break collect
		}
	}

	t.Logf("received messages: %#v", messages)

	assert.NotContains(t, messages, "2026-01-01 foo 1\\nfoo 2\\nfoo 3",
		"content must not be stitched across a truncation: the two fragments were never adjacent")
	// The honest pre-fix outcome: both halves are emitted separately, and
	// neither is lost.
	assert.Contains(t, messages, "2026-01-01 foo 1\\nfoo 2")
	assert.Contains(t, messages, "foo 3")

	launcher.cleanup()
}
