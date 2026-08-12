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
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	// The port list has already been sanitized in config.New()
	ports := e.cfg.DNSMonitoringPortList
	log.Infof("DNS monitoring ports: %v", ports)

	constantEditors := make([]manager.ConstantEditor, 0, config.DNSPortsMax+1)
	if e.cfg.CollectDNSStats {
		constantEditors = append(constantEditors, manager.ConstantEditor{
			Name:  "dns_stats_enabled",
			Value: uint64(1),
		})
	}
	for i := 0; i < config.DNSPortsMax; i++ {
		var val uint64
		if i < len(ports) {
			val = uint64(uint16(ports[i]))
		}
		constantEditors = append(constantEditors, manager.ConstantEditor{
			Name:  fmt.Sprintf("dns_port_%d", i),
			Value: val,
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
	}
	return err
}
