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
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

func TestEndpointFilteringRequireAnyFileCheckpointReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "endpoint-filter-replay.log")
	require.NoError(t, os.WriteFile(path, []byte("info-1\nnotice-2\n"), 0o600))
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

	sink := &replaySink{ch: make(chan *message.Payload, 4)}
	progress, err := routing.NewProgressWithMode(4, sink, routing.CompletionRequireAny)
	require.NoError(t, err)

	output := make(chan *message.Message, 2)
	tailer := makeTailer(output)
	require.NoError(t, tailer.StartFromBeginning())
	first, second := read(output), read(output)
	require.Equal(t, "info-1", string(first.GetContent()))
	require.Equal(t, "notice-2", string(second.GetContent()))

	// Both records have US1 ("ep-0") as their reliable obligation; US5 is
	// optional and never enters Progress.
	for _, m := range []*message.Message{first, second} {
		_, err := progress.Register(context.Background(), m, routing.Selection{Generation: 1}, []string{"ep-0"})
		require.NoError(t, err)
	}

	complete := func(p *routing.Progress, group string, m *message.Message) {
		meta := m.MessageMetadata
		p.CompleteGroup(group, message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0))
	}

	// US1 delivers first -> checkpoint advances past info-1.
	complete(progress, "ep-0", first)
	select {
	case cp := <-sink.ch:
		registry.SetOffset(identifier, cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("first checkpoint did not arrive")
	}
	require.Equal(t, strconv.Itoa(len("info-1\n")), registry.GetOffset(identifier))

	// Simulate abrupt restart while second is still pending on US1 (even if
	// optional US5 already received second).
	tailer.Stop()
	progress.Close()

	offset, whence, err := Position(registry, identifier, config.End, filetailer.NewFingerprinterMock(), opener.NewFileOpener(), nil)
	require.NoError(t, err)
	require.Equal(t, int64(len("info-1\n")), offset)
	require.Equal(t, io.SeekStart, whence)

	// Replay resumes at notice-2 and completes once US1 (or MRF in RequireAny) delivers it.
	replayOut := make(chan *message.Message, 2)
	replayTailer := makeTailer(replayOut)
	require.NoError(t, replayTailer.Start(offset, whence))
	defer replayTailer.Stop()
	replayed := read(replayOut)
	require.Equal(t, "notice-2", string(replayed.GetContent()))

	replaySinkCh := &replaySink{ch: make(chan *message.Payload, 2)}
	replayProgress, err := routing.NewProgressWithMode(2, replaySinkCh, routing.CompletionRequireAny)
	require.NoError(t, err)
	defer replayProgress.Close()
	_, err = replayProgress.Register(context.Background(), replayed, routing.Selection{Generation: 2}, []string{"ep-0", "ep-mrf"})
	require.NoError(t, err)
	// MRF delivery satisfies RequireAny and advances the file checkpoint.
	complete(replayProgress, "ep-mrf", replayed)
	select {
	case cp := <-replaySinkCh.ch:
		require.Equal(t, strconv.Itoa(len("info-1\nnotice-2\n")), cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("replayed file checkpoint did not advance via RequireAny")
	}
}
