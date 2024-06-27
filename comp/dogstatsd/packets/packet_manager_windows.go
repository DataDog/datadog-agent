// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package packets

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// PacketManager gathers everything required to create and assemble packets.
type PacketManager struct {
	PacketsBuffer   *Buffer
	PacketAssembler *Assembler
	bufferSize      int
}

// NewPacketManagerFromConfig creates a PacketManager from the relevant config settings.
func NewPacketManagerFromConfig(packetOut chan Packets, sharedPacketPoolManager *PoolManager[Packet], cfg config.Reader) *PacketManager {
	bufferSize := cfg.GetInt("dogstatsd_buffer_size")
	packetsBufferSize := cfg.GetInt("dogstatsd_packet_buffer_size")
	flushTimeout := cfg.GetDuration("dogstatsd_packet_buffer_flush_timeout")

	return NewPacketManager(bufferSize, packetsBufferSize, flushTimeout, packetOut, sharedPacketPoolManager)
}

// NewPacketManager instantiates a PacketManager
func NewPacketManager(
	bufferSize int,
	packetsBufferSize int,
	flushTimeout time.Duration,
	packetOut chan Packets,
	sharedPacketPoolManager *PoolManager[Packet]) *PacketManager {

	packetsBuffer := NewBuffer(uint(packetsBufferSize), flushTimeout, packetOut, "named_pipe")

	return &PacketManager{
		bufferSize:      bufferSize,
		PacketsBuffer:   packetsBuffer,
		PacketAssembler: NewAssembler(flushTimeout, packetsBuffer, sharedPacketPoolManager, NamedPipe),
	}
}

// CreateBuffer creates a new buffer
func (l *PacketManager) CreateBuffer() []byte {
	return make([]byte, l.bufferSize)
}

// Close closes the PacketManager.
func (l *PacketManager) Close() {
	l.PacketAssembler.Close()
	l.PacketsBuffer.Close()
}
