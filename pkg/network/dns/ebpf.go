// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dns

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

const probeUID = "dns"

type ebpfProgram struct {
	*manager.Manager
	cfg      *config.Config
	bytecode bytecode.AssetReader
}

func newEBPFProgram(c *config.Config) (*ebpfProgram, error) {
	bc, err := netebpf.ReadDNSModule(c.BPFDir, c.BPFDebug)
	if err != nil {
		return nil, err
	}

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probes.SocketDNSFilter,
					UID:          probeUID,
				},
			},
		},
	}

	return &ebpfProgram{
		Manager:  mgr,
		bytecode: bc,
		cfg:      c,
	}, nil
}

func (e *ebpfProgram) Init() error {
	defer e.bytecode.Close()

	var constantEditors []manager.ConstantEditor
	if e.cfg.CollectDNSStats {
		constantEditors = append(constantEditors, manager.ConstantEditor{
			Name:  "dns_stats_enabled",
			Value: uint64(1),
		})
	}

	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if e.cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}
	err := e.InitWithOptions(e.bytecode, manager.Options{
		RemoveRlimit: true,
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probes.SocketDNSFilter,
					UID:          probeUID,
				},
			},
		},
		ConstantEditors:           constantEditors,
		DefaultKprobeAttachMethod: kprobeAttachMethod,
		BypassEnabled:             e.cfg.BypassEnabled,
	})
	if err == nil {
		ddebpf.AddNameMappings(e.Manager, "npm_dns")

		dnsPortsMap, _, err := e.Manager.GetMap("dns_ports")
		if err != nil {
			return fmt.Errorf("getting dns_ports map: %w", err)
		}

		// clear out existing entries, because if the user changes the config
		// to remove a port, the map will still be there with the old config
		var key uint16
		iter := dnsPortsMap.Iterate()
		for iter.Next(&key, nil) {
			if err := dnsPortsMap.Delete(&key); err != nil {
				return fmt.Errorf("error deleting dns port %d: %w", key, err)
			}
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("error iterating dns_ports map: %w", err)
		}

		val := uint8(1)
		for _, p := range e.cfg.DNSMonitoringPortList {
			port := uint16(p)
			if err := dnsPortsMap.Put(&port, &val); err != nil {
				return fmt.Errorf("error putting dns port %d: %w", p, err)
			}
		}
	}
	return err
}
