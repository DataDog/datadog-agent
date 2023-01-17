// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package dns

import (
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var _ packetSource = &windowsPacketSource{}

type windowsPacketSource struct {
	di *dnsDriver
}

// newWindowsPacketSource constructs a new packet source
func newWindowsPacketSource() (packetSource, error) {
	di, err := newDriver()
	if err != nil {
		return nil, err
	}
	return &windowsPacketSource{di: di}, nil
}

func (p *windowsPacketSource) VisitPackets(exit <-chan struct{}, visit func([]byte, time.Time) error) error {
	for {
		didReadPacket, err := p.di.ReadDNSPacket(visit)
		if err != nil {
			return err
		}
		if !didReadPacket {
			return nil
		}

		// break out of loop if exit is closed
		select {
		case <-exit:
			return nil
		default:
		}
	}
}

func (p *windowsPacketSource) PacketType() gopacket.LayerType {
	return layers.LayerTypeIPv4
}

func (p *windowsPacketSource) Stats() map[string]int64 {
	// this is a no-op because all the stats are handled by driver_interface.go
	s, _ := p.di.GetStatsForHandle()
	return s["handle"]
}

func (p *windowsPacketSource) Close() {
	_ = p.di.Close()
}
