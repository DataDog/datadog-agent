// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package filter

import (
	"sync"
	"time"

	"github.com/google/gopacket"
)

// PacketVisitor consumes one packet while the source-owned buffer is valid.
type PacketVisitor func(data []byte, info PacketInfo, timestamp time.Time) error

// PacketSourceFanout gives one owner responsibility for a capture source while
// synchronously distributing each packet to non-owning consumers. Synchronous
// delivery avoids packet copies and prevents consumers from retaining pooled
// libpcap buffers.
type PacketSourceFanout struct {
	source PacketSource

	mu          sync.RWMutex
	nextID      uint64
	subscribers map[uint64]PacketVisitor
	closeOnce   sync.Once
}

// NewPacketSourceFanout creates an owner for source.
func NewPacketSourceFanout(source PacketSource) *PacketSourceFanout {
	return &PacketSourceFanout{
		source:      source,
		subscribers: make(map[uint64]PacketVisitor),
	}
}

// Subscribe adds a non-owning packet consumer and returns an idempotent
// unsubscribe function.
func (f *PacketSourceFanout) Subscribe(visitor PacketVisitor) func() {
	f.mu.Lock()
	id := f.nextID
	f.nextID++
	f.subscribers[id] = visitor
	f.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			delete(f.subscribers, id)
			f.mu.Unlock()
		})
	}
}

// VisitPackets distributes packets to subscribers before invoking visitor.
func (f *PacketSourceFanout) VisitPackets(visitor func(data []byte, info PacketInfo, timestamp time.Time) error) error {
	return f.source.VisitPackets(func(data []byte, info PacketInfo, timestamp time.Time) error {
		f.mu.RLock()
		subscribers := make([]PacketVisitor, 0, len(f.subscribers))
		for _, subscriber := range f.subscribers {
			subscribers = append(subscribers, subscriber)
		}
		f.mu.RUnlock()
		for _, subscriber := range subscribers {
			if err := subscriber(data, info, timestamp); err != nil {
				return err
			}
		}
		if visitor != nil {
			return visitor(data, info, timestamp)
		}
		return nil
	})
}

// LayerType returns the owned source's default link-layer type.
func (f *PacketSourceFanout) LayerType() gopacket.LayerType {
	return f.source.LayerType()
}

// Close closes the owned source exactly once.
func (f *PacketSourceFanout) Close() {
	f.closeOnce.Do(f.source.Close)
}

var _ PacketSource = (*PacketSourceFanout)(nil)
