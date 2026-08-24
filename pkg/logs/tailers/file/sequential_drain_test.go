// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

func TestBeginSequentialDrainQuietPeriod(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.close_timeout", 60)

	dir := t.TempDir()
	path := dir + "/quiet.log"
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.WriteString("line\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	outputChan := make(chan *message.Message, 10)
	logSource := sources.NewLogSource("test", &config.LogsConfig{Type: config.FileType, Path: path})
	source := sources.NewReplaceableSource(logSource)
	info := status.NewInfoRegistry()
	tailer := NewTailer(&TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(path, logSource, false),
		SleepDuration:   50 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	})
	require.NoError(t, tailer.StartFromBeginning())

	// Drain the initial line so the tailer reaches EOF before rotation bookkeeping.
	require.Eventually(t, func() bool {
		select {
		case <-outputChan:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	tailer.BeginSequentialDrain(200*time.Millisecond, 2*time.Second)

	// Pre-rotation EOF must not satisfy quiet period immediately.
	assert.False(t, tailer.IsReaderClosed())
	assert.False(t, tailer.IsFinished())

	// Post-detection zero-byte read starts quiet period; reader stop follows.
	require.Eventually(t, func() bool {
		return tailer.IsFinished()
	}, 3*time.Second, 50*time.Millisecond)
}

func TestBeginSequentialDrainRotationBookkeeping(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.close_timeout", 60)

	dir := t.TempDir()
	path := dir + "/bookkeeping.log"
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.WriteString("before\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	outputChan := make(chan *message.Message, 10)
	logSource := sources.NewLogSource("test", &config.LogsConfig{Type: config.FileType, Path: path})
	source := sources.NewReplaceableSource(logSource)
	info := status.NewInfoRegistry()
	tailer := NewTailer(&TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(path, logSource, false),
		SleepDuration:   50 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	})
	require.NoError(t, tailer.StartFromBeginning())
	require.Eventually(t, func() bool {
		select {
		case <-outputChan:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	tailer.BeginSequentialDrain(50*time.Millisecond, time.Second)

	// Append after rotation detect; drained message should use rotation identifier semantics.
	f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("after\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var msg *message.Message
	require.Eventually(t, func() bool {
		select {
		case msg = <-outputChan:
			return true
		default:
			return false
		}
	}, 2*time.Second, 20*time.Millisecond)

	assert.True(t, tailer.didFileRotate.Load())
	assert.Equal(t, "", msg.Origin.Identifier)
	assert.Equal(t, "0", msg.Origin.Offset)

	require.Eventually(t, func() bool {
		return tailer.IsFinished()
	}, 2*time.Second, 20*time.Millisecond)
}

func TestBeginSequentialDrainMaxDrainReaderClosedAtShutdown(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.close_timeout", 60)

	dir := t.TempDir()
	path := dir + "/max-drain.log"
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.WriteString("line\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	outputChan := make(chan *message.Message, 10)
	logSource := sources.NewLogSource("test", &config.LogsConfig{Type: config.FileType, Path: path})
	source := sources.NewReplaceableSource(logSource)
	info := status.NewInfoRegistry()
	tailer := NewTailer(&TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(path, logSource, false),
		SleepDuration:   20 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	})
	require.NoError(t, tailer.StartFromBeginning())
	require.Eventually(t, func() bool {
		select {
		case <-outputChan:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	tailer.sequentialForceCloseGrace = 30 * time.Millisecond
	tailer.BeginSequentialDrain(time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		return tailer.IsFinished()
	}, 2*time.Second, 10*time.Millisecond)
	assert.True(t, tailer.IsReaderClosed(), "reader fd must be closed when sequential drain completes")
}

func TestBeginSequentialDrainMaxDrainReaderClosedBeforeForwardFinishes(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.close_timeout", 60)

	dir := t.TempDir()
	path := dir + "/blocked-forward.log"
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.WriteString("line1\nline2\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Buffer one message so the next forward send blocks while the reader is still active.
	outputChan := make(chan *message.Message, 1)
	logSource := sources.NewLogSource("test", &config.LogsConfig{Type: config.FileType, Path: path})
	source := sources.NewReplaceableSource(logSource)
	info := status.NewInfoRegistry()
	tailer := NewTailer(&TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(path, logSource, false),
		SleepDuration:   20 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	})
	require.NoError(t, tailer.StartFromBeginning())
	require.Eventually(t, func() bool {
		return tailer.bytesRead.Get() > int64(len("line1\n"))
	}, time.Second, 10*time.Millisecond)

	tailer.sequentialForceCloseGrace = 30 * time.Millisecond
	tailer.BeginSequentialDrain(time.Second, 50*time.Millisecond)

	var readerClosedBeforeFinished bool
	require.Eventually(t, func() bool {
		if tailer.IsReaderClosed() && !tailer.IsFinished() {
			readerClosedBeforeFinished = true
		}
		return tailer.IsFinished()
	}, 2*time.Second, 5*time.Millisecond)
	require.True(t, readerClosedBeforeFinished, "reader fd should close before forward finishes")
	<-outputChan // release buffered message from before the drain
}
