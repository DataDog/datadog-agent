// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !windows

package file

import (
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// statCountingFile witnesses that the accounting ran: only a loss stats the file.
type statCountingFile struct {
	*opener.MockFile
	stats atomic.Int64
}

func (f *statCountingFile) Stat() (os.FileInfo, error) {
	f.stats.Add(1)
	return f.MockFile.Stat()
}

// A tailer with no filesystem or pipeline behind it; callers drive
// StopAfterFileRotation directly. fileSize backs the handle that path stats to
// size the loss.
func newMissedBytesTailer(t *testing.T, readOffset, fileSize int64) (*Tailer, *statCountingFile) {
	t.Helper()

	const path = "rotated.log"
	source := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type:    config.FileType,
		Path:    path,
		Source:  "missed-bytes-source",
		Service: "missed-bytes-service",
	}))
	info := status.NewInfoRegistry()

	tailer := NewTailer(&TailerOptions{
		OutputChan:      make(chan *message.Message, 1),
		File:            NewFile(path, source.UnderlyingSource(), false),
		SleepDuration:   time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewMockFileOpener(),
	})
	tailer.lastReadOffset.Store(readOffset)
	// Post-rotation reads land after StopAfterFileRotation returns, inside this window.
	tailer.closeTimeout = 500 * time.Millisecond
	osFile := &statCountingFile{MockFile: opener.NewMockFile(path, [][]byte{make([]byte, fileSize)})}
	tailer.osFile = osFile

	return tailer, osFile
}

// The loss-is-recorded case lives in TestStopAfterFileRotationRealFileMissedBytes,
// which covers it with a real file. These are the ways a rotation records nothing.
func TestStopAfterFileRotationNoMissedBytes(t *testing.T) {
	tests := []struct {
		name       string
		readOffset int64
		fileSize   int64
		// Read between the rotation and the close timeout. Non-zero is the
		// pre-existing condition for a loss to be accounted at all.
		readAfterRotation int64
		// Keeps these cases from passing on an unrun path.
		wantFileSized bool
	}{
		{"a fully read file lost nothing", 4096, 4096, 512, true},
		{"a truncated file is unquantifiable, not lossless", 4096, 0, 512, true},
		{"nothing read after the rotation leaves the loss unreported", 1024, 4096, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)

			tailer, osFile := newMissedBytesTailer(t, tc.readOffset, tc.fileSize)
			tailer.StopAfterFileRotation()
			tailer.bytesRead.Add(tc.readAfterRotation)

			// The goroutine signals stop after the accounting, so this synchronizes
			// rather than polls.
			select {
			case <-tailer.stop:
			case <-time.After(10 * time.Second):
				t.Fatal("rotation close goroutine never finished")
			}

			if tc.wantFileSized {
				require.NotZero(t, osFile.stats.Load(), "close path never sized the file, so the assertions below prove nothing")
			} else {
				require.Zero(t, osFile.stats.Load(), "close path sized the file even though nothing was read after the rotation")
			}

			require.Empty(t, metrics.MissedBytesSnapshot())
		})
	}
}

// The mock file cannot show a real rotation: the tailer stats an already-renamed
// path through a handle opened before the rename.
func TestStopAfterFileRotationRealFileMissedBytes(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)

	const fileSize, readOffset = 4096, 1024
	path := filepath.Join(t.TempDir(), "real-rotated.log")
	require.NoError(t, os.WriteFile(path, make([]byte, fileSize), 0o600))

	source := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type:    config.FileType,
		Path:    path,
		Source:  "real-file-source",
		Service: "real-file-service",
	}))
	info := status.NewInfoRegistry()
	tailer := NewTailer(&TailerOptions{
		OutputChan:      make(chan *message.Message, 1),
		File:            NewFile(path, source.UnderlyingSource(), false),
		SleepDuration:   time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(source, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditor.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	})
	require.NoError(t, tailer.setup(readOffset, io.SeekStart))
	tailer.closeTimeout = 500 * time.Millisecond

	require.NoError(t, os.Rename(path, path+".1"))
	tailer.StopAfterFileRotation()
	tailer.bytesRead.Add(512)

	select {
	case <-tailer.stop:
	case <-time.After(10 * time.Second):
		t.Fatal("rotation close goroutine never finished")
	}

	summaries := metrics.MissedBytesSnapshot()
	require.Len(t, summaries, 1)
	require.Equal(t, "real-file-source", summaries[0].Source)
	require.Equal(t, "real-file-service", summaries[0].Service)
	require.Equal(t, int64(fileSize-readOffset), summaries[0].Bytes)
	require.Equal(t, int64(1), summaries[0].Rotations)
}
