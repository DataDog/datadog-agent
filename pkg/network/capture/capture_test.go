// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildRawRecord assembles a raw ring buffer sample with the given metadata
// and trailing data, mirroring the layout buildProgram's eBPF code writes.
func buildRawRecord(t *testing.T, origLen, ifindex, ingress, capLen uint32, data []byte) []byte {
	t.Helper()

	buf := make([]byte, ringBufMetaSize+len(data))
	le := binary.LittleEndian
	le.PutUint64(buf[0:8], 123456789) // ktime_ns — value irrelevant to parseRecord
	le.PutUint32(buf[8:12], origLen)
	le.PutUint32(buf[12:16], ifindex)
	le.PutUint32(buf[16:20], ingress)
	le.PutUint32(buf[20:24], capLen)
	copy(buf[24:], data)
	return buf
}

func TestParseRecord(t *testing.T) {
	t.Run("normal record with caplen matching data length", func(t *testing.T) {
		data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		raw := buildRawRecord(t, 1500, 2, 1, uint32(len(data)), data)

		pkt, ok := parseRecord(raw)
		require.True(t, ok)
		assert.Equal(t, uint32(1500), pkt.OrigLen)
		assert.Equal(t, uint32(2), pkt.IfIndex)
		assert.True(t, pkt.Ingress)
		assert.Equal(t, data, pkt.Data)
	})

	t.Run("egress packet decoded correctly", func(t *testing.T) {
		data := []byte{0x01}
		raw := buildRawRecord(t, 64, 3, 0, uint32(len(data)), data)

		pkt, ok := parseRecord(raw)
		require.True(t, ok)
		assert.False(t, pkt.Ingress)
	})

	t.Run("caplen shorter than reservation truncates trailing garbage", func(t *testing.T) {
		// The eBPF reservation is sized to snapLen, but only capLen bytes were
		// actually loaded by bpf_skb_load_bytes — everything after that is
		// uninitialized/garbage and must not leak into pkt.Data.
		reserved := make([]byte, 32)
		copy(reserved, []byte{0x11, 0x22, 0x33})
		for i := 3; i < len(reserved); i++ {
			reserved[i] = 0xFF // simulate garbage past the actually-loaded bytes
		}
		raw := buildRawRecord(t, 3, 1, 1, 3, reserved)

		pkt, ok := parseRecord(raw)
		require.True(t, ok)
		assert.Equal(t, []byte{0x11, 0x22, 0x33}, pkt.Data)
	})

	t.Run("record shorter than metadata size is rejected", func(t *testing.T) {
		raw := make([]byte, ringBufMetaSize-1)
		_, ok := parseRecord(raw)
		assert.False(t, ok)
	})

	t.Run("record with exactly metadata size and no data is accepted", func(t *testing.T) {
		raw := buildRawRecord(t, 0, 0, 0, 0, nil)
		pkt, ok := parseRecord(raw)
		require.True(t, ok)
		assert.Empty(t, pkt.Data)
	})

	t.Run("malformed caplen exceeding available bytes is clamped defensively", func(t *testing.T) {
		data := []byte{0x01, 0x02}
		// capLen claims 10 bytes, but only 2 bytes of data actually follow the
		// metadata — parseRecord must not read out of bounds.
		raw := buildRawRecord(t, 2, 1, 1, 10, data)

		pkt, ok := parseRecord(raw)
		require.True(t, ok)
		assert.Equal(t, data, pkt.Data)
	})
}

func TestNewCapturer(t *testing.T) {
	validCfg := func() CaptureConfig {
		return CaptureConfig{
			Iface:  &net.Interface{Name: "lo", Index: 1},
			Output: &bytes.Buffer{},
		}
	}

	t.Run("nil interface is rejected", func(t *testing.T) {
		cfg := validCfg()
		cfg.Iface = nil
		_, err := newCapturer(cfg)
		assert.Error(t, err)
	})

	t.Run("nil output is rejected", func(t *testing.T) {
		cfg := validCfg()
		cfg.Output = nil
		_, err := newCapturer(cfg)
		assert.Error(t, err)
	})

	t.Run("defaults are applied", func(t *testing.T) {
		cfg := validCfg()
		c, err := newCapturer(cfg)
		require.NoError(t, err)
		assert.Equal(t, uint32(defaultSnapLen), c.snapLen)
		assert.Equal(t, defaultRingBufferSize, c.cfg.RingBufferSize)
	})

	t.Run("explicit snaplen and ring buffer size are preserved", func(t *testing.T) {
		cfg := validCfg()
		cfg.SnapLen = 128
		cfg.RingBufferSize = 4096
		c, err := newCapturer(cfg)
		require.NoError(t, err)
		assert.Equal(t, uint32(128), c.snapLen)
		assert.Equal(t, 4096, c.cfg.RingBufferSize)
	})

	t.Run("empty filter compiles to a nil (match-all) program", func(t *testing.T) {
		cfg := validCfg()
		c, err := newCapturer(cfg)
		require.NoError(t, err)
		assert.Nil(t, c.filterInsts)
	})

	t.Run("valid filter compiles to non-empty eBPF instructions", func(t *testing.T) {
		cfg := validCfg()
		cfg.Filter = "icmp"
		c, err := newCapturer(cfg)
		require.NoError(t, err)
		assert.NotEmpty(t, c.filterInsts)
	})

	t.Run("invalid filter syntax is rejected", func(t *testing.T) {
		cfg := validCfg()
		cfg.Filter = "not a valid bpf filter (("
		_, err := newCapturer(cfg)
		assert.Error(t, err)
	})
}

func TestDirectionAllowed(t *testing.T) {
	tests := []struct {
		name      string
		direction CaptureDirection
		ingress   bool
		want      bool
	}{
		{"both allows ingress", DirectionBoth, true, true},
		{"both allows egress", DirectionBoth, false, true},
		{"ingress-only allows ingress", DirectionIngress, true, true},
		{"ingress-only blocks egress", DirectionIngress, false, false},
		{"egress-only allows egress", DirectionEgress, false, true},
		{"egress-only blocks ingress", DirectionEgress, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &capturer{cfg: CaptureConfig{Direction: tt.direction}}
			assert.Equal(t, tt.want, c.directionAllowed(tt.ingress))
		})
	}
}
