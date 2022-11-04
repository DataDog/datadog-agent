// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

//go:generate go run ../../../../ebpf/include_headers.go ../c/runtime/oom-kill-kern.c ../../../../ebpf/bytecode/build/runtime/oom-kill.c ../../../../ebpf/c
//go:generate go run ../../../../ebpf/bytecode/runtime/integrity.go ../../../../ebpf/bytecode/build/runtime/oom-kill.c ../../../../ebpf/bytecode/runtime/oom-kill.go runtime

package probe

import (
	"fmt"
	"math"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"
	bpflib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <string.h>
#include "../c/runtime/oom-kill-kern-user.h"
*/
import "C"

const oomMapName = "oom_stats"

type OOMKillProbe struct {
	m      *manager.Manager
	oomMap *bpflib.Map
}

func NewOOMKillProbe(cfg *ebpf.Config) (*OOMKillProbe, error) {
	if cfg.EnableCORE {
		probe, err := loadCOREProbe(cfg)
		if err == nil {
			return probe, err
		}

		if !cfg.AllowRuntimeCompiledFallback {
			return nil, fmt.Errorf("error loading CO-RE oom-kill probe: %s. set system_probe_config.allow_runtime_compiled_fallback to true to allow fallback to runtime compilation.", err)
		}
		log.Warnf("error loading CO-RE oom-kill probe: %s. falling back to runtime compiled probe", err)
	}

	return loadRuntimeCompiledProbe(cfg)
}

func loadCOREProbe(cfg *ebpf.Config) (*OOMKillProbe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("error detecting kernel version: %s", err)
	}
	if kv < kernel.VersionCode(4, 9, 0) {
		return nil, fmt.Errorf("detected kernel version %s, but oom-kill probe requires a kernel version of at least 4.9.0.", kv)
	}

	var telemetry ebpf.COREResult
	defer func() {
		ebpf.StoreCORETelemetryForAsset("oomKill", telemetry)
	}()

	var btfData *btf.Spec
	btfData, telemetry = ebpf.GetBTF(cfg.BTFPath, cfg.BPFDir)

	if btfData == nil {
		return nil, fmt.Errorf("could not find BTF data on host")
	}

	buf, err := bytecode.GetReader(filepath.Join(cfg.BPFDir, "co-re"), "oom-kill.o")
	if err != nil {
		telemetry = ebpf.AssetReadError
		return nil, fmt.Errorf("error reading oom-kill.o file: %s", err)
	}
	defer buf.Close()

	probe, err := startOOMKillProbe(buf, btfData)
	if err != nil {
		telemetry = ebpf.VerifierError
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of oom-kill probe")
	return probe, nil
}

func loadRuntimeCompiledProbe(cfg *ebpf.Config) (*OOMKillProbe, error) {
	buf, err := runtime.OomKill.Compile(cfg, []string{"-g"}, statsd.Client)
	if err != nil {
		return nil, err
	}
	defer buf.Close()

	return startOOMKillProbe(buf, nil)
}

func startOOMKillProbe(buf bytecode.AssetReader, btfData *btf.Spec) (*OOMKillProbe, error) {
	probes := []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: "kprobe/oom_kill_process", EBPFFuncName: "kprobe__oom_kill_process", UID: "oom"},
		},
	}

	maps := []*manager.Map{
		{Name: "oom_stats"},
	}

	m := &manager.Manager{
		Probes: probes,
		Maps:   maps,
	}

	managerOptions := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		VerifierOptions: bpflib.CollectionOptions{
			Programs: bpflib.ProgramOptions{
				KernelTypes: btfData,
			},
		},
	}

	if err := m.InitWithOptions(buf, managerOptions); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	if err := m.Start(); err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	oomMap, ok, err := m.GetMap(oomMapName)
	if err != nil {
		return nil, fmt.Errorf("failed to get map '%s': %w", oomMapName, err)
	} else if !ok {
		return nil, fmt.Errorf("failed to get map '%s'", oomMapName)
	}

	return &OOMKillProbe{
		m:      m,
		oomMap: oomMap,
	}, nil
}

func (k *OOMKillProbe) Close() {
	if err := k.m.Stop(manager.CleanAll); err != nil {
		log.Errorf("error stopping OOM Kill: %s", err)
	}
}

func (k *OOMKillProbe) GetAndFlush() (results []OOMKillStats) {
	var pid uint32
	var stat C.struct_oom_stats
	it := k.oomMap.Iterate()
	for it.Next(unsafe.Pointer(&pid), unsafe.Pointer(&stat)) {
		results = append(results, convertStats(stat))

		if err := k.oomMap.Delete(unsafe.Pointer(&pid)); err != nil {
			log.Warnf("failed to delete stat: %s", err)
		}
	}

	if err := it.Err(); err != nil {
		log.Warnf("failed to iterate on OOM stats while flushing: %s", err)
	}

	return results
}

func convertStats(in C.struct_oom_stats) (out OOMKillStats) {
	out.CgroupName = C.GoString(&in.cgroup_name[0])
	out.Pid = uint32(in.pid)
	out.TPid = uint32(in.tpid)
	out.FComm = C.GoString(&in.fcomm[0])
	out.TComm = C.GoString(&in.tcomm[0])
	out.Pages = uint64(in.pages)
	out.MemCgOOM = uint32(in.memcg_oom)
	return
}
