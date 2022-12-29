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
	"unsafe"

	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"
	bpflib "github.com/cilium/ebpf"

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
#cgo CFLAGS: -I "${SRCDIR}/../../../../ebpf/c"
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
		return nil, fmt.Errorf("detected kernel version %s, but oom-kill probe requires a kernel version of at least 4.9.0", kv)
	}

	var probe *OOMKillProbe
	err = ebpf.LoadCOREAsset(cfg, "oom-kill.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startOOMKillProbe(buf, opts)
		return err
	})
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of oom-kill probe")
	return probe, nil
}

func loadRuntimeCompiledProbe(cfg *ebpf.Config) (*OOMKillProbe, error) {
	buf, err := runtime.OomKill.Compile(cfg, getCFlags(cfg), statsd.Client)
	if err != nil {
		return nil, err
	}
	defer buf.Close()

	return startOOMKillProbe(buf, manager.Options{})
}

func getCFlags(config *ebpf.Config) []string {
	cflags := []string{"-g"}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

func startOOMKillProbe(buf bytecode.AssetReader, managerOptions manager.Options) (*OOMKillProbe, error) {
	m := &manager.Manager{
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: "kprobe/oom_kill_process", EBPFFuncName: "kprobe__oom_kill_process", UID: "oom"}},
		},
		Maps: []*manager.Map{
			{Name: "oom_stats"},
		},
	}

	managerOptions.RLimit = &unix.Rlimit{
		Cur: math.MaxUint64,
		Max: math.MaxUint64,
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
