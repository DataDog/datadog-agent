// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !windows

package file

import (
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

// A tailer with no filesystem or pipeline behind it; callers drive
// StopAfterFileRotation directly. fileSize backs the handle that path stats to
// size the loss.
func newMissedBytesTailer(t *testing.T, readOffset, fileSize int64) *Tailer {
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
	tailer.closeTimeout = 10 * time.Millisecond
	tailer.osFile = opener.NewMockFile(path, [][]byte{make([]byte, fileSize)})

	return tailer
}

func TestStopAfterFileRotationMissedBytes(t *testing.T) {
	tests := []struct {
		name       string
		readOffset int64
		fileSize   int64
		// Read between the rotation and the close timeout. Non-zero is the
		// pre-existing condition for a loss to be accounted at all.
		readAfterRotation int64
		wantBytes         int64
	}{
		{"loss is recorded against the source and service", 1024, 4096, 512, 3072},
		{"a fully read file lost nothing", 4096, 4096, 512, 0},
		{"a truncated file is unquantifiable, not lossless", 4096, 0, 512, 0},
		{"nothing read after the rotation leaves the loss unreported", 1024, 4096, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)

			tailer := newMissedBytesTailer(t, tc.readOffset, tc.fileSize)
			tailer.StopAfterFileRotation()
			tailer.bytesRead.Add(tc.readAfterRotation)

			// The goroutine signals stop after the accounting, so this synchronizes
			// rather than polls.
			select {
			case <-tailer.stop:
			case <-time.After(10 * time.Second):
				t.Fatal("rotation close goroutine never finished")
			}

			summaries := metrics.MissedBytesSnapshot()
			if tc.wantBytes == 0 {
				require.Empty(t, summaries)
				return
			}

			require.Len(t, summaries, 1)
			require.Equal(t, "missed-bytes-source", summaries[0].Source)
			require.Equal(t, "missed-bytes-service", summaries[0].Service)
			require.Equal(t, tc.wantBytes, summaries[0].Bytes)
			require.Equal(t, int64(1), summaries[0].Rotations)
		})
	}
}
