// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"fmt"
	"os"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	"github.com/DataDog/datadog-agent/pkg/util/encoding"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// 5.13 for kfuncs
// 6.2 for bpf_rcu_read_lock kfunc
var minimumKernelVersion = kernel.VersionCode(6, 2, 0)

// Probe is the eBPF side of the noisy neighbor check
type Probe struct {
	mgr *ddebpf.Manager

	// cgroup id -> max ts
	stats map[uint64]uint64
	// cgroup id -> name
	names map[uint64]string
}

// NewProbe creates a [Probe]
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("kernel version: %s", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}

	p := &Probe{
		stats: make(map[uint64]uint64),
		names: make(map[uint64]string),
	}
	// TODO noisy: figure out what you want these sizes to be. ringbuf size must be power of 2
	ringbufSize := 2 * os.Getpagesize()
	chanSize := 100
	runqPool := ddsync.NewDefaultTypedPool[runqEvent]()
	handler := encoding.BinaryUnmarshalCallback(runqPool.Get, func(e *runqEvent, err error) {
		if err != nil {
			if e != nil {
				runqPool.Put(e)
			}
			log.Debug(err.Error())
			return
		}
		p.handleEvent(e)
	})
	eventHandler, err := perf.NewEventHandler("runq_events", handler,
		perf.UseRingBuffers(ringbufSize, chanSize),
		perf.SendTelemetry(cfg.InternalTelemetryEnabled),
	)

	filename := "noisy-neighbor.o"
	if cfg.BPFDebug {
		filename = "noisy-neighbor-debug.o"
	}
	err = ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		p.mgr = ddebpf.NewManagerWithDefault(&manager.Manager{}, "noisy_neighbor", eventHandler)
		const uid = "noisy"
		p.mgr.Probes = []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_wakeup", UID: uid}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tp_sched_switch", UID: uid}},
		}
		p.mgr.Maps = []*manager.Map{
			{Name: "runq_enqueued"},
			{Name: "cgroup_id_to_last_event_ts"},
		}
		if err := p.mgr.InitWithOptions(buf, &opts); err != nil {
			return fmt.Errorf("failed to init ebpf manager: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = p.mgr.Start()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Close releases all associated resources
func (p *Probe) Close() {
	if p.mgr != nil {
		if err := p.mgr.Stop(manager.CleanAll); err != nil {
			log.Warnf("error stopping ebpf manager: %s", err)
		}
	}
}

// GetAndFlush gets the stats
func (p *Probe) GetAndFlush() []model.NoisyNeighborStats {
	// TODO noisy: populate stats you want to return to the core check here
	// this is just an example
	var nnstats []model.NoisyNeighborStats
	for id, maxLatency := range p.stats {
		name := p.names[id]
		nnstats = append(nnstats, model.NoisyNeighborStats{
			Name:       name,
			MaxLatency: maxLatency,
		})
	}
	clear(p.stats)
	clear(p.names)
	return nnstats
}

func (p *Probe) handleEvent(e *runqEvent) {
	// log.Debugf("noisy neighbor event: %+v", e)
	// TODO noisy: handle ebpf data here, this is just an example
	v := p.stats[e.CgroupID]
	if e.RunqLatency > v {
		p.stats[e.CgroupID] = e.RunqLatency
	}
	if _, ok := p.names[e.CgroupID]; !ok {
		p.names[e.CgroupID] = e.CgroupName
	}
}
