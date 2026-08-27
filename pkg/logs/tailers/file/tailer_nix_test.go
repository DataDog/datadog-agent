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

// newMissedBytesTailer builds a tailer with no filesystem or pipeline behind it.
// Callers arm the platform's own loss measurement and drive
// StopAfterFileRotation directly.
func newMissedBytesTailer(t *testing.T, readOffset int64) *Tailer {
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

	return tailer
}

// awaitRotationClose waits for the goroutine StopAfterFileRotation spawned.
// It signals t.stop after the accounting, so this is a synchronization point
// rather than a poll.
func awaitRotationClose(t *testing.T, tailer *Tailer) {
	t.Helper()
	select {
	case <-tailer.stop:
	case <-time.After(10 * time.Second):
		t.Fatal("rotation close goroutine never finished")
	}
}

// armRotationLoss gives the tailer an open handle reporting fileSize, which is
// what StopAfterFileRotation stats to size the loss.
func armRotationLoss(t *testing.T, tailer *Tailer, fileSize int64) {
	t.Helper()
	tailer.osFile = opener.NewMockFile(tailer.file.Path, [][]byte{make([]byte, fileSize)})
}

// stopAfterRotationHavingRead drives a rotation where the tailer read n more
// bytes before the close timeout expired, which is the pre-existing condition
// for the loss to be accounted for at all.
func stopAfterRotationHavingRead(t *testing.T, tailer *Tailer, n int64) {
	t.Helper()
	tailer.StopAfterFileRotation()
	tailer.bytesRead.Add(n)
	awaitRotationClose(t, tailer)
}

func TestStopAfterFileRotationRecordsMissedBytes(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	defer metrics.ResetMissedBytesForTest()

	tailer := newMissedBytesTailer(t, 1024)
	armRotationLoss(t, tailer, 4096)
	stopAfterRotationHavingRead(t, tailer, 512)

	summaries := metrics.MissedBytesSnapshot()
	require.Len(t, summaries, 1)
	require.Equal(t, "missed-bytes-source", summaries[0].Source)
	require.Equal(t, "missed-bytes-service", summaries[0].Service)
	require.Equal(t, int64(3072), summaries[0].Bytes)
	require.Equal(t, int64(1), summaries[0].Rotations)
}

// TestStopAfterFileRotationFullyRead is the common rotation: the tailer finished
// the file, so there is no loss and no issue to raise.
func TestStopAfterFileRotationFullyRead(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	defer metrics.ResetMissedBytesForTest()

	tailer := newMissedBytesTailer(t, 4096)
	armRotationLoss(t, tailer, 4096)
	stopAfterRotationHavingRead(t, tailer, 512)

	require.Empty(t, metrics.MissedBytesSnapshot())
}

// TestStopAfterFileRotationTruncated covers the offset outrunning the file, which
// is loss the tailer cannot quantify rather than loss of zero bytes.
func TestStopAfterFileRotationTruncated(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	defer metrics.ResetMissedBytesForTest()

	tailer := newMissedBytesTailer(t, 4096)
	armRotationLoss(t, tailer, 0)
	stopAfterRotationHavingRead(t, tailer, 512)

	require.Empty(t, metrics.MissedBytesSnapshot())
}

// TestStopAfterFileRotationNothingReadAfterTimeout pins the pre-existing gate:
// with no read after the rotation, the loss goes unreported here.
func TestStopAfterFileRotationNothingReadAfterTimeout(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	defer metrics.ResetMissedBytesForTest()

	tailer := newMissedBytesTailer(t, 1024)
	armRotationLoss(t, tailer, 4096)
	stopAfterRotationHavingRead(t, tailer, 0)

	require.Empty(t, metrics.MissedBytesSnapshot())
}
