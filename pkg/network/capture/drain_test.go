// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCapturer returns a capturer whose drain loop reads from read and
// writes to a buffer, without a libpcap handle.
func newTestCapturer(t *testing.T, read func() ([]byte, gopacket.CaptureInfo, error)) *capturer {
	t.Helper()
	c, err := newCapturer(CaptureConfig{Output: &bytes.Buffer{}, Duration: 10 * time.Second})
	require.NoError(t, err)
	c.readPacket = read
	c.linkType = layers.LinkTypeEthernet
	c.writer = pcapgo.NewWriter(c.cfg.Output)
	require.NoError(t, c.writer.WriteFileHeader(c.cfg.SnapLen, c.linkType))
	return c
}

// A read error that repeats on every call, as libpcap returns once the
// interface is gone, must end the capture instead of spinning.
func TestDrainLoopEndsOnReadError(t *testing.T) {
	reads := 0
	c := newTestCapturer(t, func() ([]byte, gopacket.CaptureInfo, error) {
		reads++
		return nil, gopacket.CaptureInfo{}, errors.New("read error")
	})

	go c.drainLoop(context.Background())
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("drain loop did not end on a read error")
	}
	assert.Equal(t, 1, reads)
	assert.Equal(t, uint64(1), c.Stats().Errors)
}
