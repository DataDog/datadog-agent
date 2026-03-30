// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/socket-contention-kern.c pkg/ebpf/bytecode/build/runtime/socket-contention.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/socket-contention.c pkg/ebpf/bytecode/runtime/socket-contention.go runtime

package socketcontention

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/socketcontention/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	statsMapName          = "socket_contention_stats"
	socketContentionGroup = "socket_contention"
)

var minimumKernelVersion = kernel.VersionCode(5, 5, 0)

// Probe is the eBPF side of the socket contention check.
type Probe struct {
	m        *manager.Manager
	statsMap *maps.GenericMap[uint32, ebpfSocketContentionStats]
}

// NewProbe creates a [Probe].
func NewProbe(cfg *ebpf.Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("detect kernel version: %w", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}

	var probe *Probe
	err = ebpf.LoadCOREAsset("socket-contention.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startProbe(buf, opts)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load CO-RE socket contention probe: %w", err)
	}

	return probe, nil
}

func startProbe(buf bytecode.AssetReader, managerOptions manager.Options) (*Probe, error) {
	m := &manager.Manager{
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__sock_init_data", UID: socketContentionGroup}},
		},
		Maps: []*manager.Map{
			{Name: statsMapName},
		},
	}

	managerOptions.RemoveRlimit = true
	if err := m.InitWithOptions(buf, managerOptions); err != nil {
		return nil, fmt.Errorf("init ebpf manager: %w", err)
	}

	if err := m.Start(); err != nil {
		return nil, fmt.Errorf("start ebpf manager: %w", err)
	}

	statsMap, err := maps.GetMap[uint32, ebpfSocketContentionStats](m, statsMapName)
	if err != nil {
		return nil, fmt.Errorf("get map %q: %w", statsMapName, err)
	}

	ebpf.AddNameMappings(m, socketContentionGroup)
	ebpf.AddProbeFDMappings(m)

	return &Probe{
		m:        m,
		statsMap: statsMap,
	}, nil
}

// Close releases all associated resources.
func (p *Probe) Close() {
	if p == nil || p.m == nil {
		return
	}

	ebpf.RemoveNameMappings(p.m)
	if err := p.m.Stop(manager.CleanAll); err != nil {
		log.Warnf("error stopping socket contention probe: %s", err)
	}
}

// GetAndFlush gets the current stats and clears the map.
func (p *Probe) GetAndFlush() model.SocketContentionStats {
	key := uint32(0)
	var raw ebpfSocketContentionStats
	if err := p.statsMap.Lookup(&key, &raw); err != nil {
		return model.SocketContentionStats{}
	}

	if err := p.statsMap.Delete(&key); err != nil {
		log.Warnf("failed to delete socket contention stat: %s", err)
	}

	return model.SocketContentionStats{
		SocketInits: raw.Inits,
	}
}
