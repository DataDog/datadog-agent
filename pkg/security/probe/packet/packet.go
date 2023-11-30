// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package packet holds packet related files
package packet

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"go.uber.org/atomic"
	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	captureLen = 65536
)

type state int

const (
	stateInit state = iota
	stateRunning
	stateStopped
)

// Manager is a manager for packet capture
type Manager struct {
	mu sync.Mutex
	wg sync.WaitGroup

	ctx                           context.Context
	state                         state
	stateChan                     chan state
	currentPacketFilterExpression string
	eventStub                     *model.Event
	onPacketEvent                 func(*model.Event)

	// stats
	pktCaptured      *atomic.Uint64
	pktBytesCaptured *atomic.Uint64
}

// NewManager returns a new Manager
func NewManager(ctx context.Context, eventStub *model.Event, onPacketEvent func(*model.Event)) *Manager {
	eventStub.Type = uint32(model.PacketEventType)
	entry, _ := eventStub.FieldHandlers.ResolveProcessCacheEntry(eventStub)
	eventStub.ProcessCacheEntry = entry
	eventStub.ProcessContext = &eventStub.ProcessCacheEntry.ProcessContext
	return &Manager{
		ctx:              ctx,
		state:            stateInit,
		stateChan:        make(chan state),
		onPacketEvent:    onPacketEvent,
		eventStub:        eventStub,
		pktCaptured:      atomic.NewUint64(0),
		pktBytesCaptured: atomic.NewUint64(0),
	}
}

// UpdatePacketFilter updates the packet filter used for packet capture
func (m *Manager) UpdatePacketFilter(filters []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newFilterExpression := computeGlobalFilter(filters)
	if newFilterExpression == m.currentPacketFilterExpression {
		return nil
	}

	m.stop()

	if newFilterExpression == "" {
		return nil
	}

	tpacket, err := afpacket.NewTPacket(afpacket.OptFrameSize(captureLen))
	if err != nil {
		return fmt.Errorf("failed to update packet filter: %w", err)
	}

	filter, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, captureLen, newFilterExpression)
	if err != nil {
		return fmt.Errorf("failed to update packet filter: %w", err)
	}

	if err := tpacket.SetBPF(pcapFilterToBpfFilter(filter)); err != nil {
		return fmt.Errorf("failed to update packet filter: %w", err)
	}

	m.start(tpacket)
	return nil
}

// Stop stops the packet capture
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stop()
}

func (m *Manager) start(tpacket *afpacket.TPacket) {
	m.state = stateRunning
	m.wg.Add(1)
	packetSource := gopacket.NewPacketSource(tpacket, layers.LayerTypeEthernet)
	packetSource.NoCopy = true
	packetsChan := packetSource.Packets()
	go func() {
		defer m.wg.Done()
		defer tpacket.Close()
		defer func() {
			m.state = stateStopped
		}()

		for {
			select {
			case <-m.ctx.Done():
				return
			case state := <-m.stateChan:
				if state == stateStopped {
					return
				}
			default:
				select {
				case state := <-m.stateChan:
					if state == stateStopped {
						return
					}
				case packet, ok := <-packetsChan:
					if !ok {
						return
					}

					metadata := packet.Metadata()
					if metadata == nil {
						continue
					}

					m.pktCaptured.Inc()
					m.pktBytesCaptured.Add(uint64(metadata.CaptureInfo.Length))
					m.eventStub.Timestamp = metadata.CaptureInfo.Timestamp
					m.eventStub.Packet.Packet = packet
					m.onPacketEvent(m.eventStub)
				}
			}
		}
	}()
}

func (m *Manager) stop() {
	if m.state == stateRunning {
		m.stateChan <- stateStopped
		m.wg.Wait()
	}
}

func computeGlobalFilter(filters []string) string {
	var sb strings.Builder
	for i, filter := range filters {
		sb.WriteRune('(')
		sb.WriteString(filter)
		sb.WriteRune(')')
		if i < len(filters)-1 {
			sb.WriteString(" || ")
		}
	}
	return sb.String()
}

func pcapFilterToBpfFilter(pcapFiler []pcap.BPFInstruction) []bpf.RawInstruction {
	bpfFilter := make([]bpf.RawInstruction, len(pcapFiler))
	for i, p := range pcapFiler {
		bpfFilter[i] = bpf.RawInstruction{
			Op: p.Code,
			Jt: p.Jt,
			Jf: p.Jf,
			K:  p.K,
		}
	}
	return bpfFilter
}
