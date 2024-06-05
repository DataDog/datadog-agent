// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ebpfless contains supporting code for the ebpfless tracer
package ebpfless

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type boundPortsKey struct {
	proto network.ConnectionType
	port  uint16
}

// BoundPorts is a collection of bound ports on the host
// that is periodically updated from procfs
type BoundPorts struct {
	mu sync.RWMutex

	config *config.Config
	ports  map[boundPortsKey]struct{}

	stop chan struct{}
	ino  uint32
}

// NewBoundPorts returns a new BoundPorts instance
func NewBoundPorts(cfg *config.Config) *BoundPorts {
	ino, _ := kernel.GetCurrentIno()
	return &BoundPorts{
		config: cfg,
		ports:  map[boundPortsKey]struct{}{},
		stop:   make(chan struct{}),
		ino:    ino,
	}
}

// Start starts a BoundPorts instance
func (b *BoundPorts) Start() error {
	if err := b.update(); err != nil {
		return err
	}

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-b.stop:
				return
			case <-ticker.C:
				if err := b.update(); err != nil {
					log.Errorf("error updating bound ports, exiting loop: %w", err)
					return
				}
			}
		}
	}()

	return nil
}

// Stop stops a BoundPorts instance
func (b *BoundPorts) Stop() {
	close(b.stop)
}

func (b *BoundPorts) update() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	tcpPorts, err := network.ReadListeningPorts(b.config.ProcRoot, network.TCP, b.config.CollectTCPv6Conns)
	if err != nil {
		return fmt.Errorf("failed to read initial TCP pid->port mapping: %s", err)
	}

	for p := range tcpPorts {
		if p.Ino != b.ino {
			continue
		}
		log.Debugf("adding initial TCP port binding: netns: %d port: %d", p.Ino, p.Port)
		b.ports[boundPortsKey{network.TCP, p.Port}] = struct{}{}
	}

	udpPorts, err := network.ReadListeningPorts(b.config.ProcRoot, network.UDP, b.config.CollectUDPv6Conns)
	if err != nil {
		return fmt.Errorf("failed to read initial UDP pid->port mapping: %s", err)
	}

	for p := range udpPorts {
		// ignore ephemeral port binds as they are more likely to be from
		// clients calling bind with port 0
		if network.IsPortInEphemeralRange(network.AFINET, network.UDP, p.Port) == network.EphemeralTrue {
			log.Debugf("ignoring initial ephemeral UDP port bind to %d", p)
			continue
		}

		if p.Ino != b.ino {
			continue
		}

		log.Debugf("adding initial UDP port binding: netns: %d port: %d", p.Ino, p.Port)
		b.ports[boundPortsKey{network.UDP, p.Port}] = struct{}{}
	}

	return nil

}

// Find returns `true` if the given `(proto, port)` exists in
// the BoundPorts collection
func (b *BoundPorts) Find(proto network.ConnectionType, port uint16) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	_, ok := b.ports[boundPortsKey{proto, port}]
	return ok
}
