// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type packetManager struct {
	packetsBuffer   *packetsBuffer
	packetAssembler *packetAssembler
	buffer          []byte
}

func newPacketManagerFromConfig(packetOut chan Packets, sharedPacketPool *PacketPool) *packetManager {
	bufferSize := config.Datadog.GetInt("dogstatsd_buffer_size")
	packetsBufferSize := config.Datadog.GetInt("dogstatsd_packet_buffer_size")
	flushTimeout := config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout")

	return newPacketManager(bufferSize, packetsBufferSize, flushTimeout, packetOut, sharedPacketPool)
}

func newPacketManager(
	bufferSize int,
	packetsBufferSize int,
	flushTimeout time.Duration,
	packetOut chan Packets,
	sharedPacketPool *PacketPool) *packetManager {

	packetsBuffer := newPacketsBuffer(uint(packetsBufferSize), flushTimeout, packetOut)

	return &packetManager{
		buffer:          make([]byte, bufferSize),
		packetsBuffer:   packetsBuffer,
		packetAssembler: newPacketAssembler(flushTimeout, packetsBuffer, sharedPacketPool),
	}
}

func (l *packetManager) close() {
	l.packetAssembler.close()
	l.packetsBuffer.close()
}
