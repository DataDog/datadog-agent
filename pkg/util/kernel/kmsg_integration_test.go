// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package kernel

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
)

func TestKmsgReaderIntegration(t *testing.T) {
	preStartMarker := fmt.Sprintf("datadog-kmsg-reader-pre-start-%d", time.Now().UnixNano())
	postStartMarker := fmt.Sprintf("datadog-kmsg-reader-post-start-%d", time.Now().UnixNano())
	require.NoError(t, writeKmsgMarker(preStartMarker))

	reader, err := NewKmsgReader(telemetrymock.New(t))
	require.NoError(t, err)
	t.Cleanup(reader.Stop)

	records, unsubscribe, err := reader.Subscribe("integration-test", func(record KmsgRecord) bool {
		return strings.Contains(record.Message, preStartMarker) || strings.Contains(record.Message, postStartMarker)
	})
	require.NoError(t, err)
	t.Cleanup(unsubscribe)

	require.NoError(t, writeKmsgMarker(postStartMarker))

	select {
	case record := <-records:
		require.Contains(t, record.Message, postStartMarker)
		require.NotContains(t, record.Message, preStartMarker)
		require.NotZero(t, record.Sequence)
		require.NotZero(t, record.Timestamp)
	case err := <-reader.Errors():
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for kmsg marker")
	}
}

func TestKmsgReaderStopCancelsIdleReadIntegration(t *testing.T) {
	reader, err := NewKmsgReader(telemetrymock.New(t))
	require.NoError(t, err)

	stopped := make(chan struct{})
	go func() {
		reader.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out stopping kmsg reader")
	}
}

func writeKmsgMarker(marker string) error {
	fd, err := unix.Open(kmsgPath, unix.O_WRONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s for writing: %w", kmsgPath, err)
	}
	file := os.NewFile(uintptr(fd), kmsgPath)
	defer file.Close()

	if _, err := fmt.Fprintf(file, "<6>%s\n", marker); err != nil {
		return fmt.Errorf("write kmsg marker: %w", err)
	}
	return nil
}
