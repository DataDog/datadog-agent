// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package file

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// Keep the standalone library's registry contract compatible in both directions
// without requiring it to import the Agent root module.
var (
	_ auditor.Registry = routing.Registry(nil)
	_ routing.Registry = auditor.Registry(nil)
)

type replaySink struct{ ch chan *message.Payload }

func (s *replaySink) Channel() chan *message.Payload { return s.ch }

func TestEndpointFilteringReplaysFileFromPersistedPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.log")
	require.NoError(t, os.WriteFile(path, []byte("first\nsecond\nthird\n"), 0o600))
	identifier := "file:" + path
	registry := auditorMock.NewMockRegistry()
	makeTailer := func(output chan *message.Message) *filetailer.Tailer {
		source := sources.NewReplaceableSource(sources.NewLogSource("replay", &config.LogsConfig{Type: config.FileType, Path: path}))
		info := status.NewInfoRegistry()
		return filetailer.NewTailer(&filetailer.TailerOptions{
			OutputChan: output, File: filetailer.NewFile(path, source.UnderlyingSource(), false),
			SleepDuration: 10 * time.Millisecond, Decoder: decoder.NewDecoderFromSource(source, info),
			Info: info, Registry: registry, FileOpener: opener.NewFileOpener(),
			CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		})
	}
	read := func(ch chan *message.Message) *message.Message {
		t.Helper()
		select {
		case msg := <-ch:
			return msg
		case <-time.After(5 * time.Second):
			t.Fatal("file record did not arrive")
			return nil
		}
	}
	sink := &replaySink{ch: make(chan *message.Payload, 2)}
	progress, err := routing.NewProgress(4, sink)
	require.NoError(t, err)
	output := make(chan *message.Message, 2)
	tailer := makeTailer(output)
	require.NoError(t, tailer.StartFromBeginning())
	first, second, third := read(output), read(output), read(output)
	require.Equal(t, "first", string(first.GetContent()))
	require.Equal(t, "second", string(second.GetContent()))
	require.Equal(t, "third", string(third.GetContent()))
	reserve := func(p *routing.Progress, m *message.Message, generation uint64, endpointID string) {
		t.Helper()
		token, err := p.Reserve(context.Background(), m, generation)
		require.NoError(t, err)
		require.NoError(t, p.AttachEndpointSelection(token, []string{endpointID}, nil, false))
	}
	complete := func(p *routing.Progress, endpointID string, m *message.Message) {
		meta := m.MessageMetadata
		p.CompleteReliableSuccess(endpointID, message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0))
	}
	reserve(progress, first, 1, "a")
	reserve(progress, second, 1, "a")
	reserve(progress, third, 1, "b")
	complete(progress, "a", first)
	select {
	case checkpoint := <-sink.ch:
		registry.SetOffset(identifier, checkpoint.MessageMetas[0].Origin.Offset) // Periodically persisted auditor output.
	case <-time.After(5 * time.Second):
		t.Fatal("first checkpoint did not arrive")
	}
	require.Equal(t, strconv.Itoa(len("first\n")), registry.GetOffset(identifier))
	complete(progress, "b", third)
	require.Equal(t, 2, progress.Pending(), "the third record must wait behind the second")
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("B crossed A's pending checkpoint: %s", checkpoint.MessageMetas[0].Origin.Offset)
	default:
	}
	// Capture the persisted state before any cleanup. This is the abrupt-restart
	// boundary: third reached B but never advanced the saved file position past second.
	saved := registry.GetOffset(identifier)
	tailer.Stop() // Test cleanup after the snapshot; no provider Flush is used.
	progress.Close()
	require.Equal(t, saved, registry.GetOffset(identifier))
	offset, whence, err := Position(registry, identifier, config.End, filetailer.NewFingerprinterMock(), opener.NewFileOpener(), nil)
	require.NoError(t, err)
	require.Equal(t, int64(len("first\n")), offset)
	require.Equal(t, io.SeekStart, whence)
	noCheckpoint := auditorMock.NewMockRegistry()
	missingOffset, missingWhence, err := Position(noCheckpoint, identifier, config.End, filetailer.NewFingerprinterMock(), opener.NewFileOpener(), nil)
	require.NoError(t, err)
	require.Zero(t, missingOffset)
	require.Equal(t, io.SeekEnd, missingWhence, "end mode cannot replay without an initial saved checkpoint")
	replayOutput := make(chan *message.Message, 2)
	replayTailer := makeTailer(replayOutput)
	require.NoError(t, replayTailer.Start(offset, whence))
	defer replayTailer.Stop()
	replayedSecond, replayedThird := read(replayOutput), read(replayOutput)
	require.Equal(t, "second", string(replayedSecond.GetContent()))
	require.Equal(t, "third", string(replayedThird.GetContent()))
	replaySink := &replaySink{ch: make(chan *message.Payload, 2)}
	replayProgress, err := routing.NewProgress(2, replaySink)
	require.NoError(t, err)
	defer replayProgress.Close()
	reserve(replayProgress, replayedSecond, 2, "a")
	reserve(replayProgress, replayedThird, 2, "b")
	complete(replayProgress, "b", replayedThird) // B receives its duplicate after restart.
	complete(replayProgress, "a", replayedSecond)
	for _, want := range []int{len("first\nsecond\n"), len("first\nsecond\nthird\n")} {
		select {
		case checkpoint := <-replaySink.ch:
			require.Equal(t, strconv.Itoa(want), checkpoint.MessageMetas[0].Origin.Offset)
		case <-time.After(5 * time.Second):
			t.Fatalf("replayed file checkpoint did not advance to %d", want)
		}
	}
}
