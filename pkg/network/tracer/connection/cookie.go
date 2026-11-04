// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build (linux && bpf) || darwin

// Package connection provides the connection tracer interface and implementations.
package connection

import (
	"encoding/binary"

	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/network"
)

type cookieHasher struct {
	// big-endian cookie followed by the connection byte key
	buf [8 + network.ConnectionByteKeyMaxLen]byte
}

func newCookieHasher() *cookieHasher {
	return &cookieHasher{}
}

func (h *cookieHasher) Hash(stats *network.ConnectionStats) {
	binary.BigEndian.PutUint64(h.buf[:8], stats.Cookie)
	key := stats.ByteKey(h.buf[8:])
	stats.Cookie = murmur3.Sum64(h.buf[:8+len(key)])
}
