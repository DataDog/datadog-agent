// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

//go:generate go run ../../../../ebpf/include_headers.go ../c/runtime/tcp-queue-length-kern.c ../../../../ebpf/bytecode/build/runtime/tcp-queue-length.c ../../../../ebpf/c
//go:generate go run ../../../../ebpf/bytecode/runtime/integrity.go ../../../../ebpf/bytecode/build/runtime/tcp-queue-length.c ../../../../ebpf/bytecode/runtime/tcp-queue-length.go runtime

package probe

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/iovisor/gobpf/pkg/cpupossible"
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
#include "../c/runtime/tcp-queue-length-kern-user.h"
*/
import "C"

const (
	statsMapName = "tcp_queue_stats"
)

type TCPQueueLengthTracer struct {
	m        *manager.Manager
	statsMap *bpflib.Map
}

func NewTCPQueueLengthTracer(cfg *ebpf.Config) (*TCPQueueLengthTracer, error) {
	if cfg.EnableCORE {
		probe, err := loadTCPQueueLengthCOREProbe(cfg)
		if err == nil {
			return probe, nil
		}

		if !cfg.AllowRuntimeCompiledFallback {
			return nil, fmt.Errorf("error loading CO-RE tcp-queue-length probe: %s. set system_probe_config.allow_runtime_compiled_fallback to true to allow fallback to runtime compilation", err)
		}
		log.Warnf("error loading CO-RE tcp-queue-length probe: %s. falling back to runtime compiled probe", err)
	}

	return loadTCPQueueLengthRuntimeCompiledProbe(cfg)
}

func startTCPQueueLengthProbe(buf bytecode.AssetReader, managerOptions manager.Options) (*TCPQueueLengthTracer, error) {
	probes := []*manager.Probe{
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: "kprobe/tcp_recvmsg", EBPFFuncName: "kprobe__tcp_recvmsg", UID: "tcpq"}},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: "kretprobe/tcp_recvmsg", EBPFFuncName: "kretprobe__tcp_recvmsg", UID: "tcpq"}},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: "kprobe/tcp_sendmsg", EBPFFuncName: "kprobe__tcp_sendmsg", UID: "tcpq"}},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: "kretprobe/tcp_sendmsg", EBPFFuncName: "kretprobe__tcp_sendmsg", UID: "tcpq"}},
	}

	maps := []*manager.Map{
		{Name: "tcp_queue_stats"},
		{Name: "who_recvmsg"},
		{Name: "who_sendmsg"},
	}

	m := &manager.Manager{
		Probes: probes,
		Maps:   maps,
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

	statsMap, ok, err := m.GetMap(statsMapName)
	if err != nil {
		return nil, fmt.Errorf("failed to get map '%s': %w", statsMapName, err)
	} else if !ok {
		return nil, fmt.Errorf("failed to get map '%s'", statsMapName)
	}

	return &TCPQueueLengthTracer{
		m:        m,
		statsMap: statsMap,
	}, nil
}

func (t *TCPQueueLengthTracer) Close() {
	if err := t.m.Stop(manager.CleanAll); err != nil {
		log.Errorf("error stopping TCP Queue Length: %s", err)
	}
}

func (t *TCPQueueLengthTracer) GetAndFlush() TCPQueueLengthStats {
	cpus, err := cpupossible.Get()
	if err != nil {
		log.Errorf("Failed to get online CPUs: %v", err)
		return TCPQueueLengthStats{}
	}
	nbCpus := len(cpus)

	result := make(TCPQueueLengthStats)

	var statsKey C.struct_stats_key
	statsValue := make([]C.struct_stats_value, nbCpus)
	it := t.statsMap.Iterate()
	for it.Next(unsafe.Pointer(&statsKey), unsafe.Pointer(&statsValue[0])) {
		cgroupName := C.GoString(&statsKey.cgroup_name[0])
		// This cannot happen because statsKey.cgroup_name is filled by bpf_probe_read_str which ensures a NULL-terminated string
		if len(cgroupName) >= C.sizeof_struct_stats_key {
			log.Critical("statsKey.cgroup_name wasn’t properly NULL-terminated")
			break
		}

		max := TCPQueueLengthStatsValue{}
		for _, cpu := range cpus {
			if uint32(statsValue[cpu].read_buffer_max_usage) > max.ReadBufferMaxUsage {
				max.ReadBufferMaxUsage = uint32(statsValue[cpu].read_buffer_max_usage)
			}
			if uint32(statsValue[cpu].write_buffer_max_usage) > max.WriteBufferMaxUsage {
				max.WriteBufferMaxUsage = uint32(statsValue[cpu].write_buffer_max_usage)
			}
		}
		result[cgroupName] = max

		if err := t.statsMap.Delete(unsafe.Pointer(&statsKey)); err != nil {
			log.Warnf("failed to delete stat: %s", err)
		}
	}

	if err := it.Err(); err != nil {
		log.Warnf("failed to iterate on TCP queue length stats while flushing: %s", err)
	}

	return result
}

func loadTCPQueueLengthCOREProbe(cfg *ebpf.Config) (*TCPQueueLengthTracer, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("error detecting kernel version: %s", err)
	}
	if kv < kernel.VersionCode(4, 8, 0) {
		return nil, fmt.Errorf("detected kernel version %s, but tcp-queue-length probe requires a kernel version of at least 4.8.0", kv)
	}

	var probe *TCPQueueLengthTracer
	err = ebpf.LoadCOREAsset(cfg, "tcp-queue-length.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startTCPQueueLengthProbe(buf, opts)
		return err
	})
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of tcp-queue-length probe")
	return probe, nil
}

func loadTCPQueueLengthRuntimeCompiledProbe(cfg *ebpf.Config) (*TCPQueueLengthTracer, error) {
	compiledOutput, err := runtime.TcpQueueLength.Compile(cfg, []string{"-g"}, statsd.Client)
	if err != nil {
		return nil, err
	}
	defer compiledOutput.Close()

	return startTCPQueueLengthProbe(compiledOutput, manager.Options{})
}
