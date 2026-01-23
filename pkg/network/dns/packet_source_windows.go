// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package dns

import (
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
)

var _ filter.PacketSource = &windowsPacketSource{}

type windowsPacketSource struct {
	di   *dnsDriver
	exit chan struct{}
	mu   sync.Mutex
}

// newWindowsPacketSource constructs a new packet source
func newWindowsPacketSource(telemetrycomp telemetry.Component, dnsMonitoringPorts []int) (filter.PacketSource, error) {
	di, err := newDriver(telemetrycomp, dnsMonitoringPorts)
	if err != nil {
		return nil, err
	}
	return &windowsPacketSource{
		di:   di,
		exit: make(chan struct{}),
	}, nil
}

func (p *windowsPacketSource) VisitPackets(visit func([]byte, filter.PacketInfo, time.Time) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		// break out of loop if exit is closed
		select {
		case <-p.exit:
			return nil
		default:
		}

		didReadPacket, err := p.di.ReadDNSPacket(visit)
		if err != nil {
			return err
		}
		if !didReadPacket {
			return nil
		}
	}
}

func (p *windowsPacketSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeIPv4
}

func (p *windowsPacketSource) Close() {
	close(p.exit)

	// wait for the VisitPackets loop to finish, then close
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.di.Close()
}
