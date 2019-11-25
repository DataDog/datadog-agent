// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package listeners

// Packet represents a statsd packet ready to process,
// with its origin metadata if applicable.
//
// As the Packet object is reused in a sync.Pool, we keep the
// underlying buffer reference to avoid re-sizing the slice
// before reading
type Packet struct {
	Contents []byte // Contents, might contain several messages
	buffer   []byte // Underlying buffer for data read
	Origin   string // Origin container if identified
}

// Packets is a slice of packet pointers
type Packets []*Packet

// StatsdListener opens a communication channel to get statsd packets in.
type StatsdListener interface {
	Listen()
	Stop()
}

// NoOrigin is returned if origin detection is off or failed.
const NoOrigin = ""
