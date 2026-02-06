// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package ebpfless contains supporting code for the ebpfless tracer
package ebpfless

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewBoundPorts returns a new BoundPorts instance
func NewBoundPorts(cfg *config.Config) *BoundPorts {
	ino, _ := netns.GetCurrentIno()
	return &BoundPorts{
		config:    cfg,
		ports:     map[boundPortsKey]struct{}{},
		stop:      make(chan struct{}),
		readPorts: readPortsLinux(ino),
	}
}

// readPortsLinux returns a function that reads listening ports from procfs,
// filtering by the given network namespace inode
func readPortsLinux(ino uint32) func(cfg *config.Config) (map[boundPortsKey]struct{}, error) {
	return func(cfg *config.Config) (map[boundPortsKey]struct{}, error) {
		ports := make(map[boundPortsKey]struct{})

		tcpPorts, err := network.ReadListeningPorts(cfg.ProcRoot, network.TCP, cfg.CollectTCPv6Conns)
		if err != nil {
			return nil, fmt.Errorf("failed to read TCP listening ports: %s", err)
		}

		for p := range tcpPorts {
			if p.Ino != ino {
				continue
			}
			log.Debugf("adding TCP port binding: netns: %d port: %d", p.Ino, p.Port)
			ports[boundPortsKey{network.TCP, p.Port}] = struct{}{}
		}

		udpPorts, err := network.ReadListeningPorts(cfg.ProcRoot, network.UDP, cfg.CollectUDPv6Conns)
		if err != nil {
			return nil, fmt.Errorf("failed to read UDP listening ports: %s", err)
		}

		for p := range udpPorts {
			if p.Ino != ino {
				continue
			}
			// ignore ephemeral port binds as they are more likely to be from
			// clients calling bind with port 0
			if network.IsPortInEphemeralRange(network.AFINET, network.UDP, p.Port) == network.EphemeralTrue {
				log.Debugf("ignoring ephemeral UDP port bind to %d", p)
				continue
			}
			log.Debugf("adding UDP port binding: netns: %d port: %d", p.Ino, p.Port)
			ports[boundPortsKey{network.UDP, p.Port}] = struct{}{}
		}

		return ports, nil
	}
}
