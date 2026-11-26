// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeMemory map[uint64][]byte

func (m fakeMemory) Read(addr uint64, buf []byte) error {
	src, ok := m[addr]
	if !ok || len(src) < len(buf) {
		return fmt.Errorf("unmapped address %#x", addr)
	}
	copy(buf, src[:len(buf)])
	return nil
}

func makeHeaderBytes(sig [8]byte, version, payloadSize uint32, publishedAtNs, payloadPtr uint64) []byte {
	buf := make([]byte, headerSize)
	copy(buf[0:8], sig[:])
	binary.NativeEndian.PutUint32(buf[8:12], version)
	binary.NativeEndian.PutUint32(buf[12:16], payloadSize)
	binary.NativeEndian.PutUint64(buf[16:24], publishedAtNs)
	binary.NativeEndian.PutUint64(buf[24:32], payloadPtr)
	return buf
}

func TestParseHeaderClassification(t *testing.T) {
	t.Run("zero signature is unpublished", func(t *testing.T) {
		buf := make([]byte, headerSize)
		_, err := parseHeader(buf)
		require.ErrorIs(t, err, ErrUnpublished)
		require.NotErrorIs(t, err, ErrMalformed)

		mem := fakeMemory{0x1000: buf}
		_, err = Read(mem, 0x1000)
		require.ErrorIs(t, err, ErrUnpublished)
		require.NotErrorIs(t, err, ErrMalformed)
	})

	t.Run("non-zero invalid signature is malformed", func(t *testing.T) {
		buf := makeHeaderBytes([8]byte{'B', 'A', 'D', '_', 'S', 'I', 'G', '!'}, headerVersion, 16, 1, 0x2000)
		_, err := parseHeader(buf)
		require.ErrorIs(t, err, ErrMalformed)
		require.NotErrorIs(t, err, ErrUnpublished)
	})
}
