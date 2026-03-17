// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package filter

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	subSourceChannelSize = 512 // max size needed for the dns, normal tracer only needs 134
)

// subPacket is a copied packet event sent through a SubSource channel.
// DarwinPacketInfo is copied by value so each sub owns its metadata independently.
type subPacket struct {
	data      []byte
	info      DarwinPacketInfo
	timestamp time.Time
}

// SubSource is a PacketSource fed by a MultiPacketSource. Each SubSource
// receives a copy of every packet that passes its predicate. Buffers are
// returned to the shared pool after the visitor returns.
type SubSource struct {
	ch        chan subPacket
	exit      chan struct{}
	predicate func([]byte, *DarwinPacketInfo) bool // nil means accept all
	pool      *sync.Pool
	multi     *MultiPacketSource
	closeOnce sync.Once
}

// VisitPackets drains the sub-source channel and calls visitor for each packet.
// The buffer passed to visitor is only valid for the duration of the call —
// after visitor returns, the buffer is returned to the shared pool.
func (s *SubSource) VisitPackets(visitor func(data []byte, info PacketInfo, timestamp time.Time) error) error {
	// Lazy-start the MultiPacketSource fan-out goroutine on first VisitPackets call.
	s.multi.startOnce.Do(func() {
		s.multi.wg.Add(1)
		go func() {
			defer s.multi.wg.Done()
			//nolint:errcheck
			s.multi.source.VisitPackets(s.multi.dispatch)
		}()
	})

	pktInfo := &DarwinPacketInfo{}
	for {
		select {
		case pkt, ok := <-s.ch:
			if !ok {
				return nil
			}
			*pktInfo = pkt.info
			err := visitor(pkt.data, pktInfo, pkt.timestamp)
			s.pool.Put(pkt.data[:cap(pkt.data)])
			if err != nil {
				return err
			}
		case <-s.exit:
			return nil
		}
	}
}

// LayerType returns LayerTypeEthernet. The accurate per-packet layer type is
// available via DarwinPacketInfo.LayerType passed to the visitor.
func (s *SubSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

// Close stops this SubSource and decrements the MultiPacketSource ref count.
// Safe to call multiple times.
func (s *SubSource) Close() {
	s.closeOnce.Do(func() {
		close(s.exit)
		s.multi.subClosed()
	})
}

// MultiPacketSource fans a single PacketSource out to multiple SubSources.
// It owns a shared sync.Pool for packet buffers; each SubSource holds a
// reference to this pool and returns buffers after each visitor call.
type MultiPacketSource struct {
	source    PacketSource
	pool      sync.Pool
	subs      []*SubSource
	startOnce sync.Once
	closeOnce sync.Once
	wg        sync.WaitGroup

	// activeCount tracks how many SubSources are still open.
	// When it reaches zero, the underlying source is closed.
	activeCount atomic.Int32
}

// newSubSource registers a new SubSource against this MultiPacketSource.
// Must be called before the first VisitPackets call on any sub.
func (m *MultiPacketSource) newSubSource(predicate func([]byte, *DarwinPacketInfo) bool) *SubSource {
	sub := &SubSource{
		ch:        make(chan subPacket, subSourceChannelSize),
		exit:      make(chan struct{}),
		predicate: predicate,
		pool:      &m.pool,
		multi:     m,
	}
	m.subs = append(m.subs, sub)
	m.activeCount.Add(1)
	return sub
}

// dispatch is the visitor passed to source.VisitPackets. It copies each packet
// into a pool buffer and fans it to each SubSource whose predicate matches.
// Packets are dropped (never block) if a SubSource's channel is full.
func (m *MultiPacketSource) dispatch(data []byte, info PacketInfo, ts time.Time) error {
	darwinInfo, _ := info.(*DarwinPacketInfo)
	if darwinInfo == nil {
		darwinInfo = &DarwinPacketInfo{}
	}

	for _, sub := range m.subs {
		if sub.predicate != nil && !sub.predicate(data, darwinInfo) {
			continue
		}

		buf := m.pool.Get().([]byte)
		buf = buf[:len(data)]
		copy(buf, data)

		select {
		case sub.ch <- subPacket{data: buf, info: *darwinInfo, timestamp: ts}:
		default:
			// Channel full — drop and recycle immediately.
			m.pool.Put(buf[:cap(buf)])
		}
	}
	return nil
}

// subClosed is called by SubSource.Close(). When the last sub closes,
// the underlying PacketSource is also closed.
func (m *MultiPacketSource) subClosed() {
	if m.activeCount.Add(-1) == 0 {
		m.closeOnce.Do(func() {
			m.source.Close()
		})
		m.wg.Wait()
	}
}

// IsDNSPacket is a lightweight predicate that returns true if the packet
// carries traffic on port 53 (UDP or TCP). It does not perform a full DNS
// parse — it only inspects the transport-layer source and destination ports.
func IsDNSPacket(data []byte, info *DarwinPacketInfo) bool {
	var ipOffset int
	if info.LayerType == layers.LayerTypeEthernet {
		ipOffset = 14
	} else {
		// BSD loopback (DLT_NULL / utun interfaces)
		ipOffset = 4
	}

	if len(data) < ipOffset+1 {
		return false
	}

	version := data[ipOffset] >> 4
	var protocol byte
	var transportOffset int

	switch version {
	case 4:
		if len(data) < ipOffset+20 {
			return false
		}
		protocol = data[ipOffset+9]
		ipHeaderLen := int(data[ipOffset]&0x0F) * 4
		transportOffset = ipOffset + ipHeaderLen
	case 6:
		if len(data) < ipOffset+40 {
			return false
		}
		protocol = data[ipOffset+6]
		transportOffset = ipOffset + 40
	default:
		return false
	}

	// UDP=17, TCP=6
	if protocol != 17 && protocol != 6 {
		return false
	}

	if len(data) < transportOffset+4 {
		return false
	}

	sport := binary.BigEndian.Uint16(data[transportOffset : transportOffset+2])
	dport := binary.BigEndian.Uint16(data[transportOffset+2 : transportOffset+4])
	return sport == 53 || dport == 53
}

// Package-level singleton — one MultiPacketSource shared across all callers.
var (
	globalMultiSource     *MultiPacketSource
	globalMultiSourceErr  error
	globalMultiSourceOnce sync.Once
)

// NewSubSource returns a SubSource from the shared LibpcapSource, creating it
// on the first call. predicate is an optional userspace filter; pass nil to
// receive all packets.
func NewSubSource(_ *config.Config, predicate func([]byte, *DarwinPacketInfo) bool) (*SubSource, error) {
	globalMultiSourceOnce.Do(func() {
		src, err := NewLibpcapSource()
		if err != nil {
			globalMultiSourceErr = err
			return
		}
		globalMultiSource = &MultiPacketSource{
			source: src,
		}
		globalMultiSource.pool = sync.Pool{
			New: func() interface{} {
				return make([]byte, defaultSnapLen)
			},
		}
	})
	if globalMultiSourceErr != nil {
		return nil, globalMultiSourceErr
	}
	return globalMultiSource.newSubSource(predicate), nil
}
