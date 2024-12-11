// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package packets

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util"
)

// SourceType is the type of listener
type SourceType int

const (
	// UDP listener
	UDP SourceType = iota
	// UDS listener
	UDS
	// NamedPipe Windows named pipe listner
	NamedPipe
)

// Packet represents a statsd packet ready to process,
// with its origin metadata if applicable.
//
// As the Packet object is reused in a sync.Pool, we keep the
// underlying buffer reference to avoid re-sizing the slice
// before reading
type Packet struct {
	Contents   []byte     // Contents, might contain several messages
	Buffer     []byte     // Underlying buffer for data read
	Origin     string     // Origin container if identified
	ListenerID string     // Listener ID
	Source     SourceType // Type of listener that produced the packet
}

// Packets is a slice of packet pointers
type Packets []*Packet

// NoOrigin is returned if origin detection is off or failed.
const NoOrigin = ""

// SizeOfPacket is the size of a packet structure in bytes
const SizeOfPacket = unsafe.Sizeof(Packet{})

// SizeInBytes returns the size of the packet in bytes
func (p *Packet) SizeInBytes() int {
	return int(SizeOfPacket)
}

// DataSizeInBytes returns the size of the packet data in bytes
func (p *Packet) DataSizeInBytes() int {
	return len(p.Contents) + len(p.Buffer) + len(p.Origin) + len(p.ListenerID)
}

var _ util.HasSizeInBytes = (*Packet)(nil)

// SizeInBytes returns the size of the packets in bytes
func (ps *Packets) SizeInBytes() int {
	return len(*ps) * (*Packet)(nil).SizeInBytes()
}

// DataSizeInBytes returns the size of the packets data in bytes
func (ps *Packets) DataSizeInBytes() int {
	size := 0
	for _, p := range *ps {
		size += p.DataSizeInBytes()
	}
	return size
}

var _ util.HasSizeInBytes = (*Packets)(nil)
