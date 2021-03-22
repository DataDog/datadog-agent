// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build windows

package listeners

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
)

// packetManager gathers everything required to create and assemble packets.
type packetManager struct {
	packetsBuffer   *packetsBuffer
	packetAssembler *packets.Assembler
	bufferSize      int
}

func newPacketManagerFromConfig(packetOut chan Packets, sharedPacketPoolManager *PoolManager) *packetManager {
	bufferSize := config.Datadog.GetInt("dogstatsd_buffer_size")
	packetsBufferSize := config.Datadog.GetInt("dogstatsd_packet_buffer_size")
	flushTimeout := config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout")

	return newPacketManager(bufferSize, packetsBufferSize, flushTimeout, packetOut, sharedPacketPoolManager)
}

func newPacketManager(
	bufferSize int,
	packetsBufferSize int,
	flushTimeout time.Duration,
	packetOut chan Packets,
	sharedPacketPoolManager *packets.PoolManager) *packetManager {

	packetsBuffer := packets.NewBuffer(uint(packetsBufferSize), flushTimeout, packetOut)

	return &packetManager{
		bufferSize:      bufferSize,
		packetsBuffer:   packetsBuffer,
		packetAssembler: packets.NewAssembler(flushTimeout, packetsBuffer, sharedPacketPoolManager),
	}
}

func (l *packetManager) createBuffer() []byte {
	return make([]byte, l.bufferSize)
}

func (l *packetManager) close() {
	l.packetAssembler.close()
	l.packetsBuffer.close()
}
