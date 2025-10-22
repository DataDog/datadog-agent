// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && linux

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/seccomp-tracer-kern.c pkg/ebpf/bytecode/build/runtime/seccomp-tracer.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/seccomp-tracer.c pkg/ebpf/bytecode/runtime/seccomp-tracer.go runtime

// Package seccomptracer is the system-probe side of the Seccomp Tracer check
package seccomptracer

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/seccomptracer/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	eventsMapName   = "seccomp_events"
	ringBufferPages = 32
	channelSize     = 1024
	moduleName      = "seccomp_tracer"
)

// seccompStatsKey represents the key of a single seccomp event
type seccompStatsKey struct {
	CgroupName    string `json:"cgroupName"`
	SyscallNr     uint32 `json:"syscallNr"`
	SeccompAction uint32 `json:"seccompAction"`
}

// Tracer is the eBPF side of the Seccomp Tracer check
type Tracer struct {
	m            *manager.Manager
	eventHandler *perf.EventHandler

	// In-memory aggregation of events
	statsMu sync.Mutex
	stats   map[seccompStatsKey]uint64
}

// NewTracer creates a [Tracer]
// Note: Seccomp tracer requires CO-RE (uses bpf_task_pt_regs and bpf_get_current_task_btf)
func NewTracer(cfg *ebpf.Config) (*Tracer, error) {
	if !cfg.EnableCORE {
		return nil, fmt.Errorf("seccomp tracer requires CO-RE support (set system_probe_config.enable_co_re to true)")
	}

	return loadSeccompTracerCOREProbe(cfg)
}

func startSeccompTracerProbe(buf bytecode.AssetReader, managerOptions manager.Options) (*Tracer, error) {
	t := &Tracer{
		stats: make(map[seccompStatsKey]uint64),
	}

	// Create event handler for ring buffer
	eventHandler, err := perf.NewEventHandler(
		eventsMapName,
		t.handleEvent,
		perf.UseRingBuffers(ringBufferPages*os.Getpagesize(), channelSize),
		perf.SendTelemetry(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event handler: %w", err)
	}
	t.eventHandler = eventHandler

	// Create manager with event handler as a modifier
	m := ebpf.NewManagerWithDefault(&manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kretprobe__seccomp_run_filters",
					UID:          "seccomp_tracer",
				},
			},
		},
	}, moduleName, eventHandler, &ebpftelemetry.ErrorsTelemetryModifier{})

	if err := m.InitWithOptions(buf, &managerOptions); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	if err := m.Start(); err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	ebpf.AddProbeFDMappings(m.Manager)
	ebpf.AddNameMappings(m.Manager, moduleName)

	t.m = m.Manager
	return t, nil
}

// handleEvent processes a single seccomp event from the ring buffer
func (t *Tracer) handleEvent(data []byte) {
	if len(data) < int(unsafe.Sizeof(SeccompEvent{})) {
		log.Warnf("seccomp event data too short: %d bytes", len(data))
		return
	}

	// Parse the event
	event := (*SeccompEvent)(unsafe.Pointer(&data[0]))

	cgroupName := unix.ByteSliceToString(event.Cgroup[:])

	key := seccompStatsKey{
		CgroupName:    cgroupName,
		SyscallNr:     event.Nr,
		SeccompAction: event.Action,
	}

	// Aggregate in memory
	t.statsMu.Lock()
	t.stats[key]++
	t.statsMu.Unlock()
}

// Close releases all associated resources
func (t *Tracer) Close() {
	ebpf.RemoveNameMappings(t.m)
	if err := t.m.Stop(manager.CleanAll); err != nil {
		log.Errorf("error stopping Seccomp Tracer: %s", err)
	}
}

// GetAndFlush gets the aggregated stats and clears them
func (t *Tracer) GetAndFlush() model.SeccompStats {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()

	result := make(model.SeccompStats, 0, len(t.stats))

	for key, count := range t.stats {
		result = append(result, model.SeccompStatsEntry{
			CgroupName:    key.CgroupName,
			SyscallNr:     key.SyscallNr,
			SeccompAction: key.SeccompAction,
			Count:         count,
		})
	}

	// Clear the stats
	t.stats = make(map[seccompStatsKey]uint64)

	return result
}

func loadSeccompTracerCOREProbe(cfg *ebpf.Config) (*Tracer, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("error detecting kernel version: %w", err)
	}
	// Ring buffers require kernel 5.8+
	if kv < kernel.VersionCode(5, 8, 0) {
		return nil, fmt.Errorf("detected kernel version %s, but seccomp-tracer probe requires a kernel version of at least 5.8.0 (for ring buffers)", kv)
	}

	filename := "seccomp-tracer.o"
	if cfg.BPFDebug {
		log.Infof("Using debug version of seccomp-tracer probe")
		filename = "seccomp-tracer-debug.o"
	}

	var probe *Tracer
	err = ebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startSeccompTracerProbe(buf, opts)
		return err
	})
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of seccomp-tracer probe with ring buffer")
	return probe, nil
}
