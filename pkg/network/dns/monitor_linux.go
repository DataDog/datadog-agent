// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dns

import (
	"fmt"
	"math"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dnsMonitor struct {
	*socketFilterSnooper
	p *ebpfProgram
}

// NewReverseDNS starts snooping on DNS traffic to allow IP -> domain reverse resolution
func NewReverseDNS(cfg *config.Config, _ telemetry.Component) (ReverseDNS, error) {
	// Create the RAW_SOCKET inside the root network namespace
	var (
		packetSrc *filter.AFPacketSource
		srcErr    error
		ns        netns.NsHandle
	)
	ns, err := cfg.GetRootNetNs()
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	err = kernel.WithNS(ns, func() error {
		packetSrc, srcErr = filter.NewAFPacketSource(4 << 20) // 4 MB total
		return srcErr
	})
	if err != nil {
		return nil, err
	}

	currKernelVersion, err := kernel.HostVersion()
	if err != nil {
		// if the platform couldn't be determined, treat it as new kernel case
		log.Warn("could not detect the platform, will use kprobes from kernel version >= 4.1.0")
		currKernelVersion = math.MaxUint32
	}
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)

	var p *ebpfProgram
	if pre410Kernel || cfg.EnableEbpfless {
		if bpfFilter, err := generateBPFFilter(cfg); err != nil {
			return nil, fmt.Errorf("error creating bpf classic filter: %w", err)
		} else if err = packetSrc.SetBPF(bpfFilter); err != nil {
			return nil, fmt.Errorf("could not set BPF filter on packet source: %w", err)
		}
	} else {
		p, err = newEBPFProgram(cfg)
		if err != nil {
			return nil, fmt.Errorf("error creating ebpf program: %w", err)
		}

		if err := p.Init(); err != nil {
			return nil, fmt.Errorf("error initializing ebpf programs: %w", err)
		}

		filter, _ := p.GetProbe(manager.ProbeIdentificationPair{EBPFFuncName: probes.SocketDNSFilter, UID: probeUID})
		if filter == nil {
			return nil, fmt.Errorf("error retrieving socket filter")
		}

		if err = packetSrc.SetEbpf(filter); err != nil {
			return nil, fmt.Errorf("could not set file descriptor for eBPF program: %w", err)
		}
	}

	snoop, err := newSocketFilterSnooper(cfg, packetSrc)
	if err != nil {
		return nil, err
	}

	return &dnsMonitor{
		snoop,
		p,
	}, nil
}

func (m *dnsMonitor) WaitForDomain(domain string) error {
	return m.statKeeper.WaitForDomain(domain)
}

// Start starts the monitor
func (m *dnsMonitor) Start() error {
	if m.p != nil {
		return m.p.Start()
	}
	return nil
}

// Close releases associated resources
func (m *dnsMonitor) Close() {
	m.socketFilterSnooper.Close()
	if m.p != nil {
		ddebpf.RemoveNameMappings(m.p.Manager)
		_ = m.p.Stop(manager.CleanAll)
	}
}
