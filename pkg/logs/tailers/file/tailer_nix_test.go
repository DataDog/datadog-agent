// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !windows

package file

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

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
