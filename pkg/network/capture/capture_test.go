// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCapturer(t *testing.T) {
	t.Run("nil output is rejected", func(t *testing.T) {
		_, err := newCapturer(CaptureConfig{})
		assert.Error(t, err)
	})

	t.Run("invalid filter syntax is rejected", func(t *testing.T) {
		_, err := newCapturer(CaptureConfig{Output: &bytes.Buffer{}, Filter: "not a valid bpf filter (("})
		assert.Error(t, err)
	})

	t.Run("valid filter is accepted", func(t *testing.T) {
		_, err := newCapturer(CaptureConfig{Output: &bytes.Buffer{}, Filter: "tcp port 443"})
		assert.NoError(t, err)
	})

	t.Run("Ethernet-only filter is accepted", func(t *testing.T) {
		_, err := newCapturer(CaptureConfig{Output: &bytes.Buffer{}, Interface: "eth0", Filter: "ether host 02:00:00:00:00:01"})
		assert.NoError(t, err)
	})

	t.Run("explicit interface is preserved", func(t *testing.T) {
		c, err := newCapturer(CaptureConfig{Output: &bytes.Buffer{}, Interface: "eth0"})
		require.NoError(t, err)
		assert.Equal(t, "eth0", c.cfg.Interface)
	})
}

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name        string
		cfg         CaptureConfig
		wantIface   string
		wantSnapLen uint32
	}{
		{"empty config captures every interface at the ceiling", CaptureConfig{}, anyInterface, maxSnapLen},
		{"snap length below the ceiling is preserved", CaptureConfig{SnapLen: 96}, anyInterface, 96},
		{"snap length above the ceiling is capped", CaptureConfig{SnapLen: 256}, anyInterface, maxSnapLen},
		{"named interface is preserved", CaptureConfig{Interface: "ens5"}, "ens5", maxSnapLen},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.applyDefaults()
			assert.Equal(t, tt.wantIface, cfg.Interface)
			assert.Equal(t, tt.wantSnapLen, cfg.SnapLen)
		})
	}
}

// Done must close when the capture ends on its own, so callers return as soon
// as the duration elapses instead of waiting out their own timeout.
func TestDoneClosesWhenDurationElapses(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("opening a libpcap handle requires root")
	}

	c, err := newCapturer(CaptureConfig{Output: &bytes.Buffer{}, Interface: "lo", Duration: time.Second})
	require.NoError(t, err)
	require.NoError(t, c.Start(context.Background()))
	defer c.Stop()

	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Done was not closed after the capture duration elapsed")
	}
}
