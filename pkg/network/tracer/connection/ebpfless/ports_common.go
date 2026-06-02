// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux || darwin

// Package ebpfless contains supporting code for the ebpfless tracer
package ebpfless

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type boundPortsKey struct {
	proto network.ConnectionType
	port  uint16
}

// BoundPorts is a collection of bound ports on the host
// that is periodically updated using platform-specific mechanisms
type BoundPorts struct {
	mu sync.RWMutex

	config *config.Config
	ports  map[boundPortsKey]struct{}

	stop chan struct{}

	// readPorts is the platform-specific function that reads listening ports
	// It returns a map of ports that should replace the current set
	readPorts func(cfg *config.Config) (map[boundPortsKey]struct{}, error)
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
					log.Errorf("error updating bound ports, exiting loop: %s", err)
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
	ports, err := b.readPorts(b.config)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.ports = ports
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
