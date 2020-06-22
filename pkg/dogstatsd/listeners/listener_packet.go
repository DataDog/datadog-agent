// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package listeners

import "github.com/DataDog/datadog-agent/pkg/config"

type listenerPacket struct {
	packetsBuffer   *packetsBuffer
	packetAssembler *packetAssembler
	buffer          []byte
}

func newListenerPacket(packetOut chan Packets, sharedPacketPool *PacketPool) *listenerPacket {
	bufferSize := config.Datadog.GetInt("dogstatsd_buffer_size")
	packetsBufferSize := config.Datadog.GetInt("dogstatsd_packet_buffer_size")
	flushTimeout := config.Datadog.GetDuration("dogstatsd_packet_buffer_flush_timeout")

	packetsBuffer := newPacketsBuffer(uint(packetsBufferSize), flushTimeout, packetOut)

	return &listenerPacket{
		buffer:          make([]byte, bufferSize),
		packetsBuffer:   packetsBuffer,
		packetAssembler: newPacketAssembler(flushTimeout, packetsBuffer, sharedPacketPool),
	}
}

func (l *listenerPacket) close() {
	l.packetAssembler.close()
	l.packetsBuffer.close()
}
