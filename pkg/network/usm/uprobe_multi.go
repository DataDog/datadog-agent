// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package usm

import (
	"errors"
	"fmt"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type multiUprobeProtocol interface {
	getUprobeAttacher() *uprobes.UprobeAttacher
}

// uprobeMulti loads a second TLS program graph with private tail-call arrays.
// The original graph stays usable for the perf backend when feature detection
// or initialization fails. It shares all data maps and existing instruction
// patches, so connection state and protocol consumers remain unchanged.
type uprobeMulti struct {
	enabled      bool
	roots        []string
	routes       []manager.TailCallRoute
	attachers    []*uprobes.UprobeAttacher
	prepared     *uprobes.MultiProgramSpec
	loaded       *uprobes.MultiPrograms
	generation   uint64
	supportProbe func() error
}

func (*uprobeMulti) String() string { return "uprobe-multi" }

func (multi *uprobeMulti) configure(enabled bool, specs []*protocols.ProtocolSpec, routes []manager.TailCallRoute) {
	multi.enabled = enabled
	multi.roots = nil
	multi.attachers = nil
	multi.routes = routes
	for _, spec := range specs {
		protocol, ok := spec.Instance.(multiUprobeProtocol)
		if !ok {
			continue
		}
		multi.attachers = append(multi.attachers, protocol.getUprobeAttacher())
		for _, probe := range spec.Probes {
			if strings.Contains(probe.EBPFFuncName, "uprobe__") || strings.Contains(probe.EBPFFuncName, "uretprobe__") {
				multi.roots = append(multi.roots, probe.EBPFFuncName)
			}
		}
	}
}

func (multi *uprobeMulti) BeforeInit(mgr *manager.Manager, _ names.ModuleName, _ *manager.Options) error {
	multi.prepared = nil
	multi.generation++
	generation := multi.generation
	if !multi.enabled || len(multi.roots) == 0 {
		return nil
	}
	// Appending from the final modifier puts this after constants, telemetry
	// and other instruction patchers. A retry uses only its newest capture.
	mgr.InstructionPatchers = append(mgr.InstructionPatchers, func(mgr *manager.Manager) error {
		if generation != multi.generation {
			return nil
		}
		// Run after the manager has raised RLIMIT_MEMLOCK. Unsupported kernels
		// are expected; permissions and other failures are reported separately.
		probe := multi.supportProbe
		if probe == nil {
			probe = features.HaveBPFLinkUprobeMulti
		}
		if err := probe(); err != nil {
			if !errors.Is(err, ebpf.ErrNotSupported) {
				log.Warnf("Cannot detect multi-uprobe support; using perf uprobes: %v", err)
			}
			return nil
		}
		var err error
		multi.prepared, err = uprobes.PrepareUprobeMultiPrograms(mgr, multi.roots, multi.routes)
		if err != nil {
			log.Warnf("Cannot prepare multi-uprobe programs; using perf uprobes: %v", err)
		}
		return nil
	})
	return nil
}

func (multi *uprobeMulti) AfterInit(mgr *manager.Manager, module names.ModuleName, opts *manager.Options) error {
	if multi.prepared == nil {
		return nil
	}
	prepared := multi.prepared
	multi.prepared = nil
	loaded, err := prepared.Load(mgr, opts.VerifierOptions, module.Name())
	if err != nil {
		log.Warnf("Cannot load multi-uprobe programs; using perf uprobes: %v", err)
		return nil
	}
	multi.loaded = loaded
	for _, attacher := range multi.attachers {
		attacher.UseUprobeMultiPrograms(loaded.Programs())
	}
	log.Info("Using multi-uprobe links for USM TLS monitoring")
	return nil
}

func (multi *uprobeMulti) BeforeStop(_ *manager.Manager, _ names.ModuleName, _ manager.MapCleanupType) error {
	// ebpfProgram.Close stops all protocol attachers before the manager.
	multi.loaded.Close()
	multi.loaded = nil
	return nil
}

// setBypass extends manager pause/resume to the cloned programs. The manager
// only toggles indexes for its own active probes; multi links are owned by the
// attachers. Both graphs share the map declared in bpf_bypass.h.
func (multi *uprobeMulti) setBypass(mgr *manager.Manager, paused bool) error {
	if multi == nil || multi.loaded == nil {
		return nil
	}
	bypass, found, err := mgr.GetMap("program_bypassed")
	if err != nil || !found {
		return err // The map is absent when bypass is disabled.
	}
	var flag uint32
	if paused {
		flag = 1
	}
	var value any = flag
	if bypass.Type() == ebpf.PerCPUArray {
		cpus, err := ebpf.PossibleCPU()
		if err != nil {
			return err
		}
		values := make([]uint32, cpus)
		for i := range values {
			values[i] = flag
		}
		value = values
	}
	for key := uint32(0); key < bypass.MaxEntries(); key++ {
		if err := bypass.Update(key, value, ebpf.UpdateExist); err != nil {
			return fmt.Errorf("set multi-uprobe bypass: %w", err)
		}
	}
	return nil
}
