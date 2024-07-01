// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && npm

package connection

import (
	"encoding/binary"
	"hash"

	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type cookieHasher struct {
	hash hash.Hash64
	buf  []byte
}

func newCookieHasher() *cookieHasher {
	return &cookieHasher{
		hash: murmur3.New64(),
		buf:  make([]byte, network.ConnectionByteKeyMaxLen),
	}
}

func (h *cookieHasher) Hash(stats *network.ConnectionStats) {
	h.hash.Reset()
	if err := binary.Write(h.hash, binary.BigEndian, stats.Cookie); err != nil {
		log.Errorf("error writing cookie to hash: %s", err)
		return
	}
	key := stats.ByteKey(h.buf)
	if _, err := h.hash.Write(key); err != nil {
		log.Errorf("error writing byte key to hash: %s", err)
		return
	}
	stats.Cookie = h.hash.Sum64()
}
